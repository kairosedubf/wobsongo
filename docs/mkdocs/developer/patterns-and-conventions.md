# Patterns & Conventions

The one rule that governs everything else: **the service layer is the single entry point for business logic.** Handlers, workers, and CLI commands are all just callers of `service/` — none of them touch `repo/` or `db/` directly. Full rationale in [ADR 0001](../adr/0001-layered-architecture.md); this page is the condensed DO/DON'T version for day-to-day work.

## DO

- **Put business logic in `service/`, transport logic in `handler/`/`webhandler/`.** A handler binds a DTO, validates it, calls exactly one service method, and writes the response. If you're writing an `if` on domain data inside a handler, it belongs in the service.
- **Own transaction boundaries in the service layer.** When multiple SQL ops must succeed or fail together, call `repo.WithTx(ctx, func(txRepo) error { ... })` and compose individual repo calls inside. When a write must be atomic with enqueuing a job, do both inside the same `WithTx()` via `txRepo.Enqueue()`.
- **Keep each repo method to exactly one SQL statement.** No repo method calls another repo method; no repo method opens its own transaction. This is what makes every repo method trivially mockable.
- **Depend on interfaces (`data/`), not concrete types.** Services take `data.XRepoer` interfaces, never `*repo.XRepo` directly — that's what makes `mockrepo/` substitution possible in tests.
- **Follow the checklist for new features**, top-down: model (`model/`) → SQL query (`sql/queries/`, then `make gen`) → repo interface (`data/`) → repo implementation (`repo/`) → mock (`make gen`) → service (`service/`) → handler (`handler/`). See [Extending](extending.md) for a full worked example.
- **Map errors at the repo boundary.** Repos translate driver-level errors (`pgx.ErrNoRows`, etc.) into typed sentinels (`data.ErrNotFound`, `data.ErrConflict`, ...). Handlers translate those sentinels into HTTP status codes via `mapDataErrorToHTTP` — don't hand-roll a new error-to-status mapping per handler.
- **Tag every secret field `json:"-"`** on `config.Config` and anything derived from it.

## DON'T

| Anti-pattern | Why it's wrong |
|---|---|
| A repo method calls `pgx.BeginFunc`/`pool.Begin` | Transactions belong at the service layer, not hidden in data access. |
| A repo method performs more than one SQL statement | Breaks the "trivially mockable" property and obscures business logic in the data layer. |
| A worker opens its own DB connection or imports `repo`/`db` directly | Bypasses the service layer, breaks the "workers are just another caller of service" invariant. |
| A CLI command (`cmd/`) calls `db.Queries` directly | Same reasoning as workers — no direct DB access outside `service/`/`repo/`. |
| A service imports `db` directly | Services depend on repo interfaces, not SQLC output — otherwise swapping persistence details requires touching business logic. |
| Hand-editing anything under `db/` | It's entirely sqlc-generated from `sql/queries/` — edits get silently overwritten on the next `sqlc generate`. |

## Verifiable in review

Two questions catch most layering violations at PR time:

- Does this repo method contain more than one query call?
- Does this worker (or CLI command) import `repo` or `db`?

If either is "yes," the change likely needs restructuring before merge.
