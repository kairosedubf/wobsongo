# OpenAPI Spec

A machine-readable Swagger 2.0 spec is generated from annotations on the handler functions (`handler/*.go`, e.g. the `@Summary`/`@Param`/`@Success` comments above each handler) using [`swag`](https://github.com/swaggo/swag).

## Where it lives

- [`swagger/swagger.yaml`](https://github.com/kairosedubf/wobsongo/blob/main/swagger/swagger.yaml)
- [`swagger/swagger.json`](https://github.com/kairosedubf/wobsongo/blob/main/swagger/swagger.json)
- `swagger/docs.go` — the same spec embedded as a Go package (used if you wire up `swaggo/echo-swagger` to serve interactive docs from a running instance; not currently mounted).

It covers the three resource groups documented here: [Documents](documents.md), [Media](media.md), and [Claims](claims.md).

## Viewing it

The easiest way to explore it interactively today: paste the contents of `swagger.yaml` or `swagger.json` into [editor.swagger.io](https://editor.swagger.io/), which renders a browsable, "try it out"-capable UI without needing a local server.

## Regenerating it

The spec is generated, not hand-written — never edit `swagger/*` directly. Regenerate it with:

```bash
go tool swag init -q --output ./swagger --generalInfo ./core/app.go
```

This is now wired into `make check`, so a stale spec (annotations edited without regenerating) will show up as a diff in `swagger/` during review rather than silently drifting from the actual handlers.
