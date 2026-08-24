# Dev Tooling

## Makefile

| Command | What it does |
|---|---|
| `make check` | `go mod tidy` → build → `swag init` (regen OpenAPI spec) → `golangci-lint run` → `sqlc diff`. Run before every PR. |
| `make fmt` | `go fmt` + `golangci-lint fmt`. |
| `make gen` | `go generate ./...` — regenerates `mockrepo/` mocks (via `moq`, triggered by `//go:generate` directives) and anything else generate-driven. Does **not** regenerate sqlc or swagger — those are separate steps (`sqlc diff`/`sqlc generate`, `swag init`). |
| `make dev` | Runs the server with `air` hot-reload. |
| `make db` | Opens a `psql` shell to the local dev database. |
| `make dbup` / `dbstop` / `dbdown` / `dball` | Start / stop / remove / fully reset the local Postgres (via `compose.yml`). |
| `make migrateup` / `migratedown` | Run/rollback migrations. |
| `make reset` | Reset with seed data. |
| `make admin` | Create a local superadmin user (`admin.com` / `LocalDev123`). |
| `make dbtestup` / `dbtestdown` | Start/stop the isolated test database (`test-compose.yaml`) — required before `make test`. |
| `make test` / `test-unit` / `test-integration` / `testrace` | See [Testing](testing.md). |

## Taskfile (frontend asset pipeline)

| Task | What it does |
|---|---|
| `task setup-static` | One-time: copy JS assets, compile initial CSS. Run before first build if you haven't already. |
| `task dev` | Tailwind watch + `templ generate --watch` (which itself runs the server via `go run`, restarting on `.go`/`.templ` changes), proxied on `:7331`. |
| `task tailwind` | Tailwind CSS watch only. |
| `task templ` | `templ generate --watch` only, proxied on `:7331` → `:8000`. |
| `task build` | Production build: compile CSS, generate templ, build the binary with embedded assets. |

## Codegen tools

- **sqlc** — generates `db/` from `sql/queries/` + `sql/migrations/`. Run `go tool sqlc diff` to check for drift, `go tool sqlc generate` to regenerate. See [Database](database.md).
- **moq** — generates mocks in `mockrepo/` from interfaces in `data/`, triggered by `//go:generate` directives on the interface source files. Run via `make gen`.
- **swag** — generates the OpenAPI spec in `swagger/` from handler annotations. Run via `make check` (wired into the `check` target) or standalone: `go tool swag init -q --output ./swagger --generalInfo ./core/app.go`. See [OpenAPI Spec](../api/openapi.md).
- **templ** — generates `_templ.go` files from `.templ` templates in `view/`.

## Linting

`golangci-lint` config lives in `.golangci.json`. Run via `make check` or `golangci-lint run ./...` directly inside `devbox shell`.

## ADRs

Architecture Decision Records live as plain markdown in `docs/adr/` and are linked directly into the mkdocs nav (see [Architecture](../architecture.md)). `devbox.json` also defines `adr`/`adr:preview`/`adr:build` scripts wrapping [log4brains](https://github.com/thomvaill/log4brains), but log4brains has no config file in this repo and isn't currently wired up — those scripts won't work out of the box. Don't rely on them; write and link ADRs as plain markdown until/unless log4brains is properly configured.
