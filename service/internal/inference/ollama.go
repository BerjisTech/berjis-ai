package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type Ollama struct {
	BaseURL string
	HTTP    *http.Client
}

func NewOllama(base string, httpClient *http.Client) *Ollama {
	c := httpClient
	if c == nil {
		c = &http.Client{Timeout: 60 * time.Second}
	}
	return &Ollama{BaseURL: strings.TrimRight(base, "/"), HTTP: c}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string         `json:"model"`
	Messages    []ChatMessage  `json:"messages"`
	Temperature float32        `json:"temperature,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
	// Important: do NOT omit 'stream' when false; Ollama defaults to streaming
	Stream bool `json:"stream"`
	// KeepAlive keeps the model loaded in memory for faster subsequent calls (e.g. "30m").
	KeepAlive any `json:"keep_alive,omitempty"`
}

type ChatResponse struct {
	Model   string `json:"model"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// ChatStream calls Ollama /api/chat with streaming enabled and invokes onChunk
// for each JSON event until done or context cancellation. It returns the first
// error encountered (other than io.EOF), or nil on clean completion.
func (o *Ollama) ChatStream(ctx context.Context, req ChatRequest, onChunk func(ChatResponse) error) error {
	req.Stream = true
	body, _ := json.Marshal(req)
	url := o.BaseURL + "/api/chat"
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ollama error: %s: %s", resp.Status, string(b))
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var ev ChatResponse
		if err := dec.Decode(&ev); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if err := onChunk(ev); err != nil {
			return err
		}
		if ev.Done {
			return nil
		}
	}
}

func (o *Ollama) Chat(req ChatRequest) (ChatResponse, error) {
	body, _ := json.Marshal(req)
	url := o.BaseURL + "/api/chat"
	httpReq, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(httpReq)
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return ChatResponse{}, fmt.Errorf("ollama error: %s: %s", resp.Status, string(b))
	}
	var out ChatResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&out); err != nil {
		return ChatResponse{}, err
	}
	return out, nil
}

type TagsResponse struct {
	Models []struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		Digest   string `json:"digest"`
		Modified string `json:"modified"`
	} `json:"models"`
}

func (o *Ollama) Models() (TagsResponse, error) {
	url := o.BaseURL + "/api/tags"
	resp, err := o.HTTP.Get(url)
	if err != nil {
		return TagsResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return TagsResponse{}, fmt.Errorf("ollama tags error: %s: %s", resp.Status, string(b))
	}
	var out TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return TagsResponse{}, err
	}
	return out, nil
}

// Text completion (generate)
type generateRequest struct {
	Model     string         `json:"model"`
	Prompt    string         `json:"prompt"`
	Stream    bool           `json:"stream"`
	Options   map[string]any `json:"options,omitempty"`
	KeepAlive any            `json:"keep_alive,omitempty"`
}

type generateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (o *Ollama) Generate(model, prompt string, maxTokens int) (string, string, error) {
	req := generateRequest{Model: model, Prompt: prompt, Stream: false}
	// Prefer deterministic summaries; default to a low temperature
	req.Options = map[string]any{"temperature": 0.1}
	if maxTokens > 0 {
		req.Options["num_predict"] = maxTokens
	}
	// Encourage faster CPU inference and keep model warm
	req.Options["num_thread"] = runtimeNumCPU()
	req.Options["num_batch"] = 256
	req.KeepAlive = "30m"
	body, _ := json.Marshal(req)
	url := o.BaseURL + "/api/generate"
	httpReq, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(httpReq)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		// Some Ollama builds may not expose /api/generate; gracefully fall back to /api/chat
		if strings.Contains(resp.Status, "404") {
			// emulate generate via chat (explicitly disable streaming)
			cr, err2 := o.Chat(ChatRequest{Model: model, Messages: []ChatMessage{{Role: "user", Content: prompt}}, Stream: false})
			if err2 == nil {
				return cr.Message.Content, cr.Model, nil
			}
		}
		return "", "", fmt.Errorf("ollama generate error: %s: %s", resp.Status, string(b))
	}
	var out generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	return out.Response, out.Model, nil
}

// runtimeNumCPU is a tiny shim to avoid importing runtime at call sites where not already needed.
func runtimeNumCPU() int     { return runtimeNumCPUImpl() }
func runtimeNumCPUImpl() int { return runtime.NumCPU() }

// Embeddings
type embeddingsRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}
type embeddingsResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed returns one embedding per input text by calling Ollama /api/embeddings individually.
func (o *Ollama) Embed(model string, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		body, _ := json.Marshal(embeddingsRequest{Model: model, Prompt: t})
		url := o.BaseURL + "/api/embeddings"
		httpReq, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := o.HTTP.Do(httpReq)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			return nil, fmt.Errorf("ollama embeddings error: %s: %s", resp.Status, string(b))
		}
		var er embeddingsResponse
		if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		out = append(out, er.Embedding)
	}
	return out, nil
}
