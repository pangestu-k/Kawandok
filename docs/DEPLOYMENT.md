# Deployment notes (M5)

Status: **development-ready, not production-ready.** The company decisions listed
at the end are prerequisites for any real deployment. This document describes the
intended production shape and what is already in place.

## Topology

- `apps/web` (Next.js): static pages + `/api` proxy to the Go API. Holds no
  secrets; the browser only ever holds a short-lived user JWT.
- `apps/api` (Go): the only component with database, object-storage and LLM
  credentials. Exposes `/healthz`, `/readyz`, `/metrics`.
- `apps/worker` (Python): pulls ingestion jobs; least-privilege `worker` DB role.
- PostgreSQL 16 + pgvector, Redis 7 (job trigger), S3-compatible bucket
  (Cloudflare R2 in production; SeaweedFS locally).

## What M5 added

- JWT dev auth with HS256 + `external_subject` mapping (OIDC swap-in point).
  The production startup guard refuses `API_ENV=prod` until a real IdP exists.
- Per-IP token-bucket rate limiting (`API_RATE_LIMIT_RPM`, default 120/min).
- Prometheus metrics at `/metrics`: HTTP counters per route/status, latency
  histogram, LLM token counters per provider (`API_METRICS_ENABLED=0` disables).
- Structured JSON logs with request IDs; sensitive fields scrubbed; query
  strings never logged.
- Container images pinned by digest in `infra/docker-compose.yml`.
- Backup/restore: `make backup-db` (pg_dump | gzip) and
  `make restore-db BACKUP_FILE=... [RESTORE_DB=...]` — restore is rehearsed
  into a scratch database and verified before dropping.
- Load test: `make loadtest [N=200] [C=10]` exercises the authenticated read
  path and reports p50/p95/p99.
- CI: `.github/workflows/ci.yml` (build, vet, tests with real Postgres
  services for API and worker, web lint/test/build, gitleaks scan).

## Required before launch (company decisions — do not skip)

1. **Identity provider**: replace the dev token endpoint with OIDC
   (the `external_subject` column and `auth.Authenticator` boundary are the
   swap points). Remove `RegisterDevToken` wiring.
2. **R2 bucket + credentials**: region, bucket naming, key management, and a
   data-processing agreement with the provider.
3. **Retention & deletion policy**: soft-delete purge workflow for R2 objects,
   chunks and chat history.
4. **LLM provider contract**: model choice, cost ceiling, data handling.
5. **Security review sign-off**: see `docs/SECURITY-REVIEW.md`.
6. TLS termination (reverse proxy), managed secrets, separate environments,
   DB connection pooling review, alerting on `/metrics` and audit anomalies.
