# AI Integration: Next Steps & Implementation Priorities

## **Phase 1: Stabilize Core (This Week)**

### **1. Pull Essential Models**

```bash
# On your host or inside ollama container
docker exec -it ollama ollama pull llama3.1:8b-instruct-q4_K_M    # General (4.9GB)
docker exec -it ollama ollama pull mistral:7b-instruct-q4_K_M     # Fast (4.1GB)
docker exec -it ollama ollama pull codellama:13b-instruct-q4_K_M  # Code (7.4GB)
docker exec -it ollama ollama pull nomic-embed-text               # Embeddings (274MB)
```

**Why these?**
- Llama 3.1: Best general-purpose reasoning
- Mistral: Fastest inference for real-time chat
- CodeLlama: For architect, docs, notes code generation
- Nomic: Required for RAG embeddings later

### **2. Test Your Setup**

```bash
# Get a valid JWT from Core API first
curl -X POST https://api.berjis.tech/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"your@email.com","password":"yourpass"}' \
  -c cookies.txt

# Test models endpoint
curl https://ai-api.berjis.tech/v1/models \
  -H "Authorization: Bearer YOUR_JWT_HERE"

# Test chat
curl -X POST https://ai-api.berjis.tech/v1/chat \
  -H "Authorization: Bearer YOUR_JWT_HERE" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.1:8b",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant for Berjis ecosystem."},
      {"role": "user", "content": "What can you help me with?"}
    ]
  }'
```

### **3. Add Missing Endpoints (Extend Gateway)**

Add to `ai/service/internal/server/server.go`:

```go
// POST /v1/completion - Simple text completion
func (s *Server) handleCompletion(c *fiber.Ctx) error {
    var req CompletionRequest
    if err := c.BodyParser(&req); err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
    }
    
    // Forward to Ollama /api/generate
    resp, err := s.ollama.Generate(req.Model, req.Prompt, req.MaxTokens)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    
    return c.JSON(resp)
}

// POST /v1/embeddings - Generate embeddings for RAG prep
func (s *Server) handleEmbeddings(c *fiber.Ctx) error {
    var req EmbeddingsRequest
    if err := c.BodyParser(&req); err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
    }
    
    embeddings, err := s.ollama.Embed("nomic-embed-text", req.Texts)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    
    return c.JSON(fiber.Map{"embeddings": embeddings})
}
```

Add types to `types.go`:

```go
type CompletionRequest struct {
    Model     string `json:"model"`
    Prompt    string `json:"prompt"`
    MaxTokens int    `json:"max_tokens,omitempty"`
}

type EmbeddingsRequest struct {
    Texts []string `json:"texts"`
}
```

---

## **Phase 2: Frontend Integration (Next 1-2 Weeks)**

### **Option A: Angular Shared Library (Recommended)**

Create `libs/ai-chat/` in your monorepo:

```
libs/
└─ ai-chat/
   ├─ src/
   │  ├─ lib/
   │  │  ├─ ai-chat-panel/
   │  │  │  ├─ ai-chat-panel.component.ts
   │  │  │  ├─ ai-chat-panel.component.html
   │  │  │  └─ ai-chat-panel.component.css
   │  │  ├─ ai-inline-button/
   │  │  │  ├─ ai-inline-button.component.ts
   │  │  │  └─ ai-inline-button.component.html
   │  │  ├─ ai-floating-widget/
   │  │  │  ├─ ai-floating-widget.component.ts
   │  │  │  └─ ai-floating-widget.component.html
   │  │  ├─ services/
   │  │  │  └─ ai.service.ts
   │  │  └─ public-api.ts
   │  └─ index.ts
   ├─ package.json
   └─ README.md
```

**Generate with:**
```bash
cd landing  # or any Angular workspace
ng generate library ai-chat --prefix=berjis
```

**Core Service** (`ai.service.ts`):

```typescript
import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

interface ChatMessage {
  role: 'system' | 'user' | 'assistant';
  content: string;
}

interface ChatRequest {
  model: string;
  messages: ChatMessage[];
  context?: {
    app: string;
    userId?: string;
    resourceId?: string;
  };
}

@Injectable({ providedIn: 'root' })
export class AiService {
  private readonly baseUrl = 'https://ai-api.berjis.tech';

  constructor(private http: HttpClient) {}

  chat(messages: ChatMessage[], context?: any): Observable<any> {
    return this.http.post(`${this.baseUrl}/v1/chat`, {
      model: 'llama3.1:8b',
      messages,
      context
    });
  }

  listModels(): Observable<any> {
    return this.http.get(`${this.baseUrl}/v1/models`);
  }

  complete(prompt: string, model = 'mistral:7b'): Observable<any> {
    return this.http.post(`${this.baseUrl}/v1/completion`, {
      model,
      prompt,
      max_tokens: 500
    });
  }
}
```

