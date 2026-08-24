# Local Setup

## Prerequisites

- [Jetify Devbox](https://www.jetify.com/docs/devbox/quickstart/) — provides a reproducible toolchain (Go, Node/pnpm, Python, `golangci-lint`, `go-migrate`, Tailwind, `make`) via Nix, so you don't need any of these installed globally.
- Docker (for local Postgres/MinIO via `compose.yml`).

## First-time setup

```bash
git clone git@github.com:kairosedubf/wobsongo.git
cd wobsongo
devbox shell        # drops you into the reproducible toolchain
go mod download
cp .env.example .env  # then fill in the required values below
```

### Required env vars to get the server running

Per `.env.example`, these are **required for the server to start** (not just for full functionality):

- `APP_DB_URI`, `APP_JWT_SECRET`
- `VLM_BASE_URL` / `VLM_MODEL` (image captioning)
- `EMBEDDING_BASE_URL` / `EMBEDDING_MODEL` (chunk + knowledge embeddings)
- `EXTRACTION_BASE_URL` / `EXTRACTION_MODEL` (atomic knowledge extraction)

These point at OpenAI-compatible chat-completions/embeddings endpoints — self-hosted (vLLM, Ollama, text-embeddings-inference) or a hosted provider. See [External Integrations](external-integrations.md) for the full picture, including the `bge` embedding shape as an alternative to the default OpenAI-compatible one.

### Database

```bash
make dbup        # start Postgres (+ pgvector) via compose.yml
make migrateup   # run migrations
```

## Running the server

Two alternatives — **don't run both at once**, they fight over port 8000:

- **`make dev`** — [air](https://github.com/air-verse/air) hot-reload, plain `go run`, manual browser refresh. Also the option that regenerates swagger docs on rebuild (see the `check` target).
- **`task dev`** — full auto-reload flow: `templ generate --watch` + Tailwind watch, proxied through `:7331` → `:8000`. Browse via `http://localhost:7331` (not `:8000`) to get live reload on `.go`/`.templ`/CSS changes.

First time using `task dev`, run `task setup-static` once beforehand to copy JS assets and compile the initial CSS bundle.

## Quality checks before committing

```bash
make check    # go mod tidy, build, swag regen, golangci-lint, sqlc diff
make test     # requires make dbtestup first — see Testing
```

See [Dev Tooling](dev-tooling.md) for the full command reference and [Testing](testing.md) for the test database setup.
