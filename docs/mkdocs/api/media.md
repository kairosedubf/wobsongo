# Media

Two endpoints handle object storage (S3/MinIO, depending on `STORAGE_PROVIDER`) indirectly — the API never proxies file bytes itself, it only issues presigned URLs the client uploads/downloads with directly.

## Upload flow

1. **`GET /api/v1/media/upload`** — request a presigned POST policy before uploading a file.

   | Query param | Required | Notes |
   |---|---|---|
   | `intent` | yes | `document` or `document_image` — determines the storage prefix (`documents/`, `document_images/`). |
   | `filename` | yes | The filename you intend to upload. |
   | `content_type` | yes | MIME type of the file. |

   Response (`200`):
   ```json
   {
     "data": {
       "url": "https://s3.example.com/bucket",
       "prefix": "documents/",
       "formFields": { "key": "documents/report.pdf", "policy": "...", "...": "..." }
     }
   }
   ```

   POST the actual file directly to `url` as `multipart/form-data`, including every key in `formFields` plus a `file` field with the file content. The resulting object key is `prefix + filename` (e.g. `documents/report.pdf`) — that's the value to use as `file_key` when creating a document (see [Documents](documents.md)).

   Errors: `404`/`409` if the intent/filename combination can't be prepared, `422` for an invalid `intent` or malformed `filename`.

2. **`GET /api/v1/media/presigned-url`** — get a temporary signed GET URL for an existing object.

   | Query param | Required | Notes |
   |---|---|---|
   | `s3_key` | yes | Full object key, e.g. `documents/report.pdf`. |
   | `ttl` | no | Seconds until expiry. Default `900` (15 min), max `86400` (24h). |

   Response (`200`):
   ```json
   { "data": { "presigned_url": "https://s3.example.com/bucket/documents/report.pdf?...", "ttl": 900 } }
   ```

   Errors: `400` for an invalid S3 key, `422` for a validation failure, `500` on an internal error.
