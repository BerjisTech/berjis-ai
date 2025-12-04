package config

import (
	"os"
)

type Config struct {
	Env            string
	Port           string
	AllowedOrigins string
	// Core API base URL (for token verification via /v1/auth/verify)
	CoreAPIBase string
	// Ollama base URL (e.g. http://ollama:11434)
	OllamaBase string
	// DefaultModel used when client omits model or when requested model is unavailable
	DefaultModel string
	// SafetyStrict toggles hard blocking of sensitive prompts
	SafetyStrict bool
	// SearchProxyKey authorizes internal services (like the public search summary bridge) to call AI without user tokens.
	SearchProxyKey string
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() Config {
	return Config{
		Env:  getenv("APP_ENV", "development"),
		Port: getenv("PORT", "8094"),
		// Explicit origins by default to allow credentials; browsers block '*' with cookies.
		AllowedOrigins: getenv("ALLOWED_ORIGINS", "https://ai.berjis.tech,https://berjis.tech,http://localhost:5600,http://localhost:4200"),
		CoreAPIBase:    getenv("CORE_API_BASE", "http://api:8080"),
		OllamaBase:     getenv("OLLAMA_BASE", "http://ollama:11434"),
		DefaultModel:   getenv("DEFAULT_MODEL", "llama3.1:8b"),
		SafetyStrict:   getenv("AI_SAFETY_STRICT", "true") == "true",
		SearchProxyKey: getenv("SEARCH_PROXY_KEY", ""),
	}
}
