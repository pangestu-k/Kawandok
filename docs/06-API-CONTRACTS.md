# 06 — Initial API and event contracts

Endpoints are proposed; describe with OpenAPI before coding client. All endpoints except health require auth.

## Documents
- `POST /v1/documents` — multipart or upload-init; fields `file`, `title`, `classification`, `department_id`, `grants`; response `document_id`, `version_id`, `status`.
- `GET /v1/documents?query=&status=&page=` — authorized list/search only.
- `GET /v1/documents/{id}` — document metadata and current version, ACL verified.
- `GET /v1/documents/{id}/download` — authorized short-lived URL or proxy; do not expose secret.
- `GET /v1/documents/{id}/versions/{versionId}/status` — queued/processing/ready/failed with sanitized error.
- `POST /v1/documents/{id}/reindex` — permission protected, idempotent version-specific job.

## Chat
- `POST /v1/chat/sessions` — create chat session.
- `POST /v1/chat/sessions/{id}/messages` — body `{message, client_request_id}`; SSE response with typed events `text_delta`, `tool_status`, `citation`, `done`, `error`. `citation` includes opaque document/version identifier, page or sheet and allowed display title.
- `GET /v1/chat/sessions/{id}/messages` — owner/authorized history.

## Financial
- Tool-only v1: `get_financial_summary` and `compare_financial_periods`, validated parameters and backend-enforced scopes. If public endpoints are introduced, require explicit finance permissions and audited use.

## Job state
`uploaded -> queued -> processing -> ready | failed`, plus retry/reindex as new attempts. Record timestamps, immutable version ID, deduplication id, sanitized failure reason. Do not return source contents from ingestion status APIs.

## Error format
`{ "error": { "code": "DOCUMENT_FORBIDDEN", "message": "Access denied", "request_id": "..." } }`.
Avoid leaking existence of forbidden files if policies require indistinguishable 404 responses.
