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

## M1 implementation decisions (2026-10-13)
- **Dev auth fixture**: `Authorization: Bearer dev-<email>` resolves seeded dev users (replaceable `auth.Authenticator` interface). The API **refuses to start in `prod` env** because no production IdP exists yet (M5).
- **ACL is one code path** (`internal/authz`): deny-by-default; access = admin role OR exact-action grant to user/role/department. Grant actions do not inherit (`read` does not allow `download`); `manage` is reserved for future admin tooling. All document queries receive a computed scope — repositories have no scopeless read path.
- **Uniform 404** for missing and forbidden documents on get/download/status; list/search omit inaccessible documents including from counts. Access-denied attempts are written to `audit_events` (`document.access_denied`, result `denied`) without confirming existence in the response.
- **Downloads stream through the API after ACL checks** by default; `?mode=presigned` returns a short-lived signed URL (TTL default 60s, hard cap 15 min) only when the storage backend supports presigning.
- **Uploads**: admin role only; buffered in memory; validated by extension allowlist (.pdf/.docx/.xlsx) AND magic bytes (`%PDF-`, ZIP for OOXML) with mismatch rejection; default cap 25 MB (`API_MAX_UPLOAD_BYTES`); filename sanitized for storage metadata and `Content-Disposition` only — storage keys are opaque UUIDs.
- **Object storage backends**: `memory` (dev default; files lost on restart) and `s3` (R2-compatible). Production requires `s3` with bucket + credentials + endpoint (validated at startup).
- Classification whitelist for MVP: `internal | confidential | restricted`.
- Version status is `uploaded` only in M1; `queued/processing/ready/failed` transitions arrive with the M2 worker.
- Audit writes are best-effort (failure logged, request proceeds) — revisit reliability requirements in M5.
- AWS SDK for Go v2 pinned to the Go-1.22-compatible release line (s3 v1.71.1); Go toolchain stays 1.22 (`GOTOOLCHAIN=local`).
- Integration tests run against the compose database via `ACL_TEST_DATABASE_URL` (set by `make test`); they skip cleanly when the database is unavailable, keeping `make test` usable offline.

## M2 implementation decisions (2026-10-14)
- **Redis Streams is the trigger; Postgres is the source of truth for job state.** Upload writes `document_versions` + `ingestion_jobs` + `ingestion_outbox` in one transaction (outbox pattern). The API dispatches to the stream after commit; a 30s outbox sweep (startup + ticker) re-dispatches anything left `pending`, so a Redis outage degrades to slower pickup instead of lost jobs.
- **Retries are DB-driven**: the worker claims with `status='queued' AND run_after <= now()`; transient failures set exponential `run_after` backoff (`base * 2^(attempts-1)`, capped 1h); `max_attempts` is written by the worker at claim time (config wins, per-job override possible). Terminal state is `failed` (job + version). Stale `processing` jobs older than 10 minutes are re-queued at worker startup.
- **Idempotency**: processing does `DELETE chunks + INSERT chunks` for the version inside one transaction, so re-processing never duplicates active chunks (verified end-to-end with a duplicated stream message and with reindex).
- **pgvector dimension is config-driven**: `cmd/migrate` renders `{{EMBEDDING_DIM}}` (default `API_EMBEDDING_DIM`=1536) into a temp copy of the migrations before applying; changing models requires re-index (M3 concern).
- **Chunking**: units are PDF pages (exact page provenance) and DOCX paragraphs/tables (`section_ref` such as `para:12`, `table:1:row:2`; DOCX chunks carry `section_ref` instead of page numbers). Units pack up to `WORKER_CHUNK_TARGET_CHARS` (default 1200); oversized units split on sentence boundaries with `WORKER_CHUNK_OVERLAP_CHARS` (default 150) overlap between pieces. `token_count` is a deterministic estimate (`len/4`, min 1).
- **FTS uses the `simple` dictionary** in a generated `tsv` column (Indonesian has no built-in Postgres dictionary; `unaccent` is available for query normalization in M3).
- **Local S3-compatible backend**: compose runs SeaweedFS (`chrislusf/seaweedfs:3.80`, S3 API on 127.0.0.1:19000) because `minio/minio` is not pullable from this environment. The Go S3 adapter attempts HeadBucket/CreateBucket at startup (warn-only on failure; R2 buckets are usually pre-created). Dev S3 credentials are placeholders.
- **Worker fetches objects directly** (S3 API via boto3, mirroring the production R2 path); a memory fetcher exists for tests/dev. Worker uses the least-privilege `worker` DB role.
- Malformed envelopes are acked to a dead-letter stream (`<stream>:dead`); unexpected worker errors are acked + dead-lettered to avoid redelivery loops, with the job row left visible for the reindex recovery path.
- Pinned worker deps: redis 5.0.8, psycopg[binary] 3.2.3, pypdf 4.3.1, python-docx 1.1.2, boto3 1.35.10. XLSX parsing is deliberately absent until M4.
- Both Go and Python integration tests TRUNCATE their fixtures on cleanup so the shared dev database is not polluted across runs.
