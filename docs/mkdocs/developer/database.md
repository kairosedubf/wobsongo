# Database

PostgreSQL with the [pgvector](https://github.com/pgvector/pgvector) extension for embedding similarity search.

## Migrations

`sql/migrations/` holds paired up/down [golang-migrate](https://github.com/golang-migrate/migrate) files, applied in order:

1. `init_schema` — base schema.
2. `create_documents_table`
3. `create_document_chunks_table`
4. `add_document_chunk_embeddings`
5. `create_atomic_knowledge_table`
6. `add_atomic_knowledge_embeddings`
7. `change_embedding_dimension_to_1024` — the reference pattern for changing embedding dimensions (see [External Integrations](external-integrations.md)).
8. `add_search_infra`
9. `add_atomic_knowledge_category`
10. `add_bilingual_search` — adds the `text_fts_en`/`text_fts_fr` generated full-text-search columns and translated-text columns backing bilingual retrieval.
11. `add_user_auth_fields`
12. `add_document_chunk_page_index`

```bash
make migrateup     # apply all pending migrations
make migratedown   # roll back one
```

## sqlc

`db/` is entirely generated from `sql/queries/*.sql` against the schema in `sql/migrations/` — never hand-edit anything in `db/`. Config lives in `sqlc.yaml`.

```bash
go tool sqlc diff       # check for drift between queries/schema and generated code (part of `make check`)
go tool sqlc generate   # regenerate db/
```

Notable overrides in `sqlc.yaml`: `uuid`/`timestamptz` map to `uuid.UUID`/`time.Time`, and both embedding columns are forced to the pointer form of `pgvector.Vector` — the non-pointer form panics on scanning a `NULL` embedding (a chunk/fact that hasn't been embedded yet), verified empirically; the pointer form round-trips `nil`/`NULL` correctly via pgx's generic nil-pointer handling.

## Language / bilingual search

`Language` (English/French) is stored as a plain integer, matching the existing `TruthTier`/`FactCategory` convention rather than a native Postgres enum. It's set explicitly at document ingestion — never auto-detected — and copied down onto `document_chunks`/`atomic_knowledge` rows at insert/extraction time (needed directly on each row since Postgres generated columns can't reference another table for the full-text search columns). `TranslateChunksWorker`/extraction populate a `*_translated` column per row so full-text search matches regardless of which language the search query is in.

## Adding a query

1. Write the SQL in `sql/queries/<table>.sql` following existing sqlc annotation conventions in that file.
2. `go tool sqlc generate` (or `sqlc diff` to check first) to regenerate `db/`.
3. Add the corresponding method to the relevant interface in `data/`, implement it in `repo/` (one SQL statement per method — see [Patterns & Conventions](patterns-and-conventions.md)), and regenerate its mock via `make gen`.