**Chat Panel Component** (`ai-chat-panel.component.ts`):

```typescript
import { Component, Input, Output, EventEmitter } from '@angular/core';
import { AiService } from '../services/ai.service';

@Component({
  selector: 'berjis-ai-chat-panel',
  templateUrl: './ai-chat-panel.component.html',
  styleUrls: ['./ai-chat-panel.component.css']
})
export class AiChatPanelComponent {
  @Input() context?: { app: string; resourceId?: string };
  @Output() messageEvent = new EventEmitter<any>();

  messages: any[] = [];
  userInput = '';
  isLoading = false;

  constructor(private aiService: AiService) {}

  sendMessage() {
    if (!this.userInput.trim()) return;

    const userMsg = { role: 'user', content: this.userInput };
    this.messages.push(userMsg);
    this.isLoading = true;

    const allMessages = [
      { role: 'system', content: this.getSystemPrompt() },
      ...this.messages
    ];

    this.aiService.chat(allMessages, this.context).subscribe({
      next: (response) => {
        const aiMsg = { role: 'assistant', content: response.message.content };
        this.messages.push(aiMsg);
        this.messageEvent.emit({ type: 'response', message: aiMsg });
        this.isLoading = false;
        this.userInput = '';
      },
      error: (err) => {
        console.error('AI Error:', err);
        this.isLoading = false;
      }
    });
  }

  private getSystemPrompt(): string {
    const prompts: Record<string, string> = {
      logistics: 'You are an assistant for logistics operations. Help with routes, inventory, and shipping.',
      docs: 'You are a writing assistant. Help with drafting, editing, and formatting documents.',
      marketplace: 'You are a marketplace assistant. Help vendors and customers with products, orders, and recommendations.',
      conquer: 'You are a strategy advisor for the Conquer game. Provide tactical and strategic guidance.',
      // Add more...
    };
    return prompts[this.context?.app || 'default'] || 'You are a helpful assistant for the Berjis ecosystem.';
  }
}
```

**HTML Template** (`ai-chat-panel.component.html`):

```html
<div class="flex flex-col h-full bg-slate-50 dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700">
  <!-- Header -->
  <div class="px-4 py-3 border-b border-slate-200 dark:border-slate-700">
    <h3 class="text-lg font-semibold text-slate-900 dark:text-white">
      AI Assistant
      <span *ngIf="context?.app" class="text-sm text-slate-500 dark:text-slate-400 ml-2">
        ({{ context.app }})
      </span>
    </h3>
  </div>

  <!-- Messages -->
  <div class="flex-1 overflow-y-auto p-4 space-y-4">
    <div *ngFor="let msg of messages" 
         [class]="msg.role === 'user' ? 'flex justify-end' : 'flex justify-start'">
      <div [class]="msg.role === 'user' 
        ? 'bg-blue-500 text-white rounded-lg px-4 py-2 max-w-[80%]'
        : 'bg-slate-100 dark:bg-slate-800 text-slate-900 dark:text-white rounded-lg px-4 py-2 max-w-[80%]'">
        {{ msg.content }}
      </div>
    </div>
    
    <div *ngIf="isLoading" class="flex justify-start">
      <div class="bg-slate-100 dark:bg-slate-800 rounded-lg px-4 py-2">
        <div class="flex space-x-2">
          <div class="w-2 h-2 bg-slate-400 rounded-full animate-bounce"></div>
          <div class="w-2 h-2 bg-slate-400 rounded-full animate-bounce" style="animation-delay: 0.1s"></div>
          <div class="w-2 h-2 bg-slate-400 rounded-full animate-bounce" style="animation-delay: 0.2s"></div>
        </div>
      </div>
    </div>
  </div>

  <!-- Input -->
  <div class="p-4 border-t border-slate-200 dark:border-slate-700">
    <div class="flex space-x-2">
      <input
        [(ngModel)]="userInput"
        (keydown.enter)="sendMessage()"
        [disabled]="isLoading"
        placeholder="Ask anything..."
        class="flex-1 px-4 py-2 rounded-lg border border-slate-300 dark:border-slate-600 
               bg-white dark:bg-slate-800 text-slate-900 dark:text-white
               focus:outline-none focus:ring-2 focus:ring-blue-500"
      />
      <button
        (click)="sendMessage()"
        [disabled]="isLoading || !userInput.trim()"
        class="px-6 py-2 bg-blue-500 hover:bg-blue-600 text-white rounded-lg
               disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Send
      </button>
    </div>
  </div>
</div>
```

