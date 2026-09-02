-- name: GetDocumentChunkByID :one
SELECT * FROM document_chunks WHERE id = $1;

-- name: ListDocumentChunksByDocumentID :many
SELECT * FROM document_chunks WHERE document_id = $1 ORDER BY sequence_number ASC;

-- name: ListDocumentChunksByDocumentIDAndPage :many
SELECT * FROM document_chunks
WHERE document_id = $1 AND page = $2
ORDER BY sequence_number ASC;

-- name: ListChunksNeedingEmbedding :many
SELECT * FROM document_chunks
WHERE document_id = $1 AND text != '' AND embedding IS NULL
ORDER BY sequence_number ASC;

-- name: ListChunksNeedingKnowledgeExtraction :many
SELECT * FROM document_chunks
WHERE document_id = $1 AND text != '' AND knowledge_extracted_at IS NULL
ORDER BY sequence_number ASC;

-- name: ListChunksNeedingTranslation :many
SELECT * FROM document_chunks
WHERE document_id = $1 AND text != '' AND text_translated IS NULL
ORDER BY sequence_number ASC;

-- name: ListDocumentIDsNeedingTranslation :many
SELECT DISTINCT document_id FROM document_chunks
WHERE text != '' AND text_translated IS NULL;

-- name: MarkChunkKnowledgeExtracted :exec
UPDATE document_chunks SET knowledge_extracted_at = now() WHERE id = $1;

-- name: UpdateChunkTranslation :exec
UPDATE document_chunks SET text_translated = $2, updated_at = $3 WHERE id = $1;

-- name: CreateDocumentChunksBatch :copyfrom
INSERT INTO document_chunks (
    id, created_at, updated_at, document_id, sequence_number, topics, factuality_score,
    text, page, chapter, layout_type, bounding_box, asset_url, language, text_translated,
    table_markdown
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
);

-- name: UpdateDocumentChunk :one
UPDATE document_chunks SET
    updated_at = $2,
    topics = $3,
    factuality_score = $4,
    text = $5,
    chapter = $6,
    asset_url = $7,
    embedding = $8
WHERE id = $1
RETURNING *;
