# Architecture decision log

Use for new decisions made during implementation. Record date, decision, options considered, consequences, and reversibility.

## Initial decisions
- Monorepo + modular monolith Go API + Python worker + Next.js UI.
- R2 private object storage rather than storing uploaded files in Git/project directory.
- Postgres with pgvector and full-text search, Redis queue for indexing.
- Cloud LLM is provider-configurable, mocked for offline tests.
- Predefined verified financial aggregate tools rather than arbitrary LLM-generated SQL.

## Pending decisions
- Production identity provider and exact authorization rules.
- Cloud LLM and embedding providers, model identifiers, dimensionality, and commercial/data-processing terms.
- R2 region, backup strategy, legal storage requirements and retention/deletion.
- Finance spreadsheet format and mappings.

## M0 implementation decisions (2026-10-13)
- Git repo initialized; **CI deferred by product-owner choice**. All tests run offline via `make test`; CI must be introduced no later than M2 so ACL negative tests gate merges.
- Job queue contract: **Redis Streams with a versioned JSON envelope** (`apps/worker/worker/envelope.py`). Canonical-UUID job ids, whitelisted job types, and payload size caps are validated on producer and consumer sides; dead-letter handling arrives in M2.
- Schema migrations live in `infra/db/migrations` and are applied by `apps/api/cmd/migrate` (golang-migrate v4.17.1). Only migrations write DDL; applications consume the schema.
- **Least-privilege Postgres roles** `api` and `worker` are created by migration 000001 (no superuser/createdb/createrole; DML only via default privileges). Verified: both roles can connect; neither can create tables; refined per-table grants continue in M1/M2.
- Role passwords in migration 000001 are **local-dev only** and must be rotated through secrets management before any shared or production environment (M5).
- Dev host ports are **15432 (Postgres)** and **16379 (Redis)** because 5432/6379 were already bound by other local containers; dev-only, reversible via env vars.
- Uniform 404 for both missing and unauthorized documents will be enforced in M1 (anti-enumeration, per docs/06).
- Storage keys will be opaque server-generated UUIDs, never returned to clients; downloads via presigned URLs with short TTL (<=60s) — enforced in M1.
- LLM and embedding providers are **mock-only in M0** (`API_LLM_PROVIDER`/`API_EMBEDDING_PROVIDER` accept `mock` only); cloud adapters with pinned models and evaluation arrive in M3.
- Mock embedding dimension defaults to 1536 via `API_EMBEDDING_DIM`; the M2 pgvector migration will take the dimension from configuration at migration time.
- Logging is structured JSON with request IDs and a sensitive-field scrub list (tokens, prompts, document content, credentials); URL query strings are intentionally never logged.
- Container images pinned by version tag (pgvector/pgvector:pg16, redis:7.2.5-alpine); digest pinning is an M5 hardening task.
- `make secret-scan` runs gitleaks when installed, otherwise a pattern-based fallback over tracked files.
