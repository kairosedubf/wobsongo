# Overview

Wobsongo is an open-source backend for automated claim verification. Feed it documents — official health guidance, fact sheets, guidelines — and it builds a structured, searchable knowledge base out of them; submit a claim or question, and it checks that claim against the knowledge base and returns a cited, per-sub-claim verdict rather than a single yes/no.

Under the hood: a REST API (Go/Echo) fronts an async ingestion pipeline (parsing → chunking → embedding → knowledge extraction, via a Postgres-backed job queue) and a hybrid retrieval layer (full-text + vector search, bilingual English/French) that both the claim-checking feature and a standalone search tool draw on.

## How you'd use it

- **Web UI** — hands-on document management and claim-checking without writing any code. See [Web UI](web-ui.md).
- **REST API** — integrate ingestion and claim-checking into your own application or bot. See [API Reference](../api/overview.md) — this is the surface to use once the web UI's forms aren't enough for your workflow.
- **CLI** — bulk operations, admin tasks, and evaluation tooling (`cmd/`) for operators running their own deployment. See [Developer Guide](../developer/local-setup.md).

## Why this exists

Wobsongo contributes to:

- **SDG 3 (Good Health & Well-being)** — infrastructure to combat health misinformation with cited, sourced answers instead of unverified claims.
- **SDG 16 (Peace, Justice, Strong Institutions)** — transparent fact-checking of public information, where every verdict traces back to a specific document and passage.

## Next steps

- New to the domain vocabulary (chunks, atomic knowledge, verdicts)? Start with [Concepts](concepts.md).
- Want to run it and try it yourself? Go to [Quickstart](quickstart.md).
