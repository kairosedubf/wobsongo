# Documents

CRUD for documents in the knowledge base. Creating a document is what kicks off the async ingestion pipeline (parsing, chunking, embedding, knowledge extraction) — see [Background Jobs](../developer/background-jobs.md) for what happens after a `201`.

Upload the file first via [Media](media.md) to get a `file_key`, then create the document record.

## Create — `POST /api/v1/documents`

Request body (`dto.CreateDocumentDTO`):

```json
{
  "sha256": "…",
  "file_key": "documents/report.pdf",
  "title": "WHO Guidance on X",
  "filename": "report.pdf",
  "filetype": "application/pdf",
  "filesize": 483920,
  "page_count": 12,
  "publisher_name": "WHO",
  "publication_year": 2024,
  "language": "en"
}
```

- `language` must be `"en"` or `"fr"` — it's required, not auto-detected.
- `file_key` must be a key already uploaded via the media-upload flow.

Returns `201` with the created `model.Document` on success. `400` for a malformed body, `422` for a validation failure (missing/invalid fields), `500` otherwise.

## List — `GET /api/v1/documents`

Query params: `page` (default 1), `per_page` (default 20, max 100) — see [Rate Limits](rate-limits.md) for the pagination cap.

Returns `200` with a `dto.PaginationResults[model.Document]`:

```json
{
  "data": {
    "page": 1,
    "per_page": 20,
    "total_items": 42,
    "items": [ { "id": "...", "title": "...", "...": "..." } ]
  }
}
```

## Get — `GET /api/v1/documents/{id}`

Returns `200` with a single `model.Document`, or `404` if the ID doesn't exist.

## Update — `PUT /api/v1/documents/{id}`

Request body (`dto.UpdateDocumentDTO`) — only descriptive metadata is mutable; the underlying file, hash, and other physical attributes are immutable once ingested:

```json
{ "title": "Updated title", "publisher_name": "WHO", "publication_year": 2024 }
```

Returns `200` with the updated `model.Document`. `404` if not found, `409` on a conflict, `422` on validation failure.

## Delete — `DELETE /api/v1/documents/{id}`

Returns `204 No Content` on success, `404` if not found.

## Errors

All endpoints share the [standard error shape](errors.md). Most document-specific failures surface as `404` (unknown ID) or `422` (validation — e.g. bad `language` value).