### **Option B: Floating Widget (For Any App)**

```typescript
// ai-floating-widget.component.ts
@Component({
  selector: 'berjis-ai-widget',
  template: `
    <div class="fixed bottom-4 right-4 z-50">
      <button *ngIf="!isOpen" 
              (click)="toggle()"
              class="w-14 h-14 bg-blue-500 hover:bg-blue-600 rounded-full shadow-lg
                     flex items-center justify-center text-white transition-transform
                     hover:scale-110">
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" 
                d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" />
        </svg>
      </button>

      <div *ngIf="isOpen" 
           class="w-96 h-[500px] bg-white dark:bg-slate-900 rounded-lg shadow-2xl 
                  border border-slate-200 dark:border-slate-700 flex flex-col">
        <div class="flex items-center justify-between px-4 py-3 border-b border-slate-200 dark:border-slate-700">
          <h3 class="font-semibold text-slate-900 dark:text-white">AI Assistant</h3>
          <button (click)="toggle()" class="text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <berjis-ai-chat-panel [context]="context" class="flex-1"></berjis-ai-chat-panel>
      </div>
    </div>
  `
})
export class AiFloatingWidgetComponent {
  @Input() context?: any;
  isOpen = false;

  toggle() {
    this.isOpen = !this.isOpen;
  }
}
```

**Usage in Any App:**

```typescript
// In app.component.ts
import { Component } from '@angular/core';

@Component({
  selector: 'app-root',
  template: `
    <router-outlet></router-outlet>
    <berjis-ai-widget [context]="{app: 'marketplace', userId: currentUserId}"></berjis-ai-widget>
  `
})
export class AppComponent {
  currentUserId = 'uuid-here';
}
```

---

## **Phase 3: App-Specific Features**

### **Priority Apps for AI Integration**

1. **Marketplace** (Highest ROI)
   - Product description generator
   - Customer support chatbot
   - Personalized recommendations
   - Vendor onboarding wizard

2. **Docs/Notes** (High User Value)
   - Auto-complete while typing
   - Grammar/style checker
   - Document summarization
   - "Rewrite this as..." actions

3. **Logistics** (Efficiency Gains)
   - Route optimization
   - Inventory forecasting
   - Natural language queries: "Show shipments delayed by >2 days"

4. **Conquer** (Engagement)
   - Strategy advisor
   - NPC dialogue
   - Battle predictions

### **Marketplace Example: Product Description Generator**

```typescript
// In marketplace frontend
export class ProductFormComponent {
  constructor(private aiService: AiService) {}

  async generateDescription(productName: string, category: string) {
    const prompt = `Generate a compelling product description for:
    - Product: ${productName}
    - Category: ${category}
    
    Make it 2-3 sentences, highlight benefits, and use persuasive language.`;

    this.aiService.complete(prompt).subscribe({
      next: (response) => {
        this.productForm.patchValue({ description: response.text });
      }
    });
  }
}
```

---

## **Phase 4: RAG Implementation (Week 3-4)**

### **Add Vector Database**

Add to `docker-compose.yml`:

```yaml
qdrant:
  image: qdrant/qdrant:latest
  ports:
    - "6333:6333"
  volumes:
    - qdrant-data:/qdrant/storage
  environment:
    - QDRANT__SERVICE__GRPC_PORT=6334

volumes:
  qdrant-data:
```

### **Extend AI Service**

```go
// ai/service/internal/rag/qdrant.go
package rag

import (
    "context"
    "github.com/qdrant/go-client/qdrant"
)

type RAGService struct {
    client *qdrant.Client
    ollama *OllamaClient // for embeddings
}

func (r *RAGService) IndexDocument(ctx context.Context, doc Document) error {
    // 1. Chunk document into 512-token segments
    chunks := r.chunkDocument(doc.Content, 512)
    
    // 2. Generate embeddings with nomic-embed-text
    embeddings, err := r.ollama.Embed("nomic-embed-text", chunks)
    if err != nil {
        return err
    }
    
    // 3. Upsert to Qdrant
    points := make([]qdrant.PointStruct, len(chunks))
    for i, chunk := range chunks {
        points[i] = qdrant.PointStruct{
            ID:      qdrant.NewIDNum(uint64(i)),
            Vector:  embeddings[i],
            Payload: map[string]interface{}{
                "text":        chunk,
                "document_id": doc.ID,
                "app":         doc.App,
                "user_id":     doc.UserID,
            },
        }
    }
    
    return r.client.Upsert(ctx, "documents", points)
}

func (r *RAGService) Search(ctx context.Context, query string, app string, topK int) ([]SearchResult, error) {
    // 1. Generate query embedding
    queryEmbed, err := r.ollama.Embed("nomic-embed-text", []string{query})
    if err != nil {
        return nil, err
    }
    
    // 2. Search Qdrant with filter
    results, err := r.client.Search(ctx, "documents", queryEmbed[0], topK, &qdrant.SearchParams{
        Filter: &qdrant.Filter{
            Must: []qdrant.Condition{
                {Key: "app", Match: &qdrant.Match{Value: app}},
            },
        },
    })
    
    return results, err
}
```

