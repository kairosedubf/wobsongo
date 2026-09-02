package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
)

// DocumentChunkRepo is a Postgres-backed implementation of data.DocumentChunkRepoer.
type DocumentChunkRepo struct {
	q           *db.Queries
	pool        *pgxpool.Pool
	riverClient func() *river.Client[pgx.Tx]
	tx          pgx.Tx // set only on the tx-scoped instance WithTx constructs; nil otherwise
}

// Ensure DocumentChunkRepo implements data.DocumentChunkRepoer.
var _ data.DocumentChunkRepoer = (*DocumentChunkRepo)(nil)

// NewDocumentChunkRepo creates a new Postgres-backed document chunk repository.
// q is accepted externally (not built internally from pool) so callers
// (including tests) can supply a tx-scoped *db.Queries.
//
// riverClient is a lazy getter, not the client itself: this repo is
// constructed and handed to a worker (ProcessParsedDocumentWorker) before
// river.NewClient exists — River requires all workers registered via
// river.AddWorker before that call. The closure is only invoked when
// Enqueue actually runs, which is always well after river.NewClient (and
// riverClient.Start) has completed. Pass a closure over a variable that's
// assigned once the real client is built, e.g.:
//
//	var riverClient *river.Client[pgx.Tx]
//	chunkRepo := repo.NewDocumentChunkRepo(q, pool, func() *river.Client[pgx.Tx] { return riverClient })
//	// ... register workers using chunkRepo ...
//	riverClient, err = river.NewClient(...)
func NewDocumentChunkRepo(
	q *db.Queries,
	pool *pgxpool.Pool,
	riverClient func() *river.Client[pgx.Tx],
) data.DocumentChunkRepoer {
	return &DocumentChunkRepo{q: q, pool: pool, riverClient: riverClient}
}

// GetByID retrieves a document chunk by its ID.
func (r *DocumentChunkRepo) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.DocumentChunk, error) {
	chunk, err := r.q.GetDocumentChunkByID(ctx, id)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	return toModelDocumentChunk(&chunk), nil
}

// ListByDocumentID retrieves all chunks for a document, ordered by SequenceNumber.
func (r *DocumentChunkRepo) ListByDocumentID(
	ctx context.Context,
	documentID uuid.UUID,
) ([]model.DocumentChunk, error) {
	rows, err := r.q.ListDocumentChunksByDocumentID(ctx, documentID)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	chunks := make([]model.DocumentChunk, 0, len(rows))
	for i := range rows {
		chunks = append(chunks, *toModelDocumentChunk(&rows[i]))
	}
	return chunks, nil
}

// ListChunksNeedingEmbedding retrieves chunks for a document that have text
// but no embedding yet, ordered by SequenceNumber.
func (r *DocumentChunkRepo) ListChunksNeedingEmbedding(
	ctx context.Context,
	documentID uuid.UUID,
) ([]model.DocumentChunk, error) {
	rows, err := r.q.ListChunksNeedingEmbedding(ctx, documentID)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	chunks := make([]model.DocumentChunk, 0, len(rows))
	for i := range rows {
		chunks = append(chunks, *toModelDocumentChunk(&rows[i]))
	}
	return chunks, nil
}

// ListChunksNeedingKnowledgeExtraction retrieves chunks for a document that
// have text but haven't had atomic-knowledge extraction run yet, ordered by
// SequenceNumber.
func (r *DocumentChunkRepo) ListChunksNeedingKnowledgeExtraction(
	ctx context.Context,
	documentID uuid.UUID,
) ([]model.DocumentChunk, error) {
	rows, err := r.q.ListChunksNeedingKnowledgeExtraction(ctx, documentID)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	chunks := make([]model.DocumentChunk, 0, len(rows))
	for i := range rows {
		chunks = append(chunks, *toModelDocumentChunk(&rows[i]))
	}
	return chunks, nil
}

// ListChunksNeedingTranslation retrieves chunks for a document that have text
// but haven't been translated yet, ordered by SequenceNumber.
func (r *DocumentChunkRepo) ListChunksNeedingTranslation(
	ctx context.Context,
	documentID uuid.UUID,
) ([]model.DocumentChunk, error) {
	rows, err := r.q.ListChunksNeedingTranslation(ctx, documentID)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	chunks := make([]model.DocumentChunk, 0, len(rows))
	for i := range rows {
		chunks = append(chunks, *toModelDocumentChunk(&rows[i]))
	}
	return chunks, nil
}

// ListByDocumentIDAndPage retrieves a document's chunks on exactly one page,
// ordered by SequenceNumber.
func (r *DocumentChunkRepo) ListByDocumentIDAndPage(
	ctx context.Context,
	documentID uuid.UUID,
	page int,
) ([]model.DocumentChunk, error) {
	rows, err := r.q.ListDocumentChunksByDocumentIDAndPage(
		ctx,
		db.ListDocumentChunksByDocumentIDAndPageParams{
			DocumentID: documentID,
			Page:       toInt32(page),
		},
	)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	chunks := make([]model.DocumentChunk, 0, len(rows))
	for i := range rows {
		chunks = append(chunks, *toModelDocumentChunk(&rows[i]))
	}
	return chunks, nil
}

