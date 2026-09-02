package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
	"golang.org/x/sync/errgroup"
)

// rrfK is Reciprocal Rank Fusion's damping constant — the standard default
// (matching Elasticsearch's own RRF implementation). Larger values flatten
// the difference between a top and a middling rank within one method's list.
const rrfK = 60.0

// RAGResult is one hybrid-search hit, normalized across both sources
// (document chunks and atomic-knowledge facts) so results from either can be
// ranked and displayed together.
type RAGResult struct {
	// Key identifies the underlying row for fusion/dedup: "chunk:<uuid>" or
	// "fact:<uuid>" — a chunk or fact hit by more than one retrieval method
	// is one RAGResult, not several.
	Key string
	// Source is "chunk" or "fact".
	Source string
	// Methods lists which retrieval method(s) produced this hit: any of
	// "vector", "fts", "trgm". More than one entry means the same row was
	// found by multiple methods, which RRF rewards.
	Methods []string
	// RRFScore is the fused ranking score (higher = more relevant). Not
	// comparable to any individual method's native score — see fuseRRF.
	RRFScore   float64
	DocumentID uuid.UUID
	// Text is the chunk's Text, or the fact's SPOText() — the exact string
	// that was embedded, so displayed text matches what search matched on.
	Text string
	// Page is set for chunk hits only; zero for fact hits.
	Page int
	// TruthTier is set for fact hits only; empty for chunk hits.
	TruthTier string
	// BoundingBox, LayoutType, and SequenceNumber come from the source
	// chunk — the hit's own chunk for chunk-source results, or the hydrated
	// parent chunk for fact-source results (see hydrateFactChunks). Lets a
	// caller (e.g. the Retrieval web page) reuse the same PDF-bbox-viewer
	// row wiring as the /chunks page for every result, regardless of source.
	BoundingBox    model.BoundingBox
	LayoutType     model.LayoutType
	SequenceNumber int
	// Language is the source chunk's/fact's own language — useful for
	// debugging cross-lingual retrieval, since a hit's language doesn't have
	// to match the query's.
	Language model.Language
	// ChunkText is the source chunk's full text, hydrated for fact-source
	// hits only (see hydrateFactChunks) — gives a downstream consumer (e.g. a
	// claim judge) the surrounding context a bare SPO fact doesn't carry on
	// its own. Empty for chunk-source hits, which already ARE the chunk.
	ChunkText string
	// TableMarkdown is the complete Markdown representation for table-backed
	// hits, kept separate from the text used for retrieval and judging.
	TableMarkdown string
	// ChunkID is the chunk this hit traces back to — the hit's own ID for
	// chunk-source results, or the parent chunk's ID for fact-source results
	// (see hydrateFactChunks). Lets a caller (e.g. a claim check's citations)
	// link back to a specific chunk regardless of which source produced the
	// hit.
	ChunkID uuid.UUID
}

// RAGService performs hybrid search across document chunks and atomic-
// knowledge facts: vector similarity, full-text search, and (for facts)
// trigram fuzzy matching over subject/predicate/object — fused into one
// ranked list via Reciprocal Rank Fusion. A real reranker model is a natural
// next step after fusion (see Search), not built here.
type RAGService struct {
	chunkRepo     data.DocumentChunkRepoer
	knowledgeRepo data.AtomicKnowledgeRepoer
	embedder      data.Embedder
}

// NewRAGService is a constructor for RAGService.
func NewRAGService(
	chunkRepo data.DocumentChunkRepoer,
	knowledgeRepo data.AtomicKnowledgeRepoer,
	embedder data.Embedder,
) *RAGService {
	return &RAGService{
		chunkRepo:     chunkRepo,
		knowledgeRepo: knowledgeRepo,
		embedder:      embedder,
	}
}

// Search embeds query once and runs it through five retrieval methods
// (chunk vector/FTS, fact vector/FTS/trigram) concurrently, fusing their
// ranked lists via RRF, and returns the top limit results overall.
func (s *RAGService) Search(ctx context.Context, query string, limit int) ([]RAGResult, error) {
	vectors, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, errors.New("embedder returned no vector for query")
	}
	queryVector := vectors[0]

	var (
		chunkVector, chunkFTS            []data.ScoredResult[model.DocumentChunk]
		factVector, factFTS, factTrigram []data.ScoredResult[model.AtomicKnowledge]
	)

	// errgroup.WithContext, not a plain group: unlike the LLM calls
	// elsewhere in this codebase (where one slow/failing call shouldn't
	// cancel its concurrently in-flight siblings), these are five fast local
	// Postgres queries — if one fails, canceling the rest and failing fast
	// is the right behavior, not wasted work.
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		chunkVector, err = s.chunkRepo.SearchByEmbedding(gctx, queryVector, limit)
		return err
	})
	g.Go(func() error {
		var err error
		chunkFTS, err = s.chunkRepo.SearchByFullText(gctx, query, limit)
		return err
	})
	g.Go(func() error {
		var err error
		factVector, err = s.knowledgeRepo.SearchByEmbedding(gctx, queryVector, limit)
		return err
	})
	g.Go(func() error {
		var err error
		factFTS, err = s.knowledgeRepo.SearchByFullText(gctx, query, limit)
		return err
	})
	g.Go(func() error {
		var err error
		factTrigram, err = s.knowledgeRepo.SearchBySimilarity(gctx, query, limit)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("hybrid search failed: %w", err)
	}

	fused := fuseRRF(
		mapChunkResults(chunkVector, "vector"),
		mapChunkResults(chunkFTS, "fts"),
		mapFactResults(factVector, "vector"),
		mapFactResults(factFTS, "fts"),
		mapFactResults(factTrigram, "trgm"),
	)

	// Reranker extension point: a cross-encoder/LLM rerank pass would slot
	// in here, re-scoring fused[:topK] before the final truncation below —
	// not built this pass (RRF fusion only).

	if len(fused) > limit {
		fused = fused[:limit]
	}

	if err := s.hydrateFactChunks(ctx, fused); err != nil {
		return nil, fmt.Errorf("failed to hydrate fact chunk context: %w", err)
	}

	return fused, nil
}

