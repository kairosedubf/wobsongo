# Errors

Handlers return errors as `model.APIError{Code, Internal, Public}`. `Code` is the HTTP status to send, `Public` is the message that ends up in the response envelope's `error` field, and `Internal` is the (unexposed) underlying error used only for server-side logging.

```json
{
  "request_id": "018f2e2b-...",
  "status": 404,
  "data": null,
  "error": "The requested resource could not be found"
}
```

## Status codes you'll see

| Status | When |
|---|---|
| `400 Bad Request` | The request body couldn't be bound/parsed (`invalid request body`). |
| `422 Unprocessable Entity` | The body parsed but failed validation (`go-playground/validator` rules on the DTO). |
| `404 Not Found` | The requested resource doesn't exist. |
| `409 Conflict` | The write conflicts with an existing resource. |
| `403 Forbidden` | Insufficient permissions for the operation. |
| `500 Internal Server Error` | An unexpected internal error, or any service-layer error not mapped to one of the above. |

The 404/409/403/500 mapping comes from a fixed sentinel-error table in `handler/errors.go` (`data.ErrNotFound`, `data.ErrConflict`, `data.ErrForbidden`, `data.ErrInternal`) — anything else falls back to a generic 500 rather than leaking internal error details.

There is no separate machine-readable error code/taxonomy beyond the HTTP status — build client-side error handling around `status` (and, for validation failures, expect to re-check your request against the endpoint's documented fields).
