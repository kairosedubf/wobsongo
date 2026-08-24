# Architecture

Wobsongo is structured around a strict layered architecture with one governing rule:

> **Every part of the system that needs to interact with data or trigger business logic uses the service layer.**

Repos are private implementation details of services. HTTP handlers, background workers, and CLI commands never access repos directly — they call services.

---

## The Layers

```
┌─────────────────────────────────────────┐
│  HTTP Handlers  (handler/)              │
│  Workers        (worker/)               │  ← all callers go through service
│  CLI            (cmd/)                  │
├─────────────────────────────────────────┤
│  Services       (service/)              │  ← single entry point for all logic
├─────────────────────────────────────────┤
│  Repos          (repo/)                 │  ← private to services
├─────────────────────────────────────────┤
│  DB             (db/)                   │  ← SQLC-generated
└─────────────────────────────────────────┘

Cross-cutting:
  data/   → repo + external provider interfaces
  model/  → domain models
  queue/  → background job definitions
```

Dependencies flow downward only. No layer reaches upward.

---

## What Belongs Where

### Handlers (`handler/`)

HTTP transport only.

- Bind and validate the incoming request DTO.
- Call one service method.
- Write the response.

Handlers contain **no business logic**. If you find yourself doing an `if` on domain data in a handler, it belongs in the service.

### Services (`service/`)

All business logic lives here.

- Services are the **only** layer that depends on repo interfaces (`data/`).
- When multiple SQL operations must succeed or fail together, the service calls `repo.WithTx()` and composes individual atomic repo calls inside.
- When an operation must atomically write to the DB and enqueue a background job (e.g., create a record and send a notification), both happen inside a single `WithTx()` call via `txRepo.Enqueue()`.

### Repos (`repo/`)

Private to services. Not part of the public API of any package.

- Each method performs **exactly one SQL statement**.
- No repo method calls another repo method.
- No repo method manages its own transaction.
- Maps database types (SQLC output) to domain models (`model/`).
- Maps database errors to typed application errors (e.g., `ErrNotFound`, `ErrConflict`).

### Workers (`worker/`)

Workers implement `river.Worker[T]` and process background jobs. They call **service methods**, never repos or DB queries directly. A worker is just another caller of the service layer.

### CLI Commands (`cmd/`)

Management commands (migrations, seed data, admin operations) use service methods for any data operations. No command handler should open its own database connection for business logic.

---

## Transactions

Transaction boundaries are owned by the service layer.

**Correct:**

```go
// Service composes atomic repo methods in one transaction
func (s *DocumentService) Ingest(ctx context.Context, form *dto.IngestDocumentDTO) error {
    doc := buildDocument(form)
    return s.repo.WithTx(ctx, func(txRepo data.DocumentRepoer) error {
        if err := txRepo.Create(ctx, doc); err != nil {
            return err
        }
        // Job is enqueued in the same transaction — atomically
        return txRepo.Enqueue(ctx, &queue.ProcessDocumentDTO{ID: doc.ID})
    })
}
```

**Incorrect:**

```go
// Repo manages its own transaction — NOT allowed
func (r *DocumentRepo) IngestAndEnqueue(ctx context.Context, ...) error {
    return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
        // multiple SQL ops...
    })
}
```

The first version keeps all business decisions in the service. The second hides them in the data layer where they can't be tested without a database.

---

## Interfaces (`data/`)

The `data` package defines Go interfaces for every repo and external provider. Services depend on these interfaces, not on concrete implementations. This is what makes unit testing possible: mocks (in `mockrepo/`) implement the same interfaces.

**Repo interfaces** embed `data.TxAware[T]` when they need transaction support, and `queue.JobEnqueuer` when they need to enqueue jobs:

```go
type DocumentRepoer interface {
    GetByID(ctx context.Context, id uuid.UUID) (*model.Document, error)
    Create(ctx context.Context, doc *model.Document) error
    // ...
    data.TxAware[DocumentRepoer]
    queue.JobEnqueuer
}
```

---

## Adding a New Feature

Follow this checklist top-down:

1. **Model** — add or extend a struct in `model/`
2. **SQL** — write the query in `sql/queries/`, run `make gen` to regenerate `db/`
3. **Repo interface** — add the method to the relevant interface in `data/`
4. **Repo implementation** — implement the method in `repo/` (one SQL op per method)
5. **Mock** — regenerate the mock via `make gen`
6. **Service** — add the business method in `service/`, compose repo calls, handle transactions
7. **Handler** — add the HTTP endpoint in `handler/`, call the service method
8. **Test** — write service-level unit tests using the mock repo

---

## Further Reading

- [ADR 0001 — Layered Architecture](adr/0001-layered-architecture.md) — the formal decision record with rationale
