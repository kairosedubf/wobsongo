package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/core"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/testhelpers"
	"github.com/labstack/echo/v4"
)

const claimsCheckPath = "/api/v1/claims/check"

// stubEmbedder, stubAnalyzer, and stubJudge are hand-rolled test doubles for
// the provider-agnostic interfaces (data.Embedder/ClaimAnalyzer/ClaimJudge)
// — these don't get moq-generated mocks in this codebase (only the repo
// interfaces in internal/mockrepo do); mirrors internal/service/rag_test.go's
// stubEmbedder pattern.
type stubEmbedder struct{ vector []float32 }

func (s *stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i := range vectors {
		vectors[i] = s.vector
	}
	return vectors, nil
}

type stubAnalyzer struct{ analysis *data.ClaimAnalysis }

func (s *stubAnalyzer) Analyze(context.Context, string) (*data.ClaimAnalysis, error) {
	return s.analysis, nil
}

type stubJudge struct{ verdict *data.JudgeVerdict }

func (s *stubJudge) Judge(context.Context, *data.JudgeRequest) (*data.JudgeVerdict, error) {
	return s.verdict, nil
}

func newClaimTestApp(
	chunkRepo data.DocumentChunkRepoer,
	knowledgeRepo data.AtomicKnowledgeRepoer,
	analyzer data.ClaimAnalyzer,
	judge data.ClaimJudge,
) *echo.Echo {
	app := core.NewApp(
		testhelpers.NewEcho(),
		config.NewConfig(),
		core.WithChunkRepo(chunkRepo),
		core.WithKnowledgeRepo(knowledgeRepo),
		core.WithEmbedder(&stubEmbedder{vector: []float32{1}}),
		core.WithClaimAnalyzer(analyzer),
		core.WithClaimJudge(judge),
	)
	return app.Echo()
}

func TestCheckClaimHandler_OutOfScope(t *testing.T) {
	analyzer := &stubAnalyzer{
		analysis: &data.ClaimAnalysis{InScope: false, RefusalReason: "not health-related"},
	}
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}

	app := newClaimTestApp(chunkRepo, knowledgeRepo, analyzer, &stubJudge{})

	body, err := json.Marshal(dto.CheckClaimDTO{Text: "what's the capital of France?"})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, claimsCheckPath, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp testhelpers.APIResponse[dto.ClaimCheckResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if resp.Data.InScope {
		t.Error("expected in_scope false")
	}
	if resp.Data.RefusalReason != "not health-related" {
		t.Errorf("expected refusal reason to propagate, got %q", resp.Data.RefusalReason)
	}
	if resp.Data.FormattedMessage != "" {
		t.Errorf(
			"expected no formatted message for out-of-scope input, got %q",
			resp.Data.FormattedMessage,
		)
	}
}

func TestCheckClaimHandler_Success(t *testing.T) {
	analyzer := &stubAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: "vitamin C prevents colds"}},
		},
	}
	judge := &stubJudge{
		verdict: &data.JudgeVerdict{
			Verdict:        model.VerdictSupported,
			Severity:       model.SeverityRoutine,
			Reasoning:      "the cited chunk backs this",
			BriefReasoning: "supported by the evidence",
			CitedEvidence:  []int{0},
		},
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, nil
	}
	chunkRepo.SearchByFullTextFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return []data.ScoredResult[model.DocumentChunk]{
			{
				Item: model.DocumentChunk{
					ParsedChunk: model.ParsedChunk{Text: "relevant passage"},
				},
				Score: 0.9,
			},
		}, nil
	}

	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}
	knowledgeRepo.SearchByFullTextFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}
	knowledgeRepo.SearchBySimilarityFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}

	app := newClaimTestApp(chunkRepo, knowledgeRepo, analyzer, judge)

	body, err := json.Marshal(dto.CheckClaimDTO{Text: "vitamin C prevents colds"})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, claimsCheckPath, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp testhelpers.APIResponse[dto.ClaimCheckResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if !resp.Data.InScope {
		t.Fatal("expected in_scope true")
	}
	if len(resp.Data.SubClaims) != 1 {
		t.Fatalf("expected 1 sub-claim, got %d", len(resp.Data.SubClaims))
	}
	if resp.Data.SubClaims[0].Verdict != "supported" {
		t.Errorf("expected verdict %q, got %q", "supported", resp.Data.SubClaims[0].Verdict)
	}
	if resp.Data.SubClaims[0].BriefReasoning != "supported by the evidence" {
		t.Errorf(
			"expected brief_reasoning %q, got %q",
			"supported by the evidence",
			resp.Data.SubClaims[0].BriefReasoning,
		)
	}
	// IsLong wasn't set on the request — default is short, so
	// FormattedMessage uses BriefReasoning, not the full Reasoning, and
	// carries no citations.
	wantMessage := "✅ supported — 1 claim checked\n\n" +
		"✅ 1. vitamin C prevents colds\n" +
		"supported by the evidence"
	if resp.Data.FormattedMessage != wantMessage {
		t.Errorf("expected formatted message %q, got %q", wantMessage, resp.Data.FormattedMessage)
	}
}

