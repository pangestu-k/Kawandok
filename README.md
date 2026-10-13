# Office AI Assistant — AI Agent Build Pack

Status: **M0 scaffold implemented** (compose dev stack, health endpoints, migrations, deterministic mock AI providers, offline tests). Product features arrive milestone by milestone per `docs/07-DELIVERY-PLAN.md`. This repository documentation remains the source of truth, not evidence that later features exist.

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
make up            # start Postgres(pgvector) on 127.0.0.1:15432 and Redis on 127.0.0.1:16379
make migrate       # apply infra/db/migrations (creates least-privilege db roles)
go run ./apps/api/cmd/api   # API health endpoints on :8080
cd apps/worker && python -m worker.main
cd apps/web && npm install && npm run dev
```

Verification (all offline, no cloud credentials required):

```sh
make test          # Go + Python + web tests
make lint          # go vet / ruff+mypy (if installed) / eslint
make secret-scan   # gitleaks if installed, else pattern fallback
make down          # stop the dev stack
```

Configuration template: `infra/.env.example` — copy to `.env` and fill in before using real providers. Never commit `.env` or real credentials. Dev-only database passwords exist in migration 000001 and must be rotated before any shared environment (see `docs/DECISIONS.md`).

## Read these files, in order
1. `AGENTS.md` — rules for coding agents
2. `docs/01-PRD.md` — scope and acceptance criteria
3. `docs/02-ARCHITECTURE.md` — boundaries and flows
4. `docs/03-DATA-MODEL.md` — database and storage contracts
5. `docs/04-AI-RAG-AND-TOOLS.md` — retrieval, agents and financial correctness
6. `docs/05-SECURITY.md` — confidentiality and authorization
7. `docs/06-API-CONTRACTS.md` — endpoints and asynchronous behaviors
8. `docs/07-DELIVERY-PLAN.md` — incremental build order, tests and definition of done
9. `prompts/START-HERE.md` — prompt to give the implementing agent

## Working assumptions to confirm before launch
Single company initially, separate departments and users; admin uploads files; PDF/DOCX/XLSX; Indonesian first; synthetic demo files only until company security and data policies are approved. Scope and integration choices are configurable; **do not invent a production IAM or data retention policy**.

## Non-goals for MVP
- Training/fine-tuning an LLM
- Google Drive/SharePoint sync
- General autonomous write actions against enterprise systems
- Free-form arbitrary SQL execution by an LLM
- Payroll / financial transactions posted into accounting systems
- Guaranteeing correct OCR of every scanned or malformed file
