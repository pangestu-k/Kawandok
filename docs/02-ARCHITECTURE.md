# 02 — Architecture and deployment

## Components
- `apps/web`: Next.js UI — employee chat, source links, admin document interface, statuses. No LLM/R2 secrets in browser.
- `apps/api`: Go modular monolith — authz, document API, chat orchestration, retrieval gateway, financial queries, issuance of presigned URL after ACL check.
- `apps/worker`: Python worker — R2 fetch, PDF/DOCX/XLSX parsing, chunking, embedding, index updates. Reads queue and writes ingestion results; least-privilege service credentials.
- PostgreSQL + `pgvector`: users/roles, documents, versions, ACLs, chunks and vectors, chat metadata, financial normalized tables, audit logs.
- Redis: background-job queue, with retries/dead-letter approach; not source of truth.
- R2: **private** buckets of originals; key is opaque, never derived from untrusted client-supplied filename.
- LLM and embedding APIs: independent provider adapters; separate model settings by environment. Embedding dimension and model identity tracked per index.

## Upload path
Browser -> authenticated API -> check type/size/quota/permission -> store original R2 object (server-side or signed upload with finalize validation) -> insert metadata/version and ingestion job transactionally using outbox or reliable job enqueue pattern -> worker fetches R2 -> parse/chunk -> embedding API -> Postgres vector index -> mark status ready. A document is not searchable until current version is ready.

## Chat path
Browser -> API identifies principal -> agent/router chooses document retrieval or fixed finance tool -> restrict SQL/search by allowed document IDs and finance scope **before** returning tool results -> gather provenance -> LLM response streamed to browser with final structured citation metadata. LLM never receives unauthorized chunk content. Each cited source must be checked again on access.

## Download path
Browser -> API document authorization -> short-lived R2 presigned GET URL OR API proxy -> file download. Do not make bucket public and do not surface direct permanent object keys.

## Development profile
Local: Next.js, Go, Python worker, PostgreSQL/pgvector, Redis via Docker Compose as desired; real R2 dev bucket, fake/sample files, mock LLM by default; optional free-tier LLM via environment configuration. Make unit tests fully offline.

## Production notes
Use managed secrets, TLS, separate service accounts, backup and restore tests, database connection pooling, per-user rate limits, environment separation, job concurrency limits, request tracing. Start with modular monolith; split services only for observed scalability needs.

## Failure handling
Cloud LLM timeouts: bounded retry and user-safe error. Embedding provider failure: retry ingestion, do not expose incomplete index. R2 failure: keep file status failed/queued with audit trail. Permission changes: apply immediately to retrieval and download irrespective of previously created embeddings.