func TestCheckClaimHandler_Success_LongMode(t *testing.T) {
	analyzer := &stubAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: "vitamin C prevents colds"}},
		},
	}
	judge := &stubJudge{
		verdict: &data.JudgeVerdict{
			Verdict:        model.VerdictSupported,
			Severity:       model.SeverityRoutine,
			Reasoning:      "the cited chunk backs this",
			BriefReasoning: "supported by the evidence",
			CitedEvidence:  []int{0},
		},
	}

	evidenceChunkID := uuid.New()
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, nil
	}
	chunkRepo.SearchByFullTextFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return []data.ScoredResult[model.DocumentChunk]{
			{
				Item: model.DocumentChunk{
					ID:          evidenceChunkID,
					ParsedChunk: model.ParsedChunk{Text: "relevant passage"},
				},
				Score: 0.9,
			},
		}, nil
	}

	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}
	knowledgeRepo.SearchByFullTextFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}
	knowledgeRepo.SearchBySimilarityFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}

	app := newClaimTestApp(chunkRepo, knowledgeRepo, analyzer, judge)

	body, err := json.Marshal(dto.CheckClaimDTO{Text: "vitamin C prevents colds", IsLong: true})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, claimsCheckPath, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp testhelpers.APIResponse[dto.ClaimCheckResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}

	// IsLong was true — FormattedMessage uses the full Reasoning and appends
	// the cited evidence, unlike the short-mode test above.
	wantMessage := "✅ supported — 1 claim checked\n\n" +
		"✅ 1. vitamin C prevents colds\n" +
		"the cited chunk backs this\n" +
		"  [0] (chunk) relevant passage"
	if resp.Data.FormattedMessage != wantMessage {
		t.Errorf("expected formatted message %q, got %q", wantMessage, resp.Data.FormattedMessage)
	}

	if len(resp.Data.SubClaims) != 1 || len(resp.Data.SubClaims[0].Citations) != 1 {
		t.Fatalf("expected 1 sub-claim with 1 citation, got %+v", resp.Data.SubClaims)
	}
	if resp.Data.SubClaims[0].Citations[0].ChunkID != evidenceChunkID {
		t.Errorf(
			"expected citation chunk_id %s, got %s",
			evidenceChunkID, resp.Data.SubClaims[0].Citations[0].ChunkID,
		)
	}
}

func TestCheckClaimHandler_MissingText(t *testing.T) {
	app := newClaimTestApp(
		&mockrepo.DocumentChunkRepoerMock{},
		&mockrepo.AtomicKnowledgeRepoerMock{},
		&stubAnalyzer{},
		&stubJudge{},
	)

	req := httptest.NewRequest(http.MethodPost, claimsCheckPath, bytes.NewReader([]byte(`{}`)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusUnprocessableEntity,
			rec.Code,
			rec.Body.String(),
		)
	}
}
