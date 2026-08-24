# API Overview

Wobsongo exposes a versioned REST API under `/api/v1`, mounted alongside the web UI on the same Echo instance (`e.Group("/api")` in `core/app.go`). It's the surface to use once the bundled web UI's forms aren't enough — e.g. ingesting documents programmatically, or checking a claim from a bot or another service.

## Base path

```
https://<your-deployment-host>/api/v1
```

## Middleware

Every `/api/v1/*` route currently gets only the global middleware stack applied to the whole app:

- panic recovery
- a request ID (returned as the `X-Request-ID` response header)
- CORS (allowed origins configured via `CORS_ALLOWED_ORIGINS`)
- request logging

No authentication or rate limiting is applied at this layer today — see [Authentication](authentication.md) and [Rate Limits](rate-limits.md) for the specifics before you rely on either.

## Response envelope

Every handler returns the same JSON shape:

```json
{
  "request_id": "018f2e2b-...",
  "status": 200,
  "data": { "...": "..." },
  "error": ""
}
```

- `request_id` — echoes the `X-Request-ID` header, useful for correlating a support request with server logs.
- `status` — the HTTP status code, duplicated in the body.
- `data` — present on success, omitted on error.
- `error` — a short public-safe message, present only on error.

See [Errors](errors.md) for the full set of status codes and messages.

## Resources

| Resource | Docs |
|---|---|
| Documents (ingest, list, update, delete) | [Documents](documents.md) |
| Media (presigned upload/download URLs) | [Media](media.md) |
| Claim checking | [Claims](claims.md) |

## Machine-readable spec

An OpenAPI (Swagger 2.0) spec is generated from the handler annotations — see [OpenAPI Spec](openapi.md).
