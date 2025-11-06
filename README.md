# berjis-ai (self-hosted)

Minimal Go gateway that authenticates against the Core API and proxies chat to Ollama. Designed for reuse across apps.

Architecture

- Gateway: Go/Fiber (`ai/service`) with Core auth verification and safety policy
- Runtime model server: Ollama (Docker)
- Optional persistence: Postgres for daily metrics (if `DATABASE_URL` is set)
- Optional monitoring: Prometheus scrape + Grafana dashboards (Docker)

Quick start

- docker compose services: `ollama`, `ai-service` (ports 11434, 8094)
- Pull models: `docker exec -it ollama ollama pull mistral:7b-instruct-q4_K_M` and `llama3.1:8b-instruct-q4_K_M`, plus `nomic-embed-text` for embeddings
- Auth: use your Core JWT (login at `api.berjis.tech` and reuse the `access` cookie), or send `Authorization: Bearer <token>`

Endpoints:

- `GET /v1/health` – liveness
- `GET /v1/models` – list Ollama models (auth required)
- `POST /v1/chat` – chat completion via Ollama (auth required)
- `POST /v1/completion` – simple text completion (auth required)
- `POST /v1/embeddings` – generate embeddings with `nomic-embed-text` (auth required)
- `GET /v1/admin/metrics` – JSON counters (auth + admin)
- `GET /metrics` – Prometheus exposition (plain text)
- `GET /v1/admin/metrics/daily?days=7|30` – Daily rolled‑up metrics for charts (auth + admin)

Auth:

- Accepts Core API access token (RS256) via `Authorization: Bearer` or `access` cookie.
- Verifies by calling `POST {CORE_API_BASE}/v1/auth/verify` and sets `uuid`/`email` in request context.

Config (env):

- `PORT` default `8094`
- `APP_ENV` default `development`
- `ALLOWED_ORIGINS` default `*`
- `CORE_API_BASE` default `http://api:8080`
- `OLLAMA_BASE` default `http://ollama:11434`
- `AI_SAFETY_STRICT` default `true` — when `true`, prompts that contain clearly sensitive developer/admin terms are blocked with a safe public response and a server log line; when `false`, safety prompts are still added but the gateway does not hard‑block.
- `DATABASE_URL` optional Postgres DSN; when set the gateway persists daily counters in `ai_metrics_daily` (see “Persistence”).

Safety and context

- The gateway prepends two system messages to every chat:
  - Safety rules that forbid exposing internal code, endpoints, schemas, secrets, or ops details; it steers answers to public, user‑facing guidance.
  - A concise “About Berjis” context so questions like “What is Berjis Ecosystem?” work without extra setup.
- A lightweight guard refuses obviously sensitive developer/admin requests and responds with a safe overview instead.
  - Logs: `policy: blocked sensitive prompt (uuid=<uuid> keyword=<matched>)` — no user text is stored.

Raising the knowledge ceiling safely

- For richer answers, index only public/marketing docs (e.g., README.md, project-structure.md, ui-guidelines.md, product pages) via `/v1/embeddings` and a vector DB. Avoid code repositories or config files.

Admin UI in landing

- Visit `/admin/ai` in landing to see live counters (requires admin).

Persistence

- Table: `ai_metrics_daily(day date, metric text, model text null, count bigint, primary key (day,metric,model))`
- Stored metrics: `chats` (per model), `completions` (per model), `embeddings` (aggregate)
- API: `GET /v1/admin/metrics/daily?days=7|30` returns `{ days: string[], metrics: { chats: { _all, <model> }, completions: {...}, embeddings: {_all} } }`

Docker (compose)

- Services: `ollama`, `ai-service` (8094), `ai-frontend` (5600), optional `prometheus` (9090) and `grafana` (3000)
- Domains via Cloudflared:
  - `ai.berjis.tech` → `ai-frontend`
  - `ai-api.berjis.tech` → `ai-service`

Angular integration

- Shared chat panel library lives at `ai/frontend/projects/ai-chat`
- Import in apps as needed (right‑sidebar drawer for marketplace and file apps recommended)

Examples

List models

```bash
curl -s https://ai-api.berjis.tech/v1/models \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Completion (expand a term)

```bash
curl -s https://ai-api.berjis.tech/v1/completion \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistral:7b",
    "prompt": "Summarize the query: Nairobi. Cover geography, history, and other notable meanings.",
    "max_tokens": 300
  }'
```

Embeddings (for RAG later)

```bash
curl -s https://ai-api.berjis.tech/v1/embeddings \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"texts": ["Nairobi is the capital of Kenya", "Masai Mara"]}'
```

Angular integration (landing)

- Add an AI Summary component that calls `/v1/completion` with the current search query.
- If the user is not authenticated, hide the card or show a sign-in prompt.

Building ai.berjis.tech frontend

- Path: `ai/frontend`
- Install deps and build production:
  - `yarn install`
  - `yarn build -c production` (equivalent to `ng build --configuration production`)
- Docker: `docker compose up -d --build ai-frontend`
