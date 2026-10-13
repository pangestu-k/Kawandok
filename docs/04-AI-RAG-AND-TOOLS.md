# 04 — AI, RAG, tooling, and financial accuracy

## Provider abstraction
Backend exposes `LLMProvider` interface supporting messages, tool schemas, tool calls, streaming and usage; adapters for mock/free-tier cloud/premium cloud. Validate actual tool and JSON support per configured model. No vendor-specific field should leak into business-domain logic. Keep separate `EmbeddingProvider` interface.

## RAG ingestion
Parse PDF page-by-page, DOCX sections/tables, XLSX sheet/table metadata; preserve source provenance. Chunk with configurable boundaries and overlap. Embed chunks with a pinned embedding model; store model ID, vector dimension and parser version. Do not index empty/failed documents.

## Query pipeline
1. Authenticate user and compute permitted document and financial scope.
2. Classify intent (find-file, ask-document, calculate-finance, general/help) using deterministic routing rules where possible, LLM if needed.
3. For document RAG, hybrid search: metadata/exact identifier and PostgreSQL full-text + vector similarity; filter ACL **inside retrieval**, optionally rerank authorized results.
4. Assemble limited passages with document version and page/sheet info.
5. Instruct LLM to answer ONLY from evidence for document questions, reference sources, and clearly flag missing information.
6. Validate citation references against authorized source chunks before returning response.

## Tool contracts (server-side implementation)
- `search_documents(query, filters?)`: returns authorized document metadata and citations only.
- `get_document(document_id)`: metadata and access path after ACL; never blindly returns R2 secret.
- `retrieve_passages(query, document_ids?)`: authorized text passages + source identifiers.
- `get_financial_summary(department_scope, start_date, end_date, categories?)`: predefined parameterized read-only aggregate.
- `compare_financial_periods(scope, first_period, second_period)`: predefined aggregate and variance logic.

LLM-provided tool arguments are **untrusted**. Validate types/ranges, enforce authenticated principal scopes, whitelist IDs, and limit query work. Treat parsed source document text as untrusted; it must never override system instructions.

## Financial correctness
Do NOT ask LLM to add spreadsheet numbers from retrieved chunks. Ingest supported spreadsheet schemas into structured tables with explicit source mapping and validation. Store original values, normalized currency, and row provenance. Use Postgres aggregates/verified Python or Go math, and reconcile expected sheet totals. Ambiguous units, missing headers, duplicates, conflicting report versions or mixed currencies => fail visibly or ask for clarification. No free-form text-to-SQL in MVP.

## Testing & evaluation
- Offline fixtures for parser, chunker, embeddings mock, authorization, retrieval filters and tool dispatch.
- Golden question-answer set covering correct citations, out-of-scope questions, missing docs, doc version changes, and prompt injection in documents.
- Financial fixture with known totals, negative values, dates, decimals, currencies and duplicated file imports.
- Compare free-tier and chosen production LLM on same evaluation dataset; track tool correctness, citation grounding, latency, token usage and cost.
- Pin model IDs, temperature/configuration, embedding model/version in test reports. Model swap requires regression evaluation.
