# Extending

## Worked example: adding a new embedding provider

Wobsongo already supports two embedding wire shapes behind one interface — see [External Integrations](external-integrations.md) for the existing `openai`/`bge` pattern. This walks through adding a third, e.g. a hypothetical `cohere` shape.

**1. Everything hangs off one interface** — `data.Embedder` (`data/vector.go`):

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

Any type satisfying this can be swapped in without touching `service/` or `worker/` at all — they only ever call `Embed`.

**2. Implement the client** in `external/cohere_client.go`, following the existing `EmbeddingClient`/`BGEClient` shape:

```go
package external

type CohereClient struct {
    baseURL, apiKey string
    httpClient      *http.Client
}

var _ data.Embedder = (*CohereClient)(nil)

func NewCohereClient(baseURL, apiKey string) *CohereClient { /* ... */ }

func (c *CohereClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
    // build the request in Cohere's wire shape, POST, parse, return vectors
    // in the same order as texts — re-sort by a response index field if the
    // API doesn't guarantee ordering (EmbeddingClient does this; BGEClient
    // doesn't need to because its API guarantees input order).
}
```

**3. Add the provider constant** in `config/config.go`:

```go
const (
    EmbeddingProviderOpenAI = "openai"
    EmbeddingProviderBGE    = "bge"
    EmbeddingProviderCohere = "cohere" // new
)
```

Update `IsEmbeddingOK`'s validation to accept it, and `NewEmbeddingConfig` if the new provider needs config fields beyond the existing `BaseURL`/`Model`/`APIKey`.

**4. Wire the constructor** in `cmd/server.go`'s `newEmbeddingClient`:

```go
func newEmbeddingClient(cfg *config.EmbeddingConfig) data.Embedder {
    switch cfg.Provider {
    case config.EmbeddingProviderBGE:
        return external.NewBGEClient(cfg.BaseURL, cfg.APIKey)
    case config.EmbeddingProviderCohere:
        return external.NewCohereClient(cfg.BaseURL, cfg.APIKey)
    default:
        return external.NewEmbeddingClient(cfg.BaseURL, cfg.Model, cfg.APIKey)
    }
}
```

**5. Document the new env value** in `.env.example`'s `EMBEDDING_PROVIDER` comment block.

**6. Test it** the same way `external/embedding_client_test.go` does — spin up an `httptest.Server` that returns a fixed response in the new shape, assert `Embed` parses it correctly (including any index re-sorting), and assert the request body/auth header match what the real API expects.

No changes needed anywhere in `service/`, `worker/`, or `handler/` — that's the point of depending on the `data.Embedder` interface rather than a concrete client type.

## The general pattern for any new feature

The embedding-provider example above is a case of *swapping an implementation behind an existing interface*. Adding a genuinely new capability (a new resource, a new verification module, a new storage backend) follows the fuller top-down checklist in [Patterns & Conventions](patterns-and-conventions.md):

```
model/ → sql/queries/ (+ make gen) → data/ (interface) → repo/ (implementation)
       → mockrepo/ (+ make gen) → service/ → handler/ (or webhandler/)
```

Work through it in that order — each layer's interface should exist before you implement the layer below it, so the mock (and therefore the test for the layer above) is available as soon as you need it.
