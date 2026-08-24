# Background Jobs

Wobsongo uses [River](https://riverqueue.com/) (Postgres-backed job queue) for everything that shouldn't block an HTTP response — document ingestion is entirely asynchronous after the initial `POST /api/v1/documents` call.

Job payloads/kinds are defined in `queue/` (`document.go`, `claim_check.go`); the workers that process them live in `worker/`, and call only into `service/` — never into `repo/` or `db/` directly (same layering rule as everything else, see [Architecture](../architecture.md)).

## Document ingestion pipeline

```
POST /api/v1/documents
        │
        ▼
ParseDocumentWorker            fetches raw Docling output for the file, stores it in
                                object storage, enqueues the next job. Deliberately
                                does nothing else — Docling responses can be huge and
                                re-fetching from a rate-limited external service on
                                every retry would be wasteful.
        │
        ▼
ProcessParsedDocumentWorker    turns the stored raw output into DocumentChunk rows.
                                Never calls Docling itself, so it can retry freely.
                                Fans out to:
        │
        ├──▶ CaptionImageChunksWorker   generates captions for picture/chart chunks
        │        │                     (its own job so a slow/rate-limited VLM call
        │        │                     never fails or retries chunk storage itself).
        │        └──▶ re-enqueues EmbedChunks / ExtractKnowledge / TranslateChunks
        │             for the now-captioned (now text-bearing) chunks.
        │
        ├──▶ EmbedChunksWorker          computes + stores embeddings for text-bearing,
        │                               not-yet-embedded chunks (batched).
        │
        ├──▶ ExtractKnowledgeWorker     extracts atomic knowledge facts (subject-
        │        │                     predicate-object + truth tier) from text-bearing
        │        │                     chunks. Self-requeues to continue past its own
        │        │                     per-run batch limit on large documents.
        │        └──▶ enqueues EmbedKnowledgeWorker for the newly extracted facts.
        │
        └──▶ TranslateChunksWorker      translates text-bearing chunks into the other
                                        supported language, so bilingual full-text
                                        search finds them regardless of query language.
```

Each stage is its own job — not inlined into its predecessor — specifically so a slow, costly, or rate-limited external call (VLM captioning, embedding, extraction, translation) can fail/retry independently without re-doing already-completed work like chunk storage.

## Claim checking

`ClaimCheckWorker` runs the full analyze → retrieve → judge pipeline for a piece of text (`queue.ClaimCheckJob`). Note: this is the async/queued path, distinct from the synchronous `POST /api/v1/claims/check` HTTP endpoint, which calls the same service layer directly and returns the verdict in the response. For async callback support (e.g. bot notifications after a queued check completes), the worker's own docs point at wrapping `ClaimService` directly rather than extending this worker.

## Concurrency tuning

`ExtractKnowledgeWorker`/`TranslateChunksWorker` bound how many chunks they process concurrently via `EXTRACTION_CONCURRENCY`/`TRANSLATION_CONCURRENCY` (default 5 each) — tune based on your LLM backend's actual rate-limit tolerance, no code change needed.
