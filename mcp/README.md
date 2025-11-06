# MCP Server (planned)

Goal: expose Berjis tools via Model Context Protocol so power users can connect compatible clients.

Initial tools:
- `search`: call `search/service` endpoints
- `docs.summarize(url|docId)`: proxy to docs service
- `marketplace.analyze(customerId|shopId)`: call marketplace service

Auth:
- Require Core API access token passed via MCP transport metadata; validate via `/v1/auth/verify`.

Status:
- Placeholder. Implement after AI gateway stabilizes.

