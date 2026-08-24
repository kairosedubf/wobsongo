# Testing

**80% line coverage is required for changed packages** on every PR (per `CONTRIBUTING.md`).

## Unit vs. integration

Tests are split via the `-short` flag:

```bash
make test-unit          # go test -short -v ./... — no database required
make test-integration   # requires a running test database — repo-layer tests
```

Repo-layer tests are integration tests by nature (they exercise real SQL against Postgres) and are skipped in `-short` mode.

## Running the full suite

```bash
make dbtestup     # spins up an isolated Postgres (test-compose.yaml), creates the
                   # test user/DB, pre-creates the pgvector extension, runs migrations
make test         # runs everything with coverage, writes coverdb.profile
make dbtestdown   # tear down the test database
```

`make test` sets `APP_ENV=testing` and friends inline — you don't need a separate `.env` for it. Coverage excludes `view/`/`components/` packages (generated `templ` code) via `-coverpkg`.

```bash
make testrace     # same test run, with -race
```

## Test helpers (`testhelpers/`)

- `SetupTestDB(t)` — connects to `APP_DB_URI` (fails the test immediately if unset), returns a pool + `db.Queries`. Used by repo integration tests.
- `WithTxRollback(t, pool, fn)` — runs `fn` inside a transaction that's always rolled back afterward, so each test gets a clean, isolated view of the database without truncating tables between runs.
- `echo.go` — Echo test-request/context helpers.
- `response.go` / `statuscode.go` — response-body and status-code assertion helpers for handler tests.

## Mocks (`mockrepo/`)

Service-layer unit tests mock the `data/` interfaces via `moq`-generated mocks in `mockrepo/` — regenerate with `make gen` after changing an interface. See [Patterns & Conventions](patterns-and-conventions.md) for how repo interfaces/mocks fit into the layering rules, and [Extending](extending.md) for a worked example that includes writing a service test against a mock.
