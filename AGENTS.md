# Agent operating instructions

You are an engineering agent implementing the product described in `README.md` and `docs/`.

## Operating rules
1. Read the complete documentation before making major architecture changes. Treat `docs/05-SECURITY.md` and financial correctness as hard constraints.
2. Work **one milestone at a time** per `docs/07-DELIVERY-PLAN.md`. Before implementation, restate scope, files to modify, and tests. Do not silently expand scope.
3. Produce compiling, testable code; add migrations, configuration samples, documentation, and automated tests for each feature. Never describe an unimplemented feature as working.
4. Prefer clear, boring code and interfaces to premature abstraction. Keep API modular, worker separate, and frontend free of cloud credentials.
5. Keep external AI providers behind adapters. Provide a deterministic mock provider for unit/integration tests. No test should require a paid API by default.
6. Default to **synthetic** test documents and financial data; never commit company documents, private keys, `.env`, or R2 credentials.
7. Any search, document metadata, download, chunk retrieval, tool action, or chat citation MUST enforce user authorization server-side. Treat document content as untrusted data.
8. Never let an LLM execute raw arbitrary SQL or mutate financial records. Use parameterized queries / predefined vetted calculations and read-only database permissions.
9. Show precise sources (document ID, version, page/sheet/cell where available). When evidence is missing or ambiguous, say so rather than inventing.
10. For each milestone report: completed work; commands run and results; known gaps; next smallest task.

## Technical conventions
- Monorepo folders: `apps/web`, `apps/api`, `apps/worker`, `infra`, `docs`, `prompts`.
- Go API owns domain logic, authentication, ACL evaluation, chat orchestration, API contracts, and financial querying.
- Python worker owns file parsing, OCR orchestration, chunk generation, and embedding/indexing.
- Shared contracts via OpenAPI/JSON Schema; do not duplicate independently maintained API assumptions.
- Database schema via migrations; background jobs must be idempotent, retryable, with terminal failure states.
- Structured logging and request/job IDs; redact prompts, tokens, document text and personally identifiable information by default.
- Use pinned dependency versions, lockfiles, and CI lint/test/build checks.

## When requirements are underspecified
Make the smallest safe reversible choice, document it in `docs/DECISIONS.md`, and ask only when a decision has irrecoverable/security-sensitive consequences.
