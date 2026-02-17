package server

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
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
	coreauth "github.com/berjistech/berjis-ecosystem/shared/coreauth"
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
	var authVerifier *coreauth.Verifier
	if v, err := coreauth.NewVerifier(coreauth.Config{
		CoreAPIBase: opts.Config.CoreAPIBase,
		HTTPClient:  httpClientAuth,
	}); err != nil {
		log.Printf("warn: coreauth verifier init failed: %v", err)
	} else {
		authVerifier = v
	}
	// Allow long cold-starts when the first model loads
	httpClientLLM := &http.Client{Timeout: 90 * time.Second}
	verify := aauth.RequireAuth(aauth.Options{CoreAPIBase: opts.Config.CoreAPIBase, HTTP: httpClientAuth, Verifier: authVerifier})
	ollama := inference.NewOllama(opts.Config.OllamaBase, httpClientLLM)
	metrics := newMetrics()
	var store *db.Store
	if opts.DB != nil {
		store = db.NewStore(opts.DB)
	}
	searchKey := strings.TrimSpace(opts.Config.SearchProxyKey)
	requireSearchKey := func(c *fiber.Ctx) error {
		if searchKey == "" {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"success": false, "message": "search bridge not configured"})
		}
		key := strings.TrimSpace(c.Get("X-Berjis-Ai-Key"))
		if key == "" {
			key = strings.TrimSpace(c.Get("X-Internal-Key"))
		}
		if key == "" || subtle.ConstantTimeCompare([]byte(key), []byte(searchKey)) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "message": "unauthorized"})
		}
		return c.Next()
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

	// Streaming chat via SSE (works better through proxies like Cloudflare/Nginx).
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
			c.Set("Content-Type", "text/event-stream")
			c.Set("Cache-Control", "no-cache")
			c.Set("Connection", "keep-alive")
			c.Set("X-Accel-Buffering", "no")
			c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
				safe := "I can't share developer or internal details. Here's a public overview of Berjis and how to get started at berjis.tech."
				type msg struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}
				type ev struct {
					Model   string `json:"model"`
					Message msg    `json:"message"`
				}
				e := ev{Model: body.Model}
				e.Message.Role = "assistant"
				e.Message.Content = safe
				b, _ := json.Marshal(e)
				fmt.Fprintf(w, "event: token\n")
				fmt.Fprintf(w, "data: %s\n\n", string(b))
				fmt.Fprintf(w, "event: done\n")
				fmt.Fprintf(w, "data: {}\n\n")
				_ = w.Flush()
			})
			return nil
		}
		temp := body.Temperature
		if temp <= 0 {
			temp = 0.2
		}
		optsMap := map[string]any{"num_predict": 256, "repeat_penalty": 1.07, "top_k": 50, "top_p": 0.9, "num_thread": runtime.NumCPU(), "num_batch": 256}
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			_ = ollama.ChatStream(c.Context(), inference.ChatRequest{Model: body.Model, Messages: msgs, Temperature: temp, Stream: true, Options: optsMap, KeepAlive: "30m"}, func(ch inference.ChatResponse) error {
				type msg2 struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}
				type ev struct {
					Model   string `json:"model"`
					Message msg2   `json:"message"`
					Done    bool   `json:"done"`
				}
				e := ev{Model: ch.Model, Done: ch.Done}
				e.Message.Role = ch.Message.Role
				e.Message.Content = ch.Message.Content
				b, _ := json.Marshal(e)
				fmt.Fprintf(w, "event: token\n")
				fmt.Fprintf(w, "data: %s\n\n", string(b))
				if ch.Done {
					fmt.Fprintf(w, "event: done\n")
					fmt.Fprintf(w, "data: {}\n\n")
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

	// Internal bridge for public search summaries (no end-user auth, guarded by shared key)
	app.Post("/v1/search/summary", requireSearchKey, func(c *fiber.Ctx) error {
		var body struct {
			Model     string `json:"model"`
			Prompt    string `json:"prompt"`
			MaxTokens int    `json:"maxTokens"`
			Query     string `json:"query"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "invalid body"})
		}
		prompt := strings.TrimSpace(body.Prompt)
		if prompt == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "prompt required"})
		}
		if len(prompt) > 4000 {
			runes := []rune(prompt)
			if len(runes) > 4000 {
				prompt = string(runes[:4000])
			}
		}
		query := strings.TrimSpace(body.Query)
		model := strings.TrimSpace(body.Model)
		if model == "" {
			model = opts.Config.DefaultModel
		}
		if opts.Config.SafetyStrict && query != "" && policy.IsSensitiveText(query) {
			metrics.incBlocked()
			safe := "I can't share developer or internal details. Here's a public overview of Berjis and how to get started at berjis.tech."
			return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"model": model, "text": safe}})
		}
		maxTokens := body.MaxTokens
		if maxTokens <= 0 || maxTokens > 400 {
			maxTokens = 220
		}
		text, usedModel, err := ollama.Generate(model, prompt, maxTokens)
		if err != nil {
			metrics.incError()
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "not found") {
				if model != opts.Config.DefaultModel {
					if text2, model2, err2 := ollama.Generate(opts.Config.DefaultModel, prompt, maxTokens); err2 == nil {
						text = text2
						usedModel = model2
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
		if strings.TrimSpace(usedModel) == "" {
			usedModel = model
		}
		metrics.incCompletion(usedModel)
		if store != nil {
			_ = store.IncDaily("completions", usedModel, 1)
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"model": usedModel, "text": text}})
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

	// ── RAG: Document Ingestion & Retrieval-Augmented Chat ──

	// Ingest a document: chunk it, embed each chunk, store in DB
	app.Post("/v1/documents", verify, func(c *fiber.Ctx) error {
		if store == nil {
			return c.Status(503).JSON(fiber.Map{"success": false, "message": "database unavailable"})
		}
		uid := c.Get("X-Berjis-Uid")
		if uid == "" {
			uid = "anonymous"
		}
		var body struct {
			Title   string `json:"title"`
			Source  string `json:"source"`
			Content string `json:"content"`
		}
		if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Content) == "" {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "content required"})
		}
		if body.Title == "" {
			body.Title = "Untitled"
		}

		// Chunk the text (~500 chars per chunk with overlap)
		chunks := chunkText(body.Content, 500, 50)
		if len(chunks) == 0 {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "content too short"})
		}

		// Embed all chunks
		embeddings, err := ollama.Embed("nomic-embed-text", chunks)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "embedding failed: " + err.Error()})
		}

		// Store chunks with embeddings
		ids := make([]string, 0, len(chunks))
		for i, chunk := range chunks {
			var emb []float32
			if i < len(embeddings) {
				emb = embeddings[i]
			}
			id, err := store.InsertRagChunk(uid, body.Title, body.Source, i, chunk, emb)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"success": false, "message": "store failed"})
			}
			ids = append(ids, id)
		}

		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"chunks": len(chunks), "ids": ids}})
	})

	// List user's documents
	app.Get("/v1/documents", verify, func(c *fiber.Ctx) error {
		if store == nil {
			return c.Status(503).JSON(fiber.Map{"success": false, "message": "database unavailable"})
		}
		uid := c.Get("X-Berjis-Uid")
		if uid == "" {
			uid = "anonymous"
		}
		docs, err := store.ListRagDocuments(uid)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false})
		}
		return c.JSON(fiber.Map{"success": true, "data": docs})
	})

	// Delete a document chunk or all chunks by title
	app.Delete("/v1/documents/:id", verify, func(c *fiber.Ctx) error {
		if store == nil {
			return c.Status(503).JSON(fiber.Map{"success": false, "message": "database unavailable"})
		}
		uid := c.Get("X-Berjis-Uid")
		if uid == "" {
			uid = "anonymous"
		}
		docID := c.Params("id")
		if err := store.DeleteRagDocument(docID, uid); err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false})
		}
		return c.JSON(fiber.Map{"success": true})
	})

	// RAG search: find relevant chunks for a query
	app.Post("/v1/search", verify, func(c *fiber.Ctx) error {
		if store == nil {
			return c.Status(503).JSON(fiber.Map{"success": false, "message": "database unavailable"})
		}
		uid := c.Get("X-Berjis-Uid")
		if uid == "" {
			uid = "anonymous"
		}
		var body struct {
			Query string `json:"query"`
			TopK  int    `json:"topK"`
		}
		if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Query) == "" {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "query required"})
		}
		if body.TopK <= 0 || body.TopK > 20 {
			body.TopK = 5
		}

		results, err := ragSearch(ollama, store, uid, body.Query, body.TopK)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": results})
	})

	// RAG-enhanced chat: retrieve context then answer
	app.Post("/v1/chat/rag", verify, func(c *fiber.Ctx) error {
		if store == nil {
			return c.Status(503).JSON(fiber.Map{"success": false, "message": "database unavailable"})
		}
		uid := c.Get("X-Berjis-Uid")
		if uid == "" {
			uid = "anonymous"
		}
		var body struct {
			Model    string `json:"model"`
			Question string `json:"question"`
			TopK     int    `json:"topK"`
		}
		if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Question) == "" {
			return c.Status(400).JSON(fiber.Map{"success": false, "message": "question required"})
		}
		if body.TopK <= 0 || body.TopK > 10 {
			body.TopK = 3
		}
		model := body.Model
		if model == "" {
			model = opts.Config.DefaultModel
		}

		// Safety check
		if opts.Config.SafetyStrict && policy.IsSensitiveText(body.Question) {
			metrics.incBlocked()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "This question relates to sensitive internal topics and cannot be answered."})
		}

		// Retrieve relevant chunks
		results, err := ragSearch(ollama, store, uid, body.Question, body.TopK)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "message": "search failed: " + err.Error()})
		}

		// Build context from retrieved chunks
		var contextParts []string
		for _, r := range results {
			contextParts = append(contextParts, fmt.Sprintf("[%s] %s", r.Title, r.Content))
		}
		contextText := strings.Join(contextParts, "\n\n")

		// Build messages with RAG context
		systemMsg := "You are a knowledgeable assistant. Use the following retrieved context to answer the user's question. If the context doesn't contain relevant information, say so and answer based on your general knowledge.\n\n--- Retrieved Context ---\n" + contextText + "\n--- End Context ---"
		messages := []inference.ChatMessage{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: body.Question},
		}

		chatReq := inference.ChatRequest{
			Model:    model,
			Messages: messages,
			Stream:   false,
		}
		resp, err := ollama.Chat(chatReq)
		if err != nil {
			metrics.incError()
			return c.Status(500).JSON(fiber.Map{"success": false, "message": err.Error()})
		}

		metrics.incChat(model)
		if store != nil {
			_ = store.IncDaily("chats", model, 1)
		}

		return c.JSON(fiber.Map{
			"success": true,
			"data": fiber.Map{
				"model":   resp.Model,
				"answer":  resp.Message.Content,
				"sources": results,
			},
		})
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