### **RAG-Augmented Chat**

```go
func (s *Server) handleRAGChat(c *fiber.Ctx) error {
    var req ChatRequest
    if err := c.BodyParser(&req); err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
    }
    
    // 1. Search relevant context
    lastUserMsg := req.Messages[len(req.Messages)-1].Content
    contextDocs, err := s.rag.Search(c.Context(), lastUserMsg, req.Context.App, 3)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    
    // 2. Augment system prompt with context
    contextStr := ""
    for _, doc := range contextDocs {
        contextStr += fmt.Sprintf("\n\nRelevant context:\n%s", doc.Text)
    }
    
    systemMsg := req.Messages[0]
    systemMsg.Content += contextStr
    req.Messages[0] = systemMsg
    
    // 3. Forward to Ollama
    return s.ollama.Chat(req.Messages)
}
```

---

## **Phase 5: Monitoring & Optimization**

### **Add Audit Table**

```sql
-- In api/migrations/
CREATE TABLE ai_interactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    app TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt_tokens INT,
    completion_tokens INT,
    latency_ms INT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_ai_user ON ai_interactions(user_id);
CREATE INDEX idx_ai_app ON ai_interactions(app);
```

### **Log Every Request**

```go
func (s *Server) logInteraction(userID, app, model string, promptTokens, completionTokens, latencyMs int) {
    s.db.Exec(`
        INSERT INTO ai_interactions (user_id, app, model, prompt_tokens, completion_tokens, latency_ms)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, userID, app, model, promptTokens, completionTokens, latencyMs)
}
```

### **Add Prometheus Metrics**

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "ai_request_duration_seconds",
            Help: "AI request latency",
        },
        []string{"model", "app"},
    )
    
    requestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ai_requests_total",
            Help: "Total AI requests",
        },
        []string{"model", "app", "status"},
    )
)

func init() {
    prometheus.MustRegister(requestDuration, requestsTotal)
}
```

---

## **Recommended Timeline**

| Week | Focus | Deliverables |
|------|-------|--------------|
| 1 | Stabilize gateway | Models pulled, /v1/completion and /v1/embeddings added |
| 2 | Frontend lib | Angular ai-chat library, floating widget, integrated in 1 app |
| 3 | App features | Marketplace AI, docs auto-complete, logistics queries |
| 4 | RAG setup | Qdrant running, document indexing, RAG-augmented chat |
| 5+ | Scale & optimize | Audit logging, metrics, fine-tuning prep |

---

## **Immediate Commands to Run**

```bash
# 1. Pull models (if not already done)
docker exec -it ollama ollama pull llama3.1:8b
docker exec -it ollama ollama pull mistral:7b
docker exec -it ollama ollama pull nomic-embed-text

# 2. Verify AI service is healthy
curl https://ai-api.berjis.tech/health

# 3. Test chat (replace TOKEN)
curl -X POST https://ai-api.berjis.tech/v1/chat \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.1:8b",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'

# 4. Generate Angular library
cd landing  # or any Angular workspace
ng generate library ai-chat --prefix=berjis

# 5. Install TypeScript AI client in frontends
cd marketplace/frontend  # or any app
npm install @berjis/ai-client
```

---

## **Cost/Performance Targets**

| Metric | Target | Your Setup |
|--------|--------|------------|
| Latency (100 tokens) | <2s | ~1.5s with 12GB GPU |
| Concurrent users | 4-8 | Limited by GPU VRAM |
| GPU utilization | <90% sustained | Monitor with `nvidia-smi` |
| Storage per app index | 1-5GB | You have 4TB available |

---

## **Questions to Answer**

1. **Which app should get AI first?** (Recommend: Marketplace for revenue impact)
2. **Chat widget style**: Floating bubble or sidebar panel?
3. **Rate limits**: Should free users have limits? (Suggest: 10/min free, 100/min pro)
4. **Privacy**: Can user content be indexed for RAG? (Need consent mechanism)
5. **Multimodal**: Do you need image understanding later? (Would require larger models)

Let me know which phase you want detailed code for!