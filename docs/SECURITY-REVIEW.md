# Security review (M5)

Scope: the implemented MVP (M0–M5) as of 2026-10-14. Hard requirements live in
`docs/05-SECURITY.md`; this is the control-by-control status.

## Implemented controls

| Control | Status | Where |
|---|---|---|
| Authentication on every `/v1/*` endpoint; health/metrics exempt | ✅ | `internal/auth` middleware; prod startup refuses dev auth |
| Deny-by-default document ACL on search, list+counts, detail, download, retrieval, citations, chat history, finance tools | ✅ | `internal/authz` single code path; negative tests per milestone |
| Uniform 404 for missing and forbidden documents (anti-enumeration) | ✅ verified byte-identical | `documents` service + tests |
| Object storage private; opaque UUID keys; keys never sent to clients | ✅ | `internal/storage`, upload flow |
| Short-lived presigned URLs (≤15 min hard cap, default 60 s), proxied streaming default | ✅ | `documents.Download` |
| Magic-byte + extension upload validation, size cap, sanitized filenames | ✅ | `documents.Service.Upload` + tests |
| Prompt injection treated as data; no tools exposed to the model; citation re-validation | ✅ | `chat.Service` + golden tests |
| No LLM arithmetic on money; predefined parameterized SQL only; finance deny-by-default + audit | ✅ | `internal/finance`, `chat/tools.go` |
| Least-privilege DB roles (api/worker/migrator), no DDL for apps | ✅ verified live | migration 000001 |
| Worker treats file content as untrusted; permanent vs transient failure separation | ✅ | `worker/*` |
| Rate limiting (per-IP token bucket) | ✅ | `httpapi/ratelimit.go` (M5) |
| Security headers (nosniff, frame-deny, referrer-policy), JSON error envelope with request IDs | ✅ | `httpapi` |
| Logs: structured JSON, request IDs, sensitive-field scrubbing, no query strings | ✅ | `internal/logging` |
| Secrets never committed; `.env` ignored; secret scanning (gitleaks/fallback) | ✅ | `make secret-scan`, CI |
| Metrics exposure gated by config | ✅ | `/metrics`, `API_METRICS_ENABLED` |
| Audit log for upload/download/denied reads/permission-relevant finance actions | ✅ | `internal/audit` + tests |

## Known gaps (accepted for MVP, tracked)

1. Chat citations are validated at emission and history read, but a revoked
   grant does not invalidate an already-issued presigned URL until TTL expiry
   (≤60 s default — documented risk per `docs/05-SECURITY.md` §10).
2. Rate limiting is per-IP in-process; horizontal deployments need a shared
   store (Redis) — trivial to add, not needed for single-instance MVP.
3. Audit writes are best-effort (failure logged, request proceeds).
4. Prompt-injection defenses are behavioral (evidence-as-data framing, no tool
   side effects, citation bounds) — measured by golden tests, not provable.
5. Dev JWT issuer must be removed before launch (guarded in prod startup).
6. No malware scanning of uploaded files (docs/05 §4 "strategy" decision
   pending with the company).

## Negative-test coverage (all automated)

- Unauthorized access via search, direct ID, download, RAG context, citations,
  chat history, finance tools → denied/empty, audited.
- Grant revocation blocks new access immediately (documents, citations).
- Injection fixtures cannot trigger tool execution or cross-document citation.
- Duplicate financial imports never double-count; mixed currencies fail loudly.

## Pending company decisions (blockers for production)

Identity provider · R2 region/backup/DPA · retention & deletion · LLM provider
agreement · malware-scanning strategy · incident-response ownership.
