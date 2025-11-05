# AI Service Integration for Berjis Ecosystem

## **Product Description**

Build a self-hosted AI inference service (`ai.berjis.tech`) that provides LLM capabilities across the entire Berjis ecosystem: landing, logistics, file-management (docs/sheets/notes/slides/pdf), games (conquer), books, schools, communities, cribs, architect, and the upcoming Shopify-style marketplace.

### **Core Requirements**

**Service Name**: `ai`  
**Repository**: `https://github.com/BerjisTech/berjis-ai.git`  
**Domain**: `ai.berjis.tech`  
**Tech Stack**: 
- Ollama or vLLM for model serving
- Go API wrapper (similar to other services)
- ChromaDB or Qdrant for vector storage (RAG)
- Redis for request queuing/rate limiting

---

## **Architecture Specifications**

### **1. Docker Container Structure**

```
berjis-ecosystem/
└─ ai/
   ├─ service/
   │  ├─ cmd/service/
   │  │  └─ main.go              # Go API wrapper
   │  ├─ internal/
   │  │  ├─ inference/            # Ollama client
   │  │  ├─ rag/                  # Vector DB integration
   │  │  ├─ context/              # App-specific context handlers
   │  │  └─ auth/                 # Shared API auth integration
   │  ├─ migrations/              # Vector DB schemas
   │  ├─ Dockerfile
   │  └─ go.mod
   ├─ models/
   │  └─ .gitkeep                 # Models downloaded at runtime
   └─ docker-compose.override.yml # AI service config
```

### **2. API Endpoints**

**Base URL**: `https://ai.berjis.tech`

```
POST   /v1/chat              # General chat completion
POST   /v1/completion        # Text completion
POST   /v1/embeddings        # Generate embeddings for RAG
POST   /v1/search            # RAG-augmented search
GET    /v1/models            # List available models
POST   /v1/fine-tune         # Queue fine-tuning job (optional)
GET    /health               # Health check
```

### **3. Request Format**

```json
{
  "model": "llama3.1:8b",
  "messages": [
    {"role": "system", "content": "You are an assistant for Berjis ecosystem"},
    {"role": "user", "content": "Help me create a logistics route"}
  ],
  "context": {
    "app": "logistics",
    "user_id": "uuid-here",
    "session_id": "session-uuid"
  },
  "rag_enabled": true,
  "max_tokens": 500,
  "temperature": 0.7
}
```

### **4. App-Specific Context Handlers**

Each app should inject context into AI requests:

- **logistics**: Route data, warehouse inventory, shipping records
- **docs/sheets/notes/slides**: Document content, templates, user writing style
- **pdf**: Document text, annotations, search queries
- **conquer**: Game state, player strategies, world map data
- **books**: Book catalog, reading history, recommendations
- **schools**: Curriculum, student data, grading rubrics
- **communities**: Thread context, user reputation, community rules
- **cribs**: Property listings, booking history, pricing models
- **architect**: Design specs, component libraries, project requirements
- **marketplace**: Product catalog, user reviews, purchase history

---

## **Setup Checklist for Claude Code / Codex**

### **Phase 1: Infrastructure Setup**

- [ ] Create `ai/service/` directory in monorepo
- [ ] Write Go API wrapper with Gin/Echo framework
- [ ] Create Dockerfile with multi-stage build:
  - Stage 1: Ollama base image
  - Stage 2: Go API layer
- [ ] Add `ai` service to `docker-compose.yml`:
  ```yaml
  ai:
    build: ./ai/service
    ports:
      - "11434:11434"  # Ollama default
      - "8080:8080"    # Go API
    volumes:
      - ai-models:/root/.ollama
    environment:
      - OLLAMA_HOST=0.0.0.0
      - API_AUTH_URL=http://api:8000
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
  ```
- [ ] Configure `edge/nginx.conf` for `ai.berjis.tech` subdomain

### **Phase 2: Model Management**

- [ ] Install Ollama in container: `curl -fsSL https://ollama.com/install.sh | sh`
- [ ] Download recommended models:
  ```bash
  ollama pull llama3.1:8b-instruct-q4_K_M  # 4.9GB
  ollama pull mistral:7b-instruct-q4_K_M    # 4.1GB
  ollama pull codellama:13b-instruct-q4_K_M # 7.4GB (code tasks)
  ```
- [ ] Implement model switching endpoint in Go API
- [ ] Add model health checks and auto-restart on OOM

### **Phase 3: RAG Integration**

- [ ] Add ChromaDB or Qdrant container:
  ```yaml
  vectordb:
    image: chromadb/chroma:latest
    ports:
      - "8001:8000"
    volumes:
      - vectordb-data:/chroma/chroma
  ```
- [ ] Implement embedding pipeline:
  - Use `nomic-embed-text` model for embeddings
  - Chunk documents into 512-token segments
  - Store in vector DB with metadata (app, user_id, timestamp)
- [ ] Create indexing endpoints:
  ```go
  POST /v1/index/document    # Index new content
  POST /v1/index/batch       # Bulk indexing
  DELETE /v1/index/:id       # Remove from index
  ```

### **Phase 4: Authentication & Authorization**

- [ ] Integrate with `api` service for JWT validation
- [ ] Implement rate limiting per user/app:
  - Free tier: 10 requests/min
  - Pro tier: 100 requests/min
  - Enterprise: Unlimited
- [ ] Add request queuing with Redis
- [ ] Log all AI interactions for audit/training

### **Phase 5: App-Specific Integrations**

For each app, implement:

- [ ] **landing**: 
  - Chat widget in bottom-right corner
  - Help with navigation, app discovery
  
- [ ] **logistics**: 
  - Route optimization suggestions
  - Inventory forecasting
  - Natural language query for shipments

