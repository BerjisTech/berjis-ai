package server

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	db "github.com/berjistech/berjis-ecosystem/ai/service/internal/db"
	"github.com/berjistech/berjis-ecosystem/ai/service/internal/inference"
)

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

// helper: map model tags -> names for JSON responses
func modelNames(models []struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Digest   string `json:"digest"`
	Modified string `json:"modified"`
}) []string {
	out := make([]string, 0, len(models))
	for _, m := range models {
		out = append(out, m.Name)
	}
	return out
}

// ── RAG helpers ──────────────────────────────────────────

// chunkText splits text into overlapping chunks of roughly maxChars.
func chunkText(text string, maxChars, overlap int) []string {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return nil
	}
	if len(text) <= maxChars {
		return []string{text}
	}
	var chunks []string
	for i := 0; i < len(text); {
		end := i + maxChars
		if end > len(text) {
			end = len(text)
		}
		// Try to break on a sentence/paragraph boundary
		if end < len(text) {
			for j := end; j > i+maxChars/2; j-- {
				if text[j] == '.' || text[j] == '\n' || text[j] == '!' || text[j] == '?' {
					end = j + 1
					break
				}
			}
		}
		chunks = append(chunks, strings.TrimSpace(text[i:end]))
		i = end - overlap
		if i < 0 {
			i = 0
		}
		if end >= len(text) {
			break
		}
	}
	return chunks
}

// cosineSimilarity computes cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// RagSearchResult is a ranked search result from the knowledge base.
type RagSearchResult struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Source  string  `json:"source"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// ragSearch finds the top-K most relevant chunks for a query.
func ragSearch(ollama *inference.Ollama, store *db.Store, ownerID, query string, topK int) ([]RagSearchResult, error) {
	// Embed the query
	vecs, err := ollama.Embed("nomic-embed-text", []string{query})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil, nil
	}
	queryVec := vecs[0]

	// Load all chunks with embeddings
	chunks, err := store.AllRagChunksWithEmbeddings(ownerID)
	if err != nil {
		return nil, err
	}

	// Score each chunk by cosine similarity
	type scored struct {
		chunk db.RagChunkWithEmbedding
		score float64
	}
	var scoredChunks []scored
	for _, chunk := range chunks {
		var emb []float32
		if err := json.Unmarshal([]byte(chunk.Embedding), &emb); err != nil {
			continue
		}
		sim := cosineSimilarity(queryVec, emb)
		scoredChunks = append(scoredChunks, scored{chunk, sim})
	}

	// Sort by score descending
	sort.Slice(scoredChunks, func(i, j int) bool {
		return scoredChunks[i].score > scoredChunks[j].score
	})

	// Take top K
	if topK > len(scoredChunks) {
		topK = len(scoredChunks)
	}
	results := make([]RagSearchResult, 0, topK)
	for i := 0; i < topK; i++ {
		sc := scoredChunks[i]
		if sc.score < 0.1 {
			break // Skip very low relevance
		}
		results = append(results, RagSearchResult{
			ID:      sc.chunk.ID,
			Title:   sc.chunk.Title,
			Source:  sc.chunk.Source,
			Content: sc.chunk.Content,
			Score:   sc.score,
		})
	}
	return results, nil
}