// UpdateChunkTranslation persists a chunk's translated text.
func (r *DocumentChunkRepo) UpdateChunkTranslation(
	ctx context.Context,
	chunkID uuid.UUID,
	textTranslated string,
) error {
	if err := r.q.UpdateChunkTranslation(ctx, db.UpdateChunkTranslationParams{
		ID:             chunkID,
		TextTranslated: toPgText(textTranslated),
		UpdatedAt:      time.Now(),
	}); err != nil {
		return mapPostgresError(err)
	}
	return nil
}

// ListDocumentIDsNeedingTranslation returns the IDs of every document that
// has at least one chunk with text but no translation yet.
func (r *DocumentChunkRepo) ListDocumentIDsNeedingTranslation(
	ctx context.Context,
) ([]uuid.UUID, error) {
	ids, err := r.q.ListDocumentIDsNeedingTranslation(ctx)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	return ids, nil
}

// CreateBatch inserts multiple fully-formed chunks in a single COPY operation.
func (r *DocumentChunkRepo) CreateBatch(ctx context.Context, chunks []model.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	params := make([]db.CreateDocumentChunksBatchParams, len(chunks))
	for i := range chunks {
		params[i] = toCreateDocumentChunksBatchParams(&chunks[i])
	}

	if _, err := r.q.CreateDocumentChunksBatch(ctx, params); err != nil {
		return mapPostgresError(err)
	}
	return nil
}

// Update saves an existing document chunk.
func (r *DocumentChunkRepo) Update(ctx context.Context, chunk *model.DocumentChunk) error {
	updated, err := r.q.UpdateDocumentChunk(ctx, db.UpdateDocumentChunkParams{
		ID:              chunk.ID,
		UpdatedAt:       chunk.UpdatedAt,
		Topics:          chunk.Topics,
		FactualityScore: chunk.FactualityScore,
		Text:            chunk.Text,
		Chapter:         chunk.Chapter,
		AssetUrl:        chunk.AssetURL,
		Embedding:       toPgvector(chunk.Embedding),
	})
	if err != nil {
		return mapPostgresError(err)
	}
	*chunk = *toModelDocumentChunk(&updated)
	return nil
}

// ShouldBeStored decides whether a chunk carries enough information/context
// to be worth persisting. Reference-layout chunks (bibliography/citation
// entries) are structurally reliable noise — always bibliographic, never
// clinical — so they're dropped before ever being stored, embedded, or
// extracted from. Footnotes are deliberately left alone: they sometimes carry
// substantive caveats, not just citations, unlike references. doc isn't
// consumed by this check yet; it's threaded through for future
// heuristic/LLM-based storage decisions that do need document-level context.
//
//nolint:gocritic // doc/chunk are passed by value: fixed by the data.DocumentChunkRepoer interface signature.
func (r *DocumentChunkRepo) ShouldBeStored(
	_ context.Context,
	_ model.Document,
	chunk model.DocumentChunk,
) (bool, error) {
	return chunk.LayoutType != model.LayoutTypeReference, nil
}

// SearchByEmbedding returns the limit chunks whose embedding is closest
// (cosine distance) to queryVector, ordered nearest-first.
func (r *DocumentChunkRepo) SearchByEmbedding(
	ctx context.Context,
	queryVector []float32,
	limit int,
) ([]data.ScoredResult[model.DocumentChunk], error) {
	return searchScored(
		ctx,
		r.pool,
		`SELECT id, embedding <=> $1 AS score FROM document_chunks
		 WHERE embedding IS NOT NULL
		 ORDER BY score ASC
		 LIMIT $2`,
		[]any{pgvector.NewVector(queryVector), limit},
		r.GetByID,
	)
}

// SearchByFullText returns the limit chunks whose text best matches query via
// Postgres full-text search (ts_rank_cd), ordered best-first. Matches
// against both the English and French tsvector columns and takes the best of
// the two — query language isn't known up front, and a chunk's own language
// may differ from the query's, so this is what makes cross-lingual full-text
// search work without the caller needing to specify a language.
func (r *DocumentChunkRepo) SearchByFullText(
	ctx context.Context,
	query string,
	limit int,
) ([]data.ScoredResult[model.DocumentChunk], error) {
	return searchScored(
		ctx,
		r.pool,
		`SELECT id, GREATEST(
		     ts_rank_cd(text_fts_en, websearch_to_tsquery('english', $1)),
		     ts_rank_cd(text_fts_fr, websearch_to_tsquery('french', $1))
		 ) AS score
		 FROM document_chunks
		 WHERE text_fts_en @@ websearch_to_tsquery('english', $1)
		    OR text_fts_fr @@ websearch_to_tsquery('french', $1)
		 ORDER BY score DESC
		 LIMIT $2`,
		[]any{query, limit},
		r.GetByID,
	)
}

