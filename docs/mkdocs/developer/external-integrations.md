# External Integrations

All LLM/embedding/parsing backends are pluggable HTTP clients in `external/`, configured entirely through environment variables (`config/config.go`). Every one of them talks to a generic OpenAI-compatible (or documented alternative) wire shape, so they work unmodified against self-hosted vLLM/Ollama/text-embeddings-inference or a hosted provider using that shape — no code change needed to swap backends, only env vars.

| Client | Purpose | Config prefix | Notes |
|---|---|---|---|
| `external/docling.go` | Parses ingested documents (PDF/etc.) into structured layout + text via [Docling Serve](https://github.com/docling-project/docling-serve). | `DOCLING_BASE_URL` | Not gated behind a validity check like the others — always expected to be configured. |
| `external/vlm_client.go` | Generates captions for picture/chart chunks. Vision chat-completions API. | `VLM_*` | Required for the server to start. |
| `external/embedding_client.go` / `external/bge_client.go` | Computes embeddings for chunks and atomic-knowledge facts. | `EMBEDDING_*` | Two wire shapes behind one `data.Embedder` interface — see below. Required for the server to start. |
| `external/extraction_client.go` | Extracts atomic knowledge (subject-predicate-object facts + truth tier) from chunk text. Text-only reasoning. | `EXTRACTION_*` | Required for the server to start. `EXTRACTION_CONCURRENCY` (default 5) bounds parallel chunk processing. |
| `external/translation_client.go` | Translates chunk/fact text into the deployment's other supported language. | `TRANSLATION_*` | Defaults to the same backend as extraction in practice, but independently configurable. `TRANSLATION_CONCURRENCY` (default 5). |
| `external/claim_analyzer_client.go` | Scope/decomposition analysis for `POST /api/v1/claims/check` — is this in scope, what are the sub-claims. | `CLAIM_CHECK_*` | |
| `external/judge_client.go` | Verdict judging for claim checking — given retrieved evidence, decide supported/contradicted/etc. | `CLAIM_CHECK_*` | Shares config with the claim analyzer. |

## The embedding dual-shape pattern

`data.Embedder` is a single-method interface (`Embed(ctx, texts) ([][]float32, error)`), and two implementations satisfy it:

- `EmbeddingClient` — the default. Speaks the standard `POST {baseURL}/v1/embeddings` shape with `{"model", "input"}`, used by OpenAI, Ollama, LiteLLM, vLLM, etc. Re-sorts the response by its `index` field rather than trusting ordering.
- `BGEClient` — for self-hosted BGE deployments that expose a simpler `POST {baseURL}` endpoint with `{"texts": [...]}` and no index field.

`EMBEDDING_PROVIDER` (`openai` default, or `bge`) selects which one `cmd/server.go`'s `newEmbeddingClient` constructs. This is the reference pattern for [adding a new provider](extending.md) of your own.

## Embedding dimension

Embedding columns (`document_chunks.embedding`, `atomic_knowledge.embedding`) are fixed at 1024 dimensions, sized for BGE-M3. Switching to a model with a different output dimension requires a migration — see `sql/migrations/000007_change_embedding_dimension_to_1024.up.sql` for the pattern — and clearing existing embeddings so they get recomputed.
