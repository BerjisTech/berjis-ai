package server

import (
    "net/http"
    "time"
    "strings"
    "log"
    "fmt"

    aauth "github.com/berjistech/berjis-ecosystem/ai/service/internal/auth"
    "github.com/berjistech/berjis-ecosystem/ai/service/internal/config"
    "github.com/berjistech/berjis-ecosystem/ai/service/internal/inference"
    "github.com/berjistech/berjis-ecosystem/ai/service/internal/policy"
    db "github.com/berjistech/berjis-ecosystem/ai/service/internal/db"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/gofiber/fiber/v2/middleware/limiter"
)

type Options struct {
    Config config.Config
    DB     *db.DB
}

func New(opts Options) *fiber.App {
    app := fiber.New()
    // Reflect CORS for *.berjis.tech and localhost before stricter CORS below,
    // to avoid edge proxies dropping headers when origin lists drift.
    app.Use(func(c *fiber.Ctx) error {
        origin := c.Get("Origin")
        if origin != "" && (strings.HasSuffix(origin, ".berjis.tech") || strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "https://localhost")) {
            c.Set("Access-Control-Allow-Origin", origin)
            c.Set("Vary", "Origin")
            c.Set("Access-Control-Allow-Credentials", "true")
            c.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,Accept,X-Requested-With")
            c.Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
            if c.Method() == fiber.MethodOptions {
                return c.SendStatus(fiber.StatusNoContent)
            }
        }
        return c.Next()
    })
    app.Use(cors.New(cors.Config{
        AllowOrigins:     opts.Config.AllowedOrigins,
        AllowMethods:     "GET,POST,OPTIONS",
        AllowHeaders:     "Authorization,Content-Type,Accept,X-Requested-With",
        AllowCredentials: true,
    }))

    app.Get("/v1/health", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{"success": true, "message": "ok"})
    })

    // Root info to avoid 404 on '/' when probed via Cloudflare
    app.Get("/", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{"success": true, "service": "berjis-ai", "endpoints": []string{"/v1/health","/v1/models","/v1/chat","/v1/completion","/v1/embeddings"}})
    })

    app.Use("/v1/*", limiter.New(limiter.Config{Max: 120, Expiration: 1 * time.Minute}))

    httpClient := &http.Client{Timeout: 60 * time.Second}
    verify := aauth.RequireAuth(aauth.Options{CoreAPIBase: opts.Config.CoreAPIBase, HTTP: httpClient})
    ollama := inference.NewOllama(opts.Config.OllamaBase, httpClient)
    metrics := newMetrics()
    var store *db.Store
    if opts.DB != nil {
        store = db.NewStore(opts.DB)
    }

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
        if body.Model == "" || len(body.Messages) == 0 {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "model and messages required"})
        }
        // Convert to Ollama request with safety + public context
        sys := policy.BuildSystem()
        msgs := make([]inference.ChatMessage, 0, len(sys)+len(body.Messages))
        msgs = append(msgs, sys...)
        for _, m := range body.Messages {
            msgs = append(msgs, inference.ChatMessage{Role: m.Role, Content: m.Content})
        }
        // Lightweight guardrail: refuse obviously sensitive developer/internal requests
        userBlob := ""
        for _, m := range body.Messages { if m.Role == "user" { userBlob += "\n" + m.Content } }
        if opts.Config.SafetyStrict && policy.IsSensitiveText(userBlob) {
            metrics.incBlocked()
            kw := policy.MatchedKeyword(userBlob)
            uid, _ := c.Locals("uuid").(string)
            log.Printf("policy: blocked sensitive prompt (uuid=%s keyword=%s)", uid, kw)
            safe := "I can't share developer or internal details. Here's a public overview of Berjis and how to get started at berjis.tech."
            return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"model": body.Model, "message": fiber.Map{"role": "assistant", "content": safe}}})
        }
        res, err := ollama.Chat(inference.ChatRequest{Model: body.Model, Messages: msgs, Temperature: body.Temperature})
        if err != nil {
            metrics.incError()
            return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"success": false, "message": err.Error()})
        }
        metrics.incChat(body.Model)
        if store != nil { _ = store.IncDaily("chats", body.Model, 1) }
        return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"model": res.Model, "message": res.Message}})
    })

    app.Post("/v1/completion", verify, func(c *fiber.Ctx) error {
        var body CompletionRequest
        if err := c.BodyParser(&body); err != nil {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "invalid body"})
        }
        if body.Model == "" || strings.TrimSpace(body.Prompt) == "" {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "model and prompt required"})
        }
        text, model, err := ollama.Generate(body.Model, body.Prompt, body.MaxTokens)
        if err != nil {
            metrics.incError()
            return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"success": false, "message": err.Error()})
        }
        metrics.incCompletion(body.Model)
        if store != nil { _ = store.IncDaily("completions", body.Model, 1) }
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
            return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"success": false, "message": err.Error()})
        }
        metrics.incEmbeddings()
        if store != nil { _ = store.IncDaily("embeddings", "", 1) }
        return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"model": "nomic-embed-text", "embeddings": vecs}})
    })

    // Admin metrics endpoint; requires admin via Core API
    app.Get("/v1/admin/metrics", verify, func(c *fiber.Ctx) error {
        token := c.Get("Authorization")
        req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(opts.Config.CoreAPIBase, "/")+"/v1/auth/admin/verify", http.NoBody)
        if token != "" { req.Header.Set("Authorization", token) }
        if token == "" {
            if ck := c.Get("Cookie"); ck != "" {
                req.Header.Set("Cookie", ck)
            }
        }
        resp, err := httpClient.Do(req)
        if err != nil { return c.Status(502).JSON(fiber.Map{"success": false, "message": "admin verify failed"}) }
        defer resp.Body.Close()
        if resp.StatusCode != 200 { return c.Status(403).JSON(fiber.Map{"success": false, "message": "forbidden"}) }
        snap := metrics.snapshot()
        return c.JSON(fiber.Map{"success": true, "data": snap})
    })

    // Admin metrics daily trend
    app.Get("/v1/admin/metrics/daily", verify, func(c *fiber.Ctx) error {
        if store == nil { return c.Status(503).JSON(fiber.Map{"success": false, "message": "metrics store unavailable"}) }
        token := c.Get("Authorization")
        req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(opts.Config.CoreAPIBase, "/")+"/v1/auth/admin/verify", http.NoBody)
        if token != "" { req.Header.Set("Authorization", token) }
        if token == "" { if ck := c.Get("Cookie"); ck != "" { req.Header.Set("Cookie", ck) } }
        resp, err := httpClient.Do(req)
        if err != nil { return c.Status(502).JSON(fiber.Map{"success": false, "message": "admin verify failed"}) }
        defer resp.Body.Close()
        if resp.StatusCode != 200 { return c.Status(403).JSON(fiber.Map{"success": false, "message": "forbidden"}) }

        days := 7
        if v := strings.TrimSpace(c.Query("days")); v == "30" { days = 30 }
        rows, err := store.GetDaily(days)
        if err != nil { return c.Status(500).JSON(fiber.Map{"success": false, "message": "db error"}) }

        // Build day axis
        axis := make([]string, days)
        dayIndex := map[string]int{}
        for i := 0; i < days; i++ {
            d := time.Now().AddDate(0,0, -(days-1-i)).Format("2006-01-02")
            axis[i] = d
            dayIndex[d] = i
        }
        type series map[string][]int64 // model -> points
        out := map[string]series{}
        for _, m := range []string{"chats","completions","embeddings"} {
            out[m] = series{"_all": make([]int64, days)}
        }
        // Fill
        for _, r := range rows {
            d := r.Day.Format("2006-01-02")
            idx, ok := dayIndex[d]; if !ok { continue }
            metric := r.Metric
            if _, ok := out[metric]; !ok { out[metric] = series{"_all": make([]int64, days)} }
            out[metric]["_all"][idx] += r.Count
            if r.Model != nil && *r.Model != "" {
                if _, ok := out[metric][*r.Model]; !ok { out[metric][*r.Model] = make([]int64, days) }
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