- [ ] **docs/sheets/notes**: 
  - Auto-complete sentences
  - Grammar/style suggestions
  - Document summarization

- [ ] **conquer**: 
  - Strategy recommendations
  - NPC dialogue generation
  - Battle outcome predictions

- [ ] **marketplace**: 
  - Product description generation
  - Personalized recommendations
  - Customer support chatbot
  - Vendor onboarding assistant

- [ ] **books**: 
  - Book recommendations
  - Reading comprehension Q&A
  - Author style analysis

- [ ] **schools**: 
  - Grading assistance
  - Lesson plan generation
  - Student query answering

- [ ] **communities**: 
  - Content moderation
  - Thread summarization
  - Auto-reply suggestions

- [ ] **cribs**: 
  - Property description enhancement
  - Pricing suggestions
  - Guest inquiry responses

- [ ] **architect**: 
  - Design pattern suggestions
  - Code generation from specs
  - Architecture review

### **Phase 6: Fine-Tuning & Optimization**

- [ ] Collect interaction logs for 2-4 weeks
- [ ] Create fine-tuning dataset:
  - Successful queries → Include
  - Corrected responses → High priority
  - Domain-specific terminology → Emphasize
- [ ] Use LoRA fine-tuning (Low-Rank Adaptation):
  ```bash
  # Example with Ollama
  ollama create berjis-custom -f Modelfile
  ```
- [ ] Benchmark performance:
  - Latency: Target <2s for 100-token responses
  - Throughput: 4-8 concurrent users
  - GPU usage: <90% sustained

### **Phase 7: Monitoring & Scaling**

- [ ] Add Prometheus metrics:
  - Request latency
  - Model load time
  - GPU memory usage
  - Token throughput
- [ ] Set up Grafana dashboards
- [ ] Implement horizontal scaling:
  - Use load balancer for multiple replicas
  - Share vector DB across instances
  - Session affinity for context continuity
- [ ] Cache frequent queries in Redis (TTL: 1 hour)

---

## **Resource Estimates**

| Component | Storage | RAM | GPU VRAM |
|-----------|---------|-----|----------|
| Ollama + Models | 15-30GB | 8-16GB | 6-10GB |
| Vector DB | 5-50GB | 4-8GB | N/A |
| Go API | <1GB | 2-4GB | N/A |
| Redis Cache | 1-5GB | 2GB | N/A |
| **Total** | **25-85GB** | **16-30GB** | **6-10GB** |

**Your PC can handle this comfortably.**

---

## **Sample Integration Code**

### **Go API Client (for frontends)**

```typescript
// clients/typescript/src/ai-client.ts
export class BerjisAI {
  constructor(
    private baseUrl = 'https://ai.berjis.tech',
    private authToken: string
  ) {}

  async chat(
    messages: Array<{role: string, content: string}>,
    context: {app: string, userId: string}
  ) {
    const response = await fetch(`${this.baseUrl}/v1/chat`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${this.authToken}`
      },
      body: JSON.stringify({
        model: 'llama3.1:8b',
        messages,
        context,
        rag_enabled: true,
        max_tokens: 500,
        temperature: 0.7
      })
    });
    return response.json();
  }

  async search(query: string, app: string) {
    const response = await fetch(`${this.baseUrl}/v1/search`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${this.authToken}`
      },
      body: JSON.stringify({query, app})
    });
    return response.json();
  }
}
```

### **Go Service Skeleton**

```go
// ai/service/cmd/service/main.go
package main

import (
    "github.com/gin-gonic/gin"
    "ai/internal/inference"
    "ai/internal/rag"
    "ai/internal/auth"
)

func main() {
    r := gin.Default()
    
    // Middleware
    r.Use(auth.ValidateJWT())
    r.Use(auth.RateLimit())
    
    // Inference endpoints
    r.POST("/v1/chat", inference.HandleChat)
    r.POST("/v1/completion", inference.HandleCompletion)
    r.POST("/v1/embeddings", inference.HandleEmbeddings)
    
    // RAG endpoints
    r.POST("/v1/search", rag.HandleSearch)
    r.POST("/v1/index/document", rag.HandleIndex)
    
    // Admin
    r.GET("/v1/models", inference.ListModels)
    r.GET("/health", func(c *gin.Context) {
        c.JSON(200, gin.H{"status": "healthy"})
    })
    
    r.Run(":8080")
}
```

---

## **Timeline Estimate**

- **Week 1**: Docker setup, Ollama installation, basic API
- **Week 2**: RAG integration, vector DB setup
- **Week 3**: App integrations (3-4 apps)
- **Week 4**: Remaining apps, testing, optimization
- **Week 5+**: Fine-tuning, monitoring, scaling

---

## **Cost Analysis (vs Cloud)**

| Provider | Cost/Month | Limitations |
|----------|------------|-------------|
| OpenAI GPT-4 | $500-2000 | API costs, rate limits |
| Anthropic Claude | $400-1800 | API costs, rate limits |
| **Your Local Setup** | **$0** (electricity: ~$15) | Hardware-bound, no API fees |

**ROI**: Pays for itself in 1-2 months if replacing cloud APIs.

---

## **Next Steps**

1. Confirm this architecture aligns with your vision
2. Run setup script to create `ai/` directory structure
3. Start with Phase 1 (infrastructure)
4. Test with one app (recommend starting with `landing` or `marketplace`)
5. Iterate based on performance metrics

**Questions to Answer**:
- Which apps should get AI features first?
- Do you want multimodal support (images, audio)?
- Should the AI have "memory" across sessions?
- Privacy requirements for storing user data in RAG?

Let me know if you want me to generate the actual Go code, Dockerfiles, or Angular integration components!