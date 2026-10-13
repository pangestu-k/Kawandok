# 03 — Logical data model and storage

The following is a starting logical schema, not final SQL. Use Postgres migrations and foreign keys.

## Identity and permissions
- `users(id, email, display_name, status, department_id, external_subject)`
- `departments(id, name)`
- `roles(id, name)`; `user_roles(user_id, role_id)`
- `document_grants(id, document_id, principal_type, principal_id, action)` — allowed principal type `user|role|department`, action `read|download|manage`; policy deny-by-default. Record owners/admins explicitly; do not infer read from upload alone.

## Documents
- `documents(id, title, classification, owning_department_id, created_by, current_version_id, created_at, deleted_at)`
- `document_versions(id, document_id, version_number, original_filename, mime_type, size_bytes, sha256, storage_bucket, storage_key, status, parser_version, embedding_model, created_at)`
- `document_chunks(id, document_version_id, ordinal, content, page_start, page_end, sheet_name, cell_range, token_count, embedding vector(<configured_dimension>), tsv tsvector)`
- `ingestion_jobs(id, document_version_id, status, attempts, error_code, created_at, finished_at)`
- `audit_events(id, actor_id, action, target_type, target_id, result, timestamp, request_id)`

Store originals only in private R2 and refer to storage keys from Postgres. Avoid original filename as unique key. Do not hard-code vector dimension until embedding model picked; changing model/dimension requires new index/re-embedding.

## Conversations
- `chat_sessions(id, owner_user_id, title, created_at)`
- `chat_messages(id, session_id, role, body, citations_json, created_at)` — apply ACL on chat history; avoid storing sensitive raw tool data unless necessary, with retention policies.

## Structured financial data (first template)
- `financial_reports(id, document_version_id, department_id, period_start, period_end, currency, validation_status)`
- `financial_transactions(id, report_id, transaction_date, category, amount_minor_units, currency, source_sheet, source_row, external_transaction_ref)`
- `financial_accounts(id, code, label, group)` (optional once accounting taxonomy established).

Use `NUMERIC` for monetary value or integer minor units with consistent currency scale, never binary float. Uniqueness/idempotency keys prevent double import. Financial row visibility must be constrained via report/department scope. Protect against duplicate cross-file aggregation.

## Suggested indexes
B-tree on document ownership, grants, dates, status, report scope; GIN on tsvector; pgvector ANN index only after validating index size and distance metric; exact search acceptable at small development scale.

## Data integrity
Soft deletion alone does not erase R2 copies or vectors; define a purge workflow for later. On version replacement, do not mix old/current chunks unless user requests version history.
