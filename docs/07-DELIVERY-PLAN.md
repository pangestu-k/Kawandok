# 07 — Implementation plan for coding agents

Implement sequentially. Each phase includes working code, migrations, docs and passing tests. Do not jump to agent orchestration before the base security boundary works.

## M0 — Scaffold and local environment
**Deliver**: repo folders, Go API health, Next.js skeleton, Python worker skeleton, Compose for postgres+pgvector and Redis, `.env.example`, lint/tests/CI, migration runner, README dev commands. No secrets. Mock provider returns deterministic output.
**Done**: single command or documented minimal commands bring up services; automated smoke tests pass.

## M1 — Identity and document upload
**Deliver**: development auth fixture (replaceable), users/departments/roles/grants, R2 adapter, authenticated upload, document metadata + status, private download flow, explicit ACL tests.
**Done**: authorized user uploads/downloads; disallowed user denied via ID/list/download; file not stored in Git.

## M2 — Worker and indexing
**Deliver**: reliable ingestion jobs, worker parser for text PDFs and DOCX first, source-aware chunking, embedding adapter/mocks, pgvector migration, worker statuses/retry/idempotency, indexing fixture.
**Done**: upload -> ready, correct citations metadata and no duplicated chunks on retry.

## M3 — Document RAG chat
**Deliver**: chat sessions, streaming, hybrid retrieval, authorized retrieval and grounded answers with verified citation IDs, mock and configurable cloud LLM adapters, golden tests.
**Done**: source-backed chat; unavailable document returns uncertainty; permission-bypass tests pass.

## M4 — Structured finance (XLSX template)
**Deliver**: mapped sample XLSX import with validation, row provenance, financial tables, predefined read-only sum and comparison tools, tool dispatcher integrated with chat, audit tests.
**Done**: finance test results exactly match fixtures; unauthorized roles cannot access financial values.

## M5 — Production hardening
**Deliver**: explicit auth provider decision, secrets, rate limits, observability/cost telemetry, security scans, backup restore walkthrough, tests against selected cloud premium model, ingestion load tests.
**Done**: documented deployment and security review; no outstanding high-severity issues; product is not declared production-ready automatically.

## Suggested test commands
Agent must establish real commands for its chosen frameworks; typical targets: `make dev`, `make test`, `make lint`, `make migrate`. Never invent successful test output.

## PR checklist
Requirement linked; migrations reversible; role/permission negative tests; no key leaks; sample data synthetic; error/retry path; observability; docs updated; tests actually run.