// hydrateFactChunks populates ChunkText for every fact-source result in
// results by fetching its parent chunk — only for the given (already fused
// and truncated to limit) results, not every raw candidate before fusion, so
// this costs at most limit single-row lookups regardless of how many
// candidates each of the five search methods returned.
func (s *RAGService) hydrateFactChunks(ctx context.Context, results []RAGResult) error {
	for i := range results {
		if results[i].Source != "fact" {
			continue
		}
		chunk, err := s.chunkRepo.GetByID(ctx, results[i].ChunkID)
		if err != nil {
			return fmt.Errorf("failed to fetch parent chunk %s: %w", results[i].ChunkID, err)
		}
		results[i].ChunkText = chunk.Text
		results[i].TableMarkdown = chunk.TableMarkdown
		results[i].Page = chunk.Page
		results[i].BoundingBox = chunk.BoundingBox
		results[i].LayoutType = chunk.LayoutType
		results[i].SequenceNumber = chunk.SequenceNumber
	}
	return nil
}

func mapChunkResults(
	results []data.ScoredResult[model.DocumentChunk],
	method string,
) []RAGResult {
	out := make([]RAGResult, len(results))
	for i, r := range results {
		out[i] = RAGResult{
			Key:            "chunk:" + r.Item.ID.String(),
			Source:         "chunk",
			Methods:        []string{method},
			DocumentID:     r.Item.DocumentID,
			Text:           r.Item.Text,
			TableMarkdown:  r.Item.TableMarkdown,
			Page:           r.Item.Page,
			Language:       r.Item.Language,
			BoundingBox:    r.Item.BoundingBox,
			LayoutType:     r.Item.LayoutType,
			SequenceNumber: r.Item.SequenceNumber,
			ChunkID:        r.Item.ID,
		}
	}
	return out
}

func mapFactResults(
	results []data.ScoredResult[model.AtomicKnowledge],
	method string,
) []RAGResult {
	out := make([]RAGResult, len(results))
	for i, r := range results {
		out[i] = RAGResult{
			Key:        "fact:" + r.Item.ID.String(),
			Source:     "fact",
			Methods:    []string{method},
			DocumentID: r.Item.DocumentID,
			Text:       r.Item.SPOText(),
			TruthTier:  r.Item.TruthTier.String(),
			Language:   r.Item.Language,
			ChunkID:    r.Item.DocumentChunkID,
		}
	}
	return out
}

// fuseRRF combines multiple ranked lists (each already sorted best-first)
// into one list ordered by combined Reciprocal Rank Fusion score: each item
// accumulates 1/(rrfK+rank) for every list it appears in (rank is 1-based),
// so an item found by several methods outranks one found by only one at a
// similar position — without needing the methods' native scores (cosine
// distance, ts_rank_cd, trigram similarity) to be comparable at all.
func fuseRRF(lists ...[]RAGResult) []RAGResult {
	scores := make(map[string]float64)
	seen := make(map[string]RAGResult)
	methodSets := make(map[string]map[string]bool)

	for _, list := range lists {
		for rank := range list {
			item := &list[rank]
			scores[item.Key] += 1.0 / (rrfK + float64(rank+1))
			if _, ok := seen[item.Key]; !ok {
				seen[item.Key] = *item
				methodSets[item.Key] = make(map[string]bool)
			}
			for _, m := range item.Methods {
				methodSets[item.Key][m] = true
			}
		}
	}

	fused := make([]RAGResult, 0, len(seen))
	for key, item := range seen { //nolint:gocritic // map values aren't addressable; copy-mutate-append is the pattern here, not an oversight
		item.RRFScore = scores[key]

		methods := make([]string, 0, len(methodSets[key]))
		for m := range methodSets[key] {
			methods = append(methods, m)
		}
		sort.Strings(methods) // deterministic display order
		item.Methods = methods

		fused = append(fused, item)
	}
	sort.Slice(fused, func(i, j int) bool { return fused[i].RRFScore > fused[j].RRFScore })
	return fused
}
