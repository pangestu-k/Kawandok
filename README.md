# Office AI Assistant â€” AI Agent Build Pack

Status: **M5 implemented — all milestones delivered** (frontend with full API integration: login/chat/admin; dev-JWT auth; rate limiting; metrics; CI; digest-pinned images; rehearsed backup/restore; load test; deployment + security review docs). NOT production-ready by policy: company decisions (OIDC, R2/DPA, retention, LLM agreement) remain open — see docs/SECURITY-REVIEW.md. This repository documentation remains the source of truth, not evidence that later features exist.

## Product in one sentence
A permission-aware internal company assistant, accessed by chat, that can locate and return original office files, answer questions grounded in their contents, and compute reliable financial results from structured data.

## Default technical decisions
- **Monorepo** with `apps/web` (Next.js/TypeScript), `apps/api` (Go), and `apps/worker` (Python).
- **Cloud LLM** behind a provider interface: free-tier compatible in development, production model configurable. Never embed model keys in web client.
- **PostgreSQL + pgvector** for relational records, document chunks, and embeddings; PostgreSQL full-text search + vector retrieval.
- **Cloudflare R2** private object bucket for original documents in dev and initial production; access through backend-generated short-lived signed URLs or authenticated streaming.
- **Redis-based background jobs** to run ingestion asynchronously.
- **Docker Compose** for local Postgres/Redis and apps. R2 remains remote.
- **Modular monolith**, not microservices proliferation; the document worker is a separate process.

## Development quickstart (M0)

Prerequisites: Go 1.22+, Node.js 20+, Python 3.10+, Docker.

```sh
make up            # infra: Postgres(pgvector):15432, Redis:16379, S3 (SeaweedFS):19000
make migrate       # apply infra/db/migrations
go run ./apps/api/cmd/api   # API on :18080 — reads .env automatically
cd apps/worker && python -m worker.main   # ingestion worker — reads .env automatically
cd apps/web && npm install && npm run dev   # UI on :3000, proxy /api -> :18080
```

`make` is for infrastructure and helper tasks only — the three apps run as
plain processes. One-time setup: `cp infra/.env.example .env` (repo root);
the API and worker auto-load that file (existing environment variables win).

Verification (all offline, no cloud credentials required):

```sh
make test          # Go + Python + web tests
make lint          # go vet / ruff+mypy (if installed) / eslint
make secret-scan   # gitleaks if installed, else pattern fallback
make down          # stop the dev stack
```

Configuration template: `infra/.env.example` â€” copy to `.env` and fill in before using real providers. Never commit `.env` or real credentials. Dev-only database passwords exist in migration 000001 and must be rotated before any shared environment (see `docs/DECISIONS.md`).

Development auth fixture: request a short-lived JWT for a seeded user, then pass it as a bearer token:

```sh
TOKEN=$(curl -s -X POST http://127.0.0.1:18080/auth/dev/token \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@dev.local"}' | python -c "import sys,json;print(json.load(sys.stdin)['access_token'])")
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18080/v1/documents
```

Seeded dev users: `admin@dev.local` (admin), `budi@dev.local` (employee, General), `siti@dev.local` (finance, Finance), `rian@dev.local` (employee, Finance). Upload requires the admin role and grants are deny-by-default. The dev issuer is disabled by the production startup guard; OIDC is still required before launch.

### IDE troubleshooting (Go imports)

If your editor reports `cannot find package ... in GOROOT or GOPATH`, the tooling is resolving in **GOPATH mode** instead of module mode (this machine has `GOPATH=D:\Working\Golang` and the repo sits inside `GOPATH\src`, with `GO111MODULE=auto`). The repo ships `go.work` and `.vscode/settings.json` that force module mode:

1. Open the **repository root** (`office-ai-agent`) in your editor, not a subfolder.
2. VS Code: reload the Go language server (`Go: Restart Language Server`) or reload the window.
3. GoLand: Settings â†’ Go â†’ Go Modules â†’ enable Go modules integration; GOROOT must point to Go 1.22.
4. Recommended machine-wide fix: `setx GO111MODULE on` (then reopen the editor).

The CLI is unaffected: `go build ./...` / `go test ./...` inside `apps/api` and from the repository root (via `go.work`) both pass.

## Read these files, in order
1. `AGENTS.md` â€” rules for coding agents
2. `docs/01-PRD.md` â€” scope and acceptance criteria
3. `docs/02-ARCHITECTURE.md` â€” boundaries and flows
4. `docs/03-DATA-MODEL.md` â€” database and storage contracts
5. `docs/04-AI-RAG-AND-TOOLS.md` â€” retrieval, agents and financial correctness
6. `docs/05-SECURITY.md` â€” confidentiality and authorization
7. `docs/06-API-CONTRACTS.md` â€” endpoints and asynchronous behaviors
8. `docs/07-DELIVERY-PLAN.md` â€” incremental build order, tests and definition of done
9. `prompts/START-HERE.md` â€” prompt to give the implementing agent

## Working assumptions to confirm before launch
Single company initially, separate departments and users; admin uploads files; PDF/DOCX/XLSX; Indonesian first; synthetic demo files only until company security and data policies are approved. Scope and integration choices are configurable; **do not invent a production IAM or data retention policy**.

## Non-goals for MVP
- Training/fine-tuning an LLM
- Google Drive/SharePoint sync
- General autonomous write actions against enterprise systems
- Free-form arbitrary SQL execution by an LLM
- Payroll / financial transactions posted into accounting systems
- Guaranteeing correct OCR of every scanned or malformed file
