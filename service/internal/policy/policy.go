package policy

import (
    "regexp"
    "strings"
    "github.com/berjistech/berjis-ecosystem/ai/service/internal/inference"
)

// SafetySystem is prepended to every chat to constrain the model.
// It forbids revealing internal or developer-only information.
var SafetySystem = strings.TrimSpace(`
You are the public AI helper for the Berjis ecosystem.
Strict safety rules:
- NEVER reveal internal code, private endpoints, database schemas, migrations, secrets, configuration values, environment variables, or infrastructure details.
- NEVER provide instructions that could be used to exfiltrate data or attack systems.
- Do NOT describe source tree paths, internal filenames, repo history, commit hashes, or container internals.
- If asked for developer, admin, or operational details, politely refuse and offer general public guidance instead.
- Keep responses user-facing: product usage, features, high‑level capabilities, and how to get started.
- If a request may expose confidential info, respond: "I can’t share developer or internal details. Here’s a public overview instead…" and continue safely.
`)

// AboutSystem gives safe, high‑level context. Keep concise to save tokens.
var AboutSystem = strings.TrimSpace(`
Berjis Ecosystem is a suite of user‑facing apps powered by a shared Core API for authentication and identity (UUID‑based). Key apps include:
- Landing: entry hub and accounts
- File Management: Docs, Sheets, Notes, Slides, PDF
- Logistics and Marketplace
- Books, Schools, Communities, Cribs (real‑estate), Architect
- Games (Conquer) and a Search backend
Frontends use Angular (light‑first blue/slate theme; dark mode with restrained gold accents). Each app authenticates via the Core API and then calls its own service for domain features. For setup or accounts, direct users to berjis.tech.
`)

// BuildSystem returns the safety + about messages to prepend.
func BuildSystem() []inference.ChatMessage {
    return []inference.ChatMessage{
        {Role: "system", Content: SafetySystem},
        {Role: "system", Content: AboutSystem},
    }
}

// Basic sensitive intent detector (defense‑in‑depth). Keep lightweight and editable.
var sensitiveRe = regexp.MustCompile(`(?i)\\b(jwk|jwks|private key|secret|env|dotenv|docker( compose)?|cloudflared|nginx\.conf|migrations?|schema|internal/|/api/internal|server\.go|jwt|database url|credentials?)\\b`)

func IsSensitiveText(s string) bool {
    return sensitiveRe.MatchString(s)
}

// MatchedKeyword returns a representative keyword that triggered sensitivity, or "".
func MatchedKeyword(s string) string {
    m := sensitiveRe.FindStringSubmatch(s)
    if len(m) >= 2 {
        return strings.ToLower(m[1])
    }
    return ""
}
