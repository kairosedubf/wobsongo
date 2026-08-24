# Project Structure

Wobsongo is a flat-layout Go module — packages live at the repo root, there is **no `internal/` prefix** anywhere in the codebase. (Older versions of this doc and the ADR referenced `internal/...` paths from an earlier refactor; those have been corrected. If you see `internal/` mentioned elsewhere — old comments, a stale Makefile target — treat it as a leftover, not the real layout.)

## Top-level packages

| Package | Purpose |
|---|---|
| `cmd/` | Cobra CLI commands — `serve`, migrations, `createsuperadmin`, evaluation/backfill utilities. `main.go` just calls `cmd.Execute()`. |
| `core/` | App wiring: builds the Echo instance, mounts `/api` and the web route group, registers middleware. |
| `config/` | Environment-variable driven `Config` struct and per-feature sub-configs (S3, email, VLM, embedding, extraction, translation, claim-check). |
| `handler/` | REST API transport layer (`/api/v1/*`) — binds/validates DTOs, calls one service method, writes the response. No business logic. |
| `webhandler/` | HTML web UI transport layer — same transport-only rule as `handler/`, but for `templ`-rendered pages and JWT-cookie auth. |
| `service/` | All business logic. The only layer allowed to depend on `data/` interfaces; owns transaction boundaries. |
| `repo/` | Persistence layer implementing the interfaces declared in `data/`. One SQL statement per method, no nested transactions. |
| `data/` | Go interfaces for every repo and external provider — the contracts services depend on, and what `mockrepo/` implements for tests. |
| `db/` | sqlc-generated code from `sql/queries/`. Never hand-edited. |
| `model/` | Domain types shared across layers (documents, users, verdicts, API envelope types). |
| `dto/` | Wire-format request/response structs for the API (validation tags, JSON tags, pagination helpers). |
| `queue/` | River job/payload definitions and the `JobEnqueuer` interface. |
| `worker/` | River job handlers — the document/knowledge processing pipeline. Call service methods only, never repos or `db` directly. |
| `external/` | HTTP clients for external services: Docling (parsing), VLM (captioning), embeddings, extraction/judge/claim-analyzer LLMs, translation. |
| `auth/` | JWT issuing/parsing and password hashing used by the web UI's session auth. |
| `view/` | `templ` templates (and their generated `_templ.go` companions) for the web UI. |
| `ui/` | Embedded static assets (compiled CSS/JS) served by the web app. |
| `validation/` | Custom `go-playground/validator` rules and DTO validation registration. |
| `mockrepo/` | `moq`-generated mocks of the `data/` interfaces, used in service-layer unit tests. |
| `testhelpers/` | Shared test setup (DB, Echo test harness, response assertions). |
| `sql/` | `migrations/` (golang-migrate) and `queries/` (sqlc source). |
| `swagger/` | Generated OpenAPI/Swagger 2.0 spec (`swag`) — see [OpenAPI Spec](../api/openapi.md). |

## Request flow at a glance

```
HTTP request
  → handler/ (or webhandler/)   transport: bind, validate, call service
  → service/                    business logic, transaction boundaries
  → repo/  (via data/ interfaces)  one SQL op per call
  → db/                         sqlc-generated queries
```

Background work follows the same rule one level removed: `worker/` picks a job off the River queue and calls into `service/` exactly like a handler would — it never reaches into `repo/` or `db/` directly. See [Architecture](../architecture.md) for the full layering rules and [ADR 0001](../adr/0001-layered-architecture.md) for the rationale.
