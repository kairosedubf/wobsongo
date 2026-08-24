# Web UI

A walkthrough of every screen, in the order they appear in the sidebar. All screens except **Evidence** require logging in (`RequireAuthMiddleware`) — none are currently gated further by role, even though the code has an unused `RequireRoleMiddleware` reserved for future admin-only pages.

## Dashboard

*(Screenshot: dashboard — to be added)*

Currently a minimal placeholder: a welcome message with your name and a note that there's nothing here yet. It's not a metrics/summary dashboard today — don't expect stats or activity feeds.

## Documents

*(Screenshot: documents list — to be added)*

A paginated table (20/page) of every ingested document: Title (falls back to filename if untitled), Type (MIME type), Size, Language, Status (**Ingested** once processing has completed, **Pending** otherwise), and Uploaded date, with Edit/Delete actions per row.

*(Screenshot: new document form — to be added)*

**Uploading a new document** here is a plain, synchronous form submission — you pick a file (PDF, Word, RTF, HTML, Markdown, or an image; 50MB max) and fill in Title, Publisher, Publication year, and required Language (English/French), then submit. This uploads the file directly as part of the form POST and hashes it server-side.

!!! note "This differs from the REST API's upload flow"
    The API's document-creation endpoint expects you to already have a `file_key` from a separate presigned-URL upload step (see [Media](../api/media.md)) — the web UI does not use that two-step flow. If you're scripting ingestion, follow the API docs, not this page's upload mechanic.

**Editing** a document only lets you change Title, Publisher, and Publication year — the file itself and its Language are immutable once uploaded.

## Chunks

*(Screenshot: chunks view — to be added)*

Pick a document from the dropdown to see its chunks, one PDF page at a time (with a page-jump selector for documents with multiple pages). Each row shows the chunk's sequence number, layout type (paragraph, table, picture, etc.), language, and a truncated text preview. Selecting or hovering a row highlights that chunk's exact location on the page in an embedded PDF viewer alongside the table.

## Knowledge

*(Screenshot: knowledge view — to be added)*

Same document-picker pattern as Chunks. Shows the atomic-knowledge facts extracted from that document's chunks: Subject, Predicate, Object, Truth Tier, Category, and when it was extracted. This view is read-only — there's currently no way to flag or invalidate a fact from the UI (the underlying data model supports marking a fact invalid, but nothing surfaces that control here yet).

## Retrieval

*(Screenshot: retrieval search — to be added)*

A search box that runs the same hybrid retrieval claim-checking uses internally, directly exposed as a standalone tool — useful for exploring what's in the knowledge base or understanding why a claim check turned out the way it did. Each result is tagged as a chunk or a fact hit, shown alongside its source document, page (or truth tier, for a fact hit), and a text preview; hovering a result shows its match in the same embedded PDF viewer used on the Chunks page.

## Check

*(Screenshot: claim check form — to be added)*

The claim-checking form: enter your claim or question, choose Short or Long detail level, and submit. The result mirrors the API's claim-check response exactly — an overall summary, then one card per sub-claim with its verdict, severity, and reasoning (brief or full, depending on the detail level you chose). In Long mode, each card also lists its citations, linking to the evidence viewer below. See [Claims](../api/claims.md) for what each verdict/severity value means.

## Evidence viewer

*(Screenshot: evidence viewer — to be added)*

A standalone page (no sidebar) reached by clicking a citation from the Check page — not something you navigate to directly. It shows the exact page of the source PDF with the cited chunk's location highlighted, so you can verify a claim's evidence in its original context. PDF documents only.

## Login / Register

Standard email/password authentication. Registration requires a password of at least 8 characters with at least two of: letters, digits, symbols.
