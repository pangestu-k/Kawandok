# 01 — Product Requirements Document

## Audience and jobs-to-be-done
- **Employee**: ask a conversational question about accessible office policies/contracts; find and download documents; request a report from authorized financial datasets.
- **Admin**: upload and tag documents, assign permitted audiences, track ingestion status, manage access and versions.
- **Finance-authorized user**: ask for totals, comparisons, and ratios based on validated structured financial rows, with transparent periods and source provenance.

## MVP capabilities (must-have)
1. Login and server-side identity/role/department handling (select a real identity mechanism before production; dev users only for local testing).
2. Admin uploads text-based PDF, DOCX, XLSX via authenticated backend; originals stored in **private R2 bucket**; metadata and status in Postgres.
3. Async ingestion worker: read R2 object, parse content, record pages/sheets, chunk text, embed, index; handle failures and reprocessing/versioning.
4. Chat with streaming text response: retrieve only authorized chunks using hybrid retrieval and respond with source citations.
5. Search documents by name/metadata/content; open details and securely download original file if authorized.
6. Financial MVP: supported spreadsheet template(s) with explicit column mapping, currency and dates; validate ingestion; predefined read-only aggregation tools for totals and period-to-period comparison. Numbers must match SQL/reference calculations.
7. Audit security-relevant events; basic API rate limits; health checks, metrics and errors.

## User stories and testable acceptance criteria
- **Document discovery**: Given an authorized uploaded document, when a user searches exact filename/contract number, the document is returned; inaccessible documents never appear in results or counts.
- **Grounded Q&A**: For an indexed, accessible policy document, chat answers using cited passages; if relevant evidence absent, explains it cannot determine the answer.
- **Download**: An authorized employee gets a short-lived download link or authenticated stream; a non-authorized employee receives 403 with no leaked object key.
- **Ingestion**: Upload status transitions `uploaded -> queued -> processing -> ready` or `failed`, with retries; same job does not duplicate active chunks.
- **Finance**: Given fixture with Jan=12m, Jan transport=5m, Feb=15m IDR, authorized query produces total 32m IDR if grouping all those rows; sums are computed in validated code/SQL rather than LLM arithmetic.
- **Permissions**: Finance-only documents and row-level financial data cannot be accessed by an employee outside finance via API, RAG results, citations, tools, signed URLs, or chat history.
- **Error handling**: Corrupt/unsupported file produces a clear error and no false ready state.

## Not in MVP
Google Workspace/Drive/SharePoint sync, scanned-document OCR at scale, arbitrary financial models or autonomous writes, document editing, cross-tenant collaboration. OCR may be added behind an opt-in feature later.

## Metrics
Document ingestion success rate; citation accuracy measured on evaluation fixture; unauthorized access regression tests; financial result exactness; latency p50/p95; LLM token/cost usage; upload size/error rates.

## Open decisions
Identity provider; production data residency/legal obligations; maximum file size; supported XLSX template; exact roles and finance dataset visibility; retention/deletion policy; model/provider selection and data-handling agreement.
