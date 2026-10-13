# Initial prompt for a coding agent

You are the lead engineer for the **Office AI Assistant** monorepo. First read `AGENTS.md`, `README.md`, and all `docs/*.md`. These files are the binding specification. Do not implement the entire product in one pass.

**Task for this run: M0 only**, as described in `docs/07-DELIVERY-PLAN.md`.

1. Summarize your understanding of the product and propose a short file-level implementation plan for M0.
2. Scaffold `apps/web` (Next.js/TypeScript), `apps/api` (Go), `apps/worker` (Python), and required Compose infrastructure (Postgres/pgvector, Redis); preserve one monorepo, do not split repositories.
3. Create maintainable `.env.example` configurations including placeholders for R2, LLM, embedding provider. Provide a deterministic mock provider that makes offline testing possible. Never include real credentials.
4. Add minimal health checks, migration setup, tests, lint/build configuration, local run commands, and CI workflows.
5. Execute the applicable checks, report actual results, unresolved issues and exact next milestone. Stop after M0.

Constraints: protect confidential data; follow security and ACL rules in specs; no cloud costs required for tests; no undocumented external services; do not implement arbitrary text-to-SQL; ask before making security-sensitive irreversible choices.
