# Quickstart

The fastest path to a working instance. This mirrors [Local Setup](../developer/local-setup.md) but framed for trying the product out rather than contributing code — see that page for the full contributor-oriented detail.

## 1. Get the toolchain and database running

```bash
git clone git@github.com:kairosedubf/wobsongo.git
cd wobsongo
devbox shell
go mod download
make dbup
make migrateup
```

## 2. Configure external services

Wobsongo needs a VLM (image captioning), an embedding endpoint, and a text-extraction endpoint configured to fully process documents and answer claim checks — these are **required for the server to start**, not optional extras. See [External Integrations](../developer/external-integrations.md) for what to point them at (self-hosted vLLM/Ollama, or a hosted provider). Without these configured, the server won't come up at all.

## 3. Start the server

```bash
make dev
```

## 4. Try it

**Via the web UI** — visit `http://localhost:8000/register`, create an account, log in, and upload a document. Once it's finished processing (parsing → chunking → embedding → knowledge extraction runs asynchronously), use **Check** to ask a claim against it. See [Web UI](web-ui.md) for a full walkthrough of every screen.

**Via the API** — once you have a document ingested (via the web UI or `POST /api/v1/documents`), check a claim directly:

```bash
curl -X POST http://localhost:8000/api/v1/claims/check \
  -H "Content-Type: application/json" \
  -d '{"text": "Does drinking cold water cause a cold?"}'
```

See [API Reference](../api/overview.md) for the full endpoint surface.

## Next steps

- Unfamiliar with the domain vocabulary used above (chunks, atomic knowledge, verdicts)? Read [Concepts](concepts.md) first.
- Building against the API rather than clicking through the UI? Jump straight to [API Reference](../api/overview.md).
