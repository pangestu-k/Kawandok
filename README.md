# Office AI Assistant â€” AI Agent Build Pack

Status: **M5 implemented — all milestones delivered** (frontend with full API integration: login/chat/admin; dev-JWT auth; rate limiting; metrics; CI; digest-pinned images; rehearsed backup/restore; load test; deployment + security review docs). NOT production-ready by policy: company decisions (OIDC, R2/DPA, retention, LLM agreement) remain open — see docs/SECURITY-REVIEW.md. This repository documentation remains the source of truth, not evidence that later features exist.

## Product in one sentence
A permission-aware internal company assistant, accessed by chat, that can locate and return original office files, answer questions grounded in their contents, and compute reliable financial results from structured data.

## Default technical decisions
- **Monorepo** with `apps/web` (Next.js/TypeScript), `apps/api` (Go), and `apps/worker` (Python).
- **LLM and embedding providers** are mock by default and OpenAI-compatible when configured. Never embed provider keys in the web client.
- **PostgreSQL + pgvector** for relational records, document chunks, and embeddings; PostgreSQL full-text search + vector retrieval.
- **SeaweedFS** provides local S3-compatible object storage; Cloudflare R2 is the intended private object store for a later production deployment.
- **Redis-based background jobs** to run ingestion asynchronously.
- **Docker Compose** runs local infrastructure only; API, worker, and web run as normal development processes.
- **Modular monolith**, not microservices proliferation; the document worker is a separate process.

## Local development quickstart

### Prerequisites

- Docker Desktop
- Go 1.22+
- Python 3.10+
- Node.js 20+ and npm
- GNU Make plus a POSIX shell (Git Bash or WSL) when using the `make` commands

### One-time setup

Run from the repository root:

```sh
cp infra/.env.example .env

cd apps/worker && python -m venv .venv && . .venv/bin/activate && python -m pip install -e ".[dev]"
cd ../web && npm ci
cd ../..
```

On Windows PowerShell, create the `.env` file with `Copy-Item infra\.env.example .env`.
To create and install the worker environment, use
`py -m venv apps\worker\.venv` then
` .\apps\worker\.venv\Scripts\python.exe -m pip install -e ".\apps\worker[dev]"`.

The API and worker find the nearest `.env` automatically. Existing process
environment variables take precedence, so do not commit `.env` or put real
credentials in it.

### Start the project

Start local infrastructure and apply migrations:

```sh
make up
make migrate
```

Run these commands in three separate terminals:

```sh
make run-go       # API: http://127.0.0.1:18080
make run-worker   # ingestion worker
make run-web      # web: http://127.0.0.1:3000
```

The equivalent commands without Make are:

```sh
docker compose -f infra/docker-compose.yml up -d
go -C apps/api run ./cmd/migrate -dir ../../infra/db/migrations -database "postgres://migrator:dev_migrator_password@127.0.0.1:15432/office_ai?sslmode=disable" up
go run ./apps/api/cmd/api
cd apps/worker && python -m worker.main
cd apps/web && npm run dev
```

Compose publishes only local infrastructure: PostgreSQL on `15432`, Redis on
`16379`, and S3-compatible SeaweedFS on `19000`. The API uses `18080` and the
web UI uses `3000`, so they can run beside projects using the usual `5432` and
`6379` ports.

### First local simulation

Open `http://localhost:3000` and sign in as `admin@dev.local`. In **Admin
Dokumen**, upload a synthetic PDF, DOCX, or XLSX and set its department and
grants. The API stores it in local SeaweedFS; the worker parses and indexes it;
the status becomes `ready` when processing succeeds. Only a user with a
matching grant can find or ask about the document.

Seeded development users are `admin@dev.local` (admin), `budi@dev.local`
(employee, General), `siti@dev.local` (finance, Finance), and
`rian@dev.local` (employee, Finance). Upload requires the admin role and
access is deny-by-default.

### Mock and optional AI providers

The default LLM and embedding providers are deterministic mocks. They require
no cloud account and let you test upload, authorization, retrieval, citations,
and finance flows. Mock embeddings are not semantic, so natural-language
matching is limited; full-text matching still works.

The `openai` provider configuration accepts an OpenAI-compatible endpoint for
both chat and embeddings. API and worker must use the same embedding endpoint,
model, API key, and dimension. Changing an embedding model or dimension
requires a fresh vector schema and re-indexing all documents.

For local learning, malware scanning defaults to `disabled`. Before accepting
real documents, run a ClamAV-compatible daemon and set
`WORKER_MALWARE_SCAN_MODE=clamav`. Worker startup in `prod` requires it.

### Common Make targets

```sh
make test                         # Go, Python, and web tests
make lint                         # Go vet, Ruff/Mypy when installed, ESLint
make secret-scan                  # gitleaks or the tracked-file fallback
make backup-db                    # compressed local PostgreSQL backup
make restore-db BACKUP_FILE=...   # restore into a scratch database
make loadtest N=200 C=10          # authenticated read-path load test
make down                         # stop infra; keeps local volumes/data
```

`docker compose -f infra/docker-compose.yml down -v` removes local database
and object-storage volumes; use it only when intentionally resetting the
simulation environment.

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) and
[docs/SECURITY-REVIEW.md](docs/SECURITY-REVIEW.md) before considering any
non-local deployment. Production startup remains intentionally blocked until
OIDC, retention, storage, and provider decisions are made.

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
