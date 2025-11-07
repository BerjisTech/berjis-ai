package server

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
	"time"

	aauth "github.com/berjistech/berjis-ecosystem/ai/service/internal/auth"
	"github.com/berjistech/berjis-ecosystem/ai/service/internal/config"
	db "github.com/berjistech/berjis-ecosystem/ai/service/internal/db"
	"github.com/berjistech/berjis-ecosystem/ai/service/internal/inference"
	"github.com/berjistech/berjis-ecosystem/ai/service/internal/policy"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

type Options struct {
	Config config.Config
	DB     *db.DB
}

func New(opts Options) *fiber.App {
	app := fiber.New()
	// CORS: allow configured origins (including localhost for dev) and *.berjis.tech
	// Keep credentials allowed; do not use '*'.
	cfgOrigins := map[string]struct{}{}
	for _, o := range strings.Split(opts.Config.AllowedOrigins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			cfgOrigins[o] = struct{}{}
		}
	}
	app.Use(func(c *fiber.Ctx) error {
		origin := c.Get("Origin")
		_, listed := cfgOrigins[origin]
		allowed := origin != "" && (listed || origin == "https://berjis.tech" || strings.HasSuffix(origin, ".berjis.tech"))
		if allowed {
			// pre-next: attach CORS early for preflights and normal requests
			c.Set("Access-Control-Allow-Origin", origin)
			c.Set("Vary", "Origin")
			c.Set("Access-Control-Allow-Credentials", "true")
			c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			reqHdrs := strings.TrimSpace(c.Get("Access-Control-Request-Headers"))
			if reqHdrs == "" {
				reqHdrs = "Authorization,Content-Type,Accept"
			}
			c.Set("Access-Control-Allow-Headers", reqHdrs)
			c.Set("Access-Control-Expose-Headers", "X-Berjis-Uid, X-Berjis-Admin")
			if c.Method() == fiber.MethodOptions {
				return c.SendStatus(fiber.StatusNoContent)
			}
		}
		// proceed to handlers
		if err := c.Next(); err != nil {
			return err
		}
		// post-next: ensure headers remain even if a handler overwrote them
		if allowed {
			c.Set("Access-Control-Allow-Origin", origin)
			c.Set("Vary", "Origin")
			c.Set("Access-Control-Allow-Credentials", "true")
			c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			reqHdrs := strings.TrimSpace(c.Get("Access-Control-Request-Headers"))
			if reqHdrs == "" {
				reqHdrs = "Authorization,Content-Type,Accept"
			}
			c.Set("Access-Control-Allow-Headers", reqHdrs)
			c.Set("Access-Control-Expose-Headers", "X-Berjis-Uid, X-Berjis-Admin")
		}
		return nil
	})

	app.Get("/v1/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "message": "ok"})
	})

	// Deep health: report core-api + ollama reachability and echo CORS origin seen
	app.Get("/v1/health/full", func(c *fiber.Ctx) error {
		origin := c.Get("Origin")
		out := fiber.Map{"success": true}
		// Core API
		coreOK := false
		if opts.Config.CoreAPIBase != "" {
			req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(opts.Config.CoreAPIBase, "/")+"/v1/health", http.NoBody)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				coreOK = resp.StatusCode == 200
				resp.Body.Close()
			}
		}
		// Ollama
		ollamaOK := false
		if opts.Config.OllamaBase != "" {
			req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(opts.Config.OllamaBase, "/")+"/api/tags", http.NoBody)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				ollamaOK = resp.StatusCode == 200
				resp.Body.Close()
			}
		}
		out["data"] = fiber.Map{
			"origin":  origin,
			"coreApi": coreOK,
			"ollama":  ollamaOK,
		}
		return c.JSON(out)
	})

	// Root info to avoid 404 on '/' when probed via Cloudflare
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "service": "berjis-ai", "endpoints": []string{"/v1/health", "/v1/models", "/v1/chat", "/v1/completion", "/v1/embeddings"}})
	})

	app.Use("/v1/*", limiter.New(limiter.Config{Max: 120, Expiration: 1 * time.Minute}))

	// Keep tight upstream timeouts to avoid edge proxy timeouts that drop CORS headers.
	httpClientAuth := &http.Client{Timeout: 8 * time.Second}
	// Allow long cold-starts when the first model loads
	httpClientLLM := &http.Client{Timeout: 90 * time.Second}
	verify := aauth.RequireAuth(aauth.Options{CoreAPIBase: opts.Config.CoreAPIBase, HTTP: httpClientAuth})
	ollama := inference.NewOllama(opts.Config.OllamaBase, httpClientLLM)
	metrics := newMetrics()
	var store *db.Store
	if opts.DB != nil {
		store = db.NewStore(opts.DB)
	}

	// Background warmup: trigger default model load so first request is fast.
	go func() {
		dm := strings.TrimSpace(opts.Config.DefaultModel)
		if dm == "" {
			return
		}
		// Try once; errors are non-fatal
		_, _ = ollama.Chat(inference.ChatRequest{Model: dm, Messages: []inference.ChatMessage{{Role: "user", Content: "ping"}}, Stream: false})
	}()

	app.Get("/v1/models", verify, func(c *fiber.Ctx) error {
		tags, err := ollama.Models()
		if err != nil {
			metrics.incError()
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"success": false, "message": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": tags.Models})
	})

	app.Post("/v1/chat", verify, func(c *fiber.Ctx) error {
		var body ChatRequest
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "invalid body"})
		}
		if len(body.Messages) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "messages required"})
		}
		if strings.TrimSpace(body.Model) == "" {
			body.Model = opts.Config.DefaultModel
		}
		sys := policy.BuildSystem()
		msgs := make([]inference.ChatMessage, 0, len(sys)+len(body.Messages))
		msgs = append(msgs, sys...)
		for _, m := range body.Messages {
			msgs = append(msgs, inference.ChatMessage{Role: m.Role, Content: m.Content})
		}
		// basic sensitive prompt guard
		userBlob := ""
		for _, m := range body.Messages {
			if m.Role == "user" {
				userBlob += "\n" + m.Content
			}
		}
		if opts.Config.SafetyStrict && policy.IsSensitiveText(userBlob) {
			metrics.incBlocked()
			kw := policy.MatchedKeyword(userBlob)
			uid, _ := c.Locals("uuid").(string)
			log.Printf("policy: blocked sensitive prompt (uuid=%s keyword=%s)", uid, kw)
			safe := "I can't share developer or internal details. Here's a public overview of Berjis and how to get started at berjis.tech."
			return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"model": body.Model, "message": fiber.Map{"role": "assistant", "content": safe}}})
		}
		temp := body.Temperature
		if temp <= 0 {
			temp = 0.2
		}
		// Explicitly disable streaming; the gateway expects a single complete JSON response
		// Tune for faster CPU inference and keep model warm
		optsMap := map[string]any{"num_predict": 256, "repeat_penalty": 1.07, "top_k": 50, "top_p": 0.9, "num_thread": runtime.NumCPU(), "num_batch": 256}
		res, err := ollama.Chat(inference.ChatRequest{Model: body.Model, Messages: msgs, Temperature: temp, Stream: false, Options: optsMap, KeepAlive: "30m"})
		if err != nil {
			metrics.incError()
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "not found") {
				if body.Model != opts.Config.DefaultModel {
					if res2, err2 := ollama.Chat(inference.ChatRequest{Model: opts.Config.DefaultModel, Messages: msgs, Temperature: temp, Stream: false, Options: optsMap, KeepAlive: "30m"}); err2 == nil {
						body.Model = opts.Config.DefaultModel
						res = res2
					} else {
						tags, terr := ollama.Models()
						if terr == nil {
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "requested model not available", "available": modelNames(tags.Models)})
						}
						return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "requested model not available"})
					}
				} else {
					tags, terr := ollama.Models()
					if terr == nil {
						return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "requested model not available", "available": modelNames(tags.Models)})
					}
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "requested model not available"})
				}
			} else {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
			}
		}
		metrics.incChat(body.Model)
		if store != nil {
			_ = store.IncDaily("chats", body.Model, 1)
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"model": res.Model, "message": res.Message}})
	})

	// Streaming chat: newline-delimited JSON (NDJSON) forwarding Ollama stream.
	app.Post("/v1/chat/stream", verify, func(c *fiber.Ctx) error {
		var body ChatRequest
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "invalid body"})
		}
		if len(body.Messages) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "messages required"})
		}
		if strings.TrimSpace(body.Model) == "" {
			body.Model = opts.Config.DefaultModel
		}
		sys := policy.BuildSystem()
		msgs := make([]inference.ChatMessage, 0, len(sys)+len(body.Messages))
		msgs = append(msgs, sys...)
		for _, m := range body.Messages {
			msgs = append(msgs, inference.ChatMessage{Role: m.Role, Content: m.Content})
		}
		// guard
		userBlob := ""
		for _, m := range body.Messages {
			if m.Role == "user" {
				userBlob += "\n" + m.Content
			}
		}
		if opts.Config.SafetyStrict && policy.IsSensitiveText(userBlob) {
			c.Set("Content-Type", "application/x-ndjson")
			c.Set("Cache-Control", "no-cache")
			c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
				safe := "I can't share developer or internal details. Here's a public overview of Berjis and how to get started at berjis.tech."
				fmt.Fprintf(w, "{\"model\":%q,\"message\":{\"role\":\"assistant\",\"content\":%q},\"done\":true}\n", body.Model, safe)
				_ = w.Flush()
			})
			return nil
		}
		temp := body.Temperature
		if temp <= 0 {
			temp = 0.2
		}
		optsMap := map[string]any{"num_predict": 256, "repeat_penalty": 1.07, "top_k": 50, "top_p": 0.9, "num_thread": runtime.NumCPU(), "num_batch": 256}
		c.Set("Content-Type", "application/x-ndjson")
		c.Set("Cache-Control", "no-cache")
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			_ = ollama.ChatStream(c.Context(), inference.ChatRequest{Model: body.Model, Messages: msgs, Temperature: temp, Stream: true, Options: optsMap, KeepAlive: "30m"}, func(ev inference.ChatResponse) error {
				// escape quotes in content to keep valid JSON lines
				content := strings.ReplaceAll(ev.Message.Content, "\"", "\\\"")
				b := fmt.Sprintf("{\\\"model\\\":%q,\\\"message\\\":{\\\"role\\\":%q,\\\"content\\\":%q},\\\"done\\\":%t}\\n", ev.Model, ev.Message.Role, content, ev.Done)
				if _, err := w.WriteString(b); err != nil {
					return err
				}
				return w.Flush()
			})
		})
		metrics.incChat(body.Model)
		if store != nil {
			_ = store.IncDaily("chats", body.Model, 1)
		}
		return nil
	})

	app.Post("/v1/completion", verify, func(c *fiber.Ctx) error {
		var body CompletionRequest
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "invalid body"})
		}
		if strings.TrimSpace(body.Prompt) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "prompt required"})
		}
		if strings.TrimSpace(body.Model) == "" {
			body.Model = opts.Config.DefaultModel
		}
		text, model, err := ollama.Generate(body.Model, body.Prompt, body.MaxTokens)
		if err != nil {
			metrics.incError()
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "not found") {
				if body.Model != opts.Config.DefaultModel {
					if text2, model2, err2 := ollama.Generate(opts.Config.DefaultModel, body.Prompt, body.MaxTokens); err2 == nil {
						text = text2
						model = model2
					} else {
						tags, terr := ollama.Models()
						if terr == nil {
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "requested model not available", "available": modelNames(tags.Models)})
						}
						return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "requested model not available"})
					}
				} else {
					tags, terr := ollama.Models()
					if terr == nil {
						return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "requested model not available", "available": modelNames(tags.Models)})
					}
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "requested model not available"})
				}
			} else {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
			}
		}
		metrics.incCompletion(body.Model)
		if store != nil {
			_ = store.IncDaily("completions", body.Model, 1)
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"model": model, "text": text}})
	})

	app.Post("/v1/embeddings", verify, func(c *fiber.Ctx) error {
		var body EmbeddingsRequest
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "invalid body"})
		}
		if len(body.Texts) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "texts required"})
		}
		vecs, err := ollama.Embed("nomic-embed-text", body.Texts)
		if err != nil {
			metrics.incError()
			// Avoid 502 so Cloudflare does not replace our body and strip CORS
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
		}
		metrics.incEmbeddings()
		if store != nil {
			_ = store.IncDaily("embeddings", "", 1)
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"model": "nomic-embed-text", "embeddings": vecs}})
	})

	// Admin metrics endpoint; requires admin via Core API
	app.Get("/v1/admin/metrics", verify, func(c *fiber.Ctx) error {
		token := c.Get("Authorization")
		req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(opts.Config.CoreAPIBase, "/")+"/v1/auth/admin/verify", http.NoBody)
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		if token == "" {
			if ck := c.Get("Cookie"); ck != "" {
				req.Header.Set("Cookie", ck)
			}
		}
		resp, err := httpClientAuth.Do(req)
		if err != nil {
			return c.Status(502).JSON(fiber.Map{"success": false, "message": "admin verify failed"})
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return c.Status(403).JSON(fiber.Map{"success": false, "message": "forbidden"})
		}
		snap := metrics.snapshot()
		return c.JSON(fiber.Map{"success": true, "data": snap})
	})

	// Admin metrics daily trend
	app.Get("/v1/admin/metrics/daily", verify, func(c *fiber.Ctx) error {
		if store == nil {
			return c.Status(503).JSON(fiber.Map{"success": false, "message": "metrics store unavailable"})
		}
		token := c.Get("Authorization")
		req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(opts.Config.CoreAPIBase, "/")+"/v1/auth/admin/verify", http.NoBody)
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		if token == "" {
			if ck := c.Get("Cookie"); ck != "" {
				req.Header.Set("Cookie", ck)
			}
		}
		resp, err := httpClientAuth.Do(req)
		if err != nil {
			return c.Status(502).JSON(fiber.Map{"success": false, "message": "admin verify failed"})
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return c.Status(403).JSON(fiber.Map{"success": false, "message": "forbidden"})
		}

		days := 7
		if v := strings.TrimSpace(c.Query("days")); v == "30" {
			days = 30
		}
		rows, err := store.GetDaily(days)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "db error"})
		}

		// Build day axis
		axis := make([]string, days)
		dayIndex := map[string]int{}
		for i := 0; i < days; i++ {
			d := time.Now().AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
			axis[i] = d
			dayIndex[d] = i
		}
		type series map[string][]int64 // model -> points
		out := map[string]series{}
		for _, m := range []string{"chats", "completions", "embeddings"} {
			out[m] = series{"_all": make([]int64, days)}
		}
		// Fill
		for _, r := range rows {
			d := r.Day.Format("2006-01-02")
			idx, ok := dayIndex[d]
			if !ok {
				continue
			}
			metric := r.Metric
			if _, ok := out[metric]; !ok {
				out[metric] = series{"_all": make([]int64, days)}
			}
			out[metric]["_all"][idx] += r.Count
			if r.Model != nil && *r.Model != "" {
				if _, ok := out[metric][*r.Model]; !ok {
					out[metric][*r.Model] = make([]int64, days)
				}
				out[metric][*r.Model][idx] += r.Count
			}
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"days": axis, "metrics": out}})
	})

	// Prometheus-style metrics (plain text). Keep simple for now.
	app.Get("/metrics", func(c *fiber.Ctx) error {
		snap := metrics.snapshot()
		c.Type("text")
		b := &strings.Builder{}
		b.WriteString("# HELP berjis_ai_total_chats Total chat requests\n")
		b.WriteString("# TYPE berjis_ai_total_chats counter\n")
		b.WriteString(fmt.Sprintf("berjis_ai_total_chats %d\n", snap.Counters.TotalChats))
		b.WriteString("# HELP berjis_ai_blocked_prompts Blocked prompts by safety policy\n")
		b.WriteString("# TYPE berjis_ai_blocked_prompts counter\n")
		b.WriteString(fmt.Sprintf("berjis_ai_blocked_prompts %d\n", snap.Counters.BlockedPrompts))
		b.WriteString("# HELP berjis_ai_total_completions Total completion requests\n")
		b.WriteString("# TYPE berjis_ai_total_completions counter\n")
		b.WriteString(fmt.Sprintf("berjis_ai_total_completions %d\n", snap.Counters.TotalCompletions))
		b.WriteString("# HELP berjis_ai_total_embeddings Total embeddings requests\n")
		b.WriteString("# TYPE berjis_ai_total_embeddings counter\n")
		b.WriteString(fmt.Sprintf("berjis_ai_total_embeddings %d\n", snap.Counters.TotalEmbeddings))
		b.WriteString("# HELP berjis_ai_errors Total gateway errors\n")
		b.WriteString("# TYPE berjis_ai_errors counter\n")
		b.WriteString(fmt.Sprintf("berjis_ai_errors %d\n", snap.Counters.Errors))
		for m, v := range snap.Models {
			b.WriteString(fmt.Sprintf("berjis_ai_requests_by_model{model=\"%s\"} %d\n", m, v))
		}
		return c.SendString(b.String())
	})

	return app
}