// WithTx executes fn within a Postgres transaction, giving it a
// transaction-scoped repo whose Enqueue calls are part of the same
// transaction as any CRUD calls it makes.
func (r *DocumentChunkRepo) WithTx(
	ctx context.Context,
	fn func(data.DocumentChunkRepoer) error,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("document chunk repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := &DocumentChunkRepo{
		q:           r.q.WithTx(tx),
		pool:        r.pool,
		riverClient: r.riverClient,
		tx:          tx,
	}

	if err := fn(txRepo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Enqueue adds a job to the queue, using the open transaction when called
// from within WithTx so the job insert is atomic with any CRUD writes.
func (r *DocumentChunkRepo) Enqueue(ctx context.Context, payload queue.BackgroundJob) error {
	if r.tx != nil {
		_, err := r.riverClient().InsertTx(ctx, r.tx, payload, nil)
		return err
	}
	_, err := r.riverClient().Insert(ctx, payload, nil)
	return err
}

// toModelDocumentChunk maps a sqlc-generated db.DocumentChunk row to model.DocumentChunk.
func toModelDocumentChunk(d *db.DocumentChunk) *model.DocumentChunk {
	return &model.DocumentChunk{
		ID:                   d.ID,
		CreatedAt:            d.CreatedAt,
		UpdatedAt:            d.UpdatedAt,
		DocumentID:           d.DocumentID,
		SequenceNumber:       int(d.SequenceNumber),
		Topics:               d.Topics,
		FactualityScore:      d.FactualityScore,
		Embedding:            fromPgvector(d.Embedding),
		KnowledgeExtractedAt: fromPgTimestamptz(d.KnowledgeExtractedAt),
		Language:             model.Language(d.Language),
		TextTranslated:       fromPgText(d.TextTranslated),
		ParsedChunk: model.ParsedChunk{
			Text:          d.Text,
			TableMarkdown: d.TableMarkdown,
			Page:          int(d.Page),
			LayoutType:    model.LayoutType(d.LayoutType),
			BoundingBox:   toBoundingBox(d.BoundingBox),
			AssetURL:      d.AssetUrl,
		},
	}
}

// toPgvector converts a nullable []float32 embedding into a *pgvector.Vector
// query param. A nil/empty vec becomes a nil pointer (SQL NULL) rather than
// an empty pgvector.Vector — Postgres rejects a zero-dimension vector value
// outright ("vector must have at least 1 dimension"), so "not yet embedded"
// must be represented as NULL, never as a zero-length vector.
func toPgvector(vec []float32) *pgvector.Vector {
	if len(vec) == 0 {
		return nil
	}
	v := pgvector.NewVector(vec)
	return &v
}

// fromPgvector converts a scanned *pgvector.Vector column back into a
// nullable []float32. A nil pointer (SQL NULL, i.e. not yet embedded) maps
// to a nil slice.
func fromPgvector(v *pgvector.Vector) []float32 {
	if v == nil {
		return nil
	}
	return v.Slice()
}

// toPgText converts a model-level "empty string means absent" value into a
// pgtype.Text, so a genuinely-untranslated chunk/fact is stored as SQL NULL
// (matching the text_translated/search_text_translated IS NULL idempotency
// filter) rather than an empty string.
func toPgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: s, Valid: true}
}

// fromPgText converts a scanned pgtype.Text column back into the model's
// "empty string means absent" convention. A SQL NULL (not yet translated)
// maps to "".
func fromPgText(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

// toCreateDocumentChunksBatchParams maps a model.DocumentChunk to sqlc's batch-insert params.
func toCreateDocumentChunksBatchParams(c *model.DocumentChunk) db.CreateDocumentChunksBatchParams {
	return db.CreateDocumentChunksBatchParams{
		ID:              c.ID,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
		DocumentID:      c.DocumentID,
		SequenceNumber:  toInt32(c.SequenceNumber),
		Topics:          c.Topics,
		FactualityScore: c.FactualityScore,
		Text:            c.Text,
		TableMarkdown:   c.TableMarkdown,
		Page:            toInt32(c.Page),
		Chapter:         c.Chapter,
		LayoutType:      string(c.LayoutType),
		BoundingBox:     c.BoundingBox[:],
		AssetUrl:        c.AssetURL,
		Language:        toInt32(int(c.Language)),
		TextTranslated:  toPgText(c.TextTranslated),
	}
}

// toBoundingBox converts a Postgres float8[] column into model.BoundingBox,
// defaulting to the zero value if the array isn't exactly 4 elements.
func toBoundingBox(v []float64) model.BoundingBox {
	var bbox model.BoundingBox
	copy(bbox[:], v)
	return bbox
}
