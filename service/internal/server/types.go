package server

type ChatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type ChatRequest struct {
    Model       string        `json:"model"`
    Messages    []ChatMessage `json:"messages"`
    Temperature float32       `json:"temperature,omitempty"`
}

type CompletionRequest struct {
    Model     string `json:"model"`
    Prompt    string `json:"prompt"`
    MaxTokens int    `json:"max_tokens,omitempty"`
}

type EmbeddingsRequest struct {
    Texts []string `json:"texts"`
}
