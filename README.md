# wobsongo

[![[![coverage](https://github.com/kairosedubf/wobsongo/actions/workflows/coverage.yml/badge.svg)](https://github.com/kairosedubf/wobsongo/actions/workflows/coverage.yml)docs](https://github.com/kairosedubf/wobsongo/actions/workflows/docs.yml/badge.svg)](https://github.com/kairosedubf/wobsongo/actions/workflows/docs.yml)[![codecov](https://codecov.io/gh/kairosedubf/wobsongo/branch/main/graph/badge.svg)](https://codecov.io/gh/kairosedubf/wobsongo)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/docs-mkdocs-blue)](https://kairosedubf.github.io/wobsongo/)

Wobsongo is an open-source backend for automated claim verification. Feed it documents — official health guidance, fact sheets, guidelines — and it builds a structured, searchable knowledge base out of them; submit a claim or question, and it checks that claim against the knowledge base and returns a cited, per-sub-claim verdict rather than a single yes/no.

Under the hood: a REST API (Go/Echo) fronts an async ingestion pipeline (parsing → chunking → embedding → knowledge extraction, via a Postgres-backed job queue) and a hybrid retrieval layer (full-text + vector search, bilingual English/French).

## Getting started

Want to try it out? See the [Quickstart](https://kairosedubf.github.io/wobsongo/guide/quickstart/) guide.

```bash
git clone git@github.com:kairosedubf/wobsongo.git
cd wobsongo
devbox shell
go mod download
make dbup
make migrateup
make dev
```

Wobsongo also needs a VLM, an embedding endpoint, and a text-extraction endpoint configured before the server will start — see [External Integrations](https://kairosedubf.github.io/wobsongo/developer/external-integrations/) for what to point those at. Once it's up, visit `http://localhost:8000/register` to create an account and start uploading documents.

## Developing

See [Local Setup](https://kairosedubf.github.io/wobsongo/developer/local-setup/) for the full contributor-oriented walkthrough (env vars, running with hot-reload, test database setup). In short:

```bash
make check        # go mod tidy, build, swag regen, lint, sqlc diff
make dbtestup      # start the test database
make test          # full test suite
```

## Documentation

Full docs — architecture, API reference, developer guide — are published at **[kairosedubf.github.io/wobsongo](https://kairosedubf.github.io/wobsongo/)**.

## Community & governance

- [Contributing](CONTRIBUTING.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Security Policy](SECURITY.md)
- [License](LICENSE) — Apache 2.0
