package service

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
)

// stubClaimAnalyzer is a hand-rolled data.ClaimAnalyzer for testing without a
// real analyzer endpoint — same pattern as stubEmbedder in rag_test.go.
type stubClaimAnalyzer struct {
	analysis *data.ClaimAnalysis
	err      error
}

func (s *stubClaimAnalyzer) Analyze(context.Context, string) (*data.ClaimAnalysis, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.analysis, nil
}

// stubClaimJudge is a hand-rolled data.ClaimJudge for testing. calls records
// every claim it was actually invoked with (mutex-protected since
// ClaimService judges sub-claims concurrently) — used to assert a sub-claim
// with no retrieved evidence never reaches the judge at all.
type stubClaimJudge struct {
	mu        sync.Mutex
	calls     []string
	judgeFunc func(req *data.JudgeRequest) (*data.JudgeVerdict, error)
}

func (s *stubClaimJudge) Judge(
	_ context.Context,
	req *data.JudgeRequest,
) (*data.JudgeVerdict, error) {
	s.mu.Lock()
	s.calls = append(s.calls, req.Claim)
	s.mu.Unlock()
	return s.judgeFunc(req)
}

func (s *stubClaimJudge) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// newEmptyRAGService returns a RAGService whose five search methods all
// return no results, for tests that don't care about retrieval content.
func newEmptyRAGService() *RAGService {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, nil
	}
	chunkRepo.SearchByFullTextFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, nil
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

	return NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1}})
}

// ragServiceWithAnyHit returns a RAGService whose full-text chunk search
// returns one hit for any query — for tests that need the judge to actually
// be called (as opposed to newEmptyRAGService's always-zero-hits shortcut).
func ragServiceWithAnyHit() *RAGService {
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
					ID:          uuid.New(),
					ParsedChunk: model.ParsedChunk{Text: "some evidence"},
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

	return NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1}})
}

func TestClaimService_CheckClaim_OutOfScopeShortCircuitsBeforeRetrieval(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{InScope: false, RefusalReason: "not health-related"},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			t.Fatal("judge should never be called for out-of-scope input")
			return nil, nil
		},
	}

	s := NewClaimService(analyzer, judge, newEmptyRAGService())
	result, err := s.CheckClaim(
		t.Context(),
		&dto.CheckClaimDTO{Text: "what's the capital of France?"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InScope {
		t.Error("expected InScope false")
	}
	if result.RefusalReason != "not health-related" {
		t.Errorf("expected refusal reason to propagate, got %q", result.RefusalReason)
	}
	if len(result.SubClaims) != 0 {
		t.Errorf("expected no sub-claims for out-of-scope input, got %d", len(result.SubClaims))
	}
	if judge.callCount() != 0 {
		t.Errorf("expected judge never called, got %d calls", judge.callCount())
	}
}

func TestClaimService_CheckClaim_ZeroEvidenceNeverReachesJudge(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: "an obscure unverifiable claim"}},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			t.Fatal("judge should never be called when retrieval returns no evidence")
			return nil, nil
		},
	}

	s := NewClaimService(analyzer, judge, newEmptyRAGService())
	result, err := s.CheckClaim(
		t.Context(),
		&dto.CheckClaimDTO{Text: "an obscure unverifiable claim"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.InScope {
		t.Fatal("expected InScope true")
	}
	if len(result.SubClaims) != 1 {
		t.Fatalf("expected 1 sub-claim result, got %d", len(result.SubClaims))
	}
	if result.SubClaims[0].Verdict != model.VerdictInsufficientEvidence {
		t.Errorf("expected InsufficientEvidence, got %s", result.SubClaims[0].Verdict)
	}
	if judge.callCount() != 0 {
		t.Errorf("expected judge never called, got %d calls", judge.callCount())
	}
	wantExplainer := insufficientEvidenceExplainerTemplates[model.LanguageFrench]
	if !strings.Contains(result.FormattedMessage, wantExplainer) {
		t.Errorf(
			"expected FormattedMessage to contain health-expert referral %q, got %q",
			wantExplainer,
			result.FormattedMessage,
		)
	}
}

func TestClaimService_CheckClaim_MultipleSubClaimsJudgedConcurrentlyAndAggregated(t *testing.T) {
	docID := uuid.New()
	factChunkID := uuid.New()

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, nil
	}
	chunkRepo.SearchByFullTextFunc = func(
		_ context.Context, query string, _ int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		if query == "claim A" {
			return []data.ScoredResult[model.DocumentChunk]{
				{
					Item: model.DocumentChunk{
						ID:          uuid.New(),
						DocumentID:  docID,
						ParsedChunk: model.ParsedChunk{Text: "supporting text"},
					},
					Score: 0.9,
				},
			}, nil
		}
		return nil, nil
	}
	// hydrateFactChunks fetches the "claim B" fact's parent chunk by ID.
	chunkRepo.GetByIDFunc = func(_ context.Context, id uuid.UUID) (*model.DocumentChunk, error) {
		if id != factChunkID {
			t.Fatalf("GetByID called with unexpected id %s", id)
		}
		return &model.DocumentChunk{
			ID:          factChunkID,
			DocumentID:  docID,
			ParsedChunk: model.ParsedChunk{Text: "contradicting passage"},
		}, nil
	}

	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}
	knowledgeRepo.SearchByFullTextFunc = func(
		_ context.Context, query string, _ int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		if query == "claim B" {
			return []data.ScoredResult[model.AtomicKnowledge]{
				{
					Item: model.AtomicKnowledge{
						ID:              uuid.New(),
						DocumentID:      docID,
						DocumentChunkID: factChunkID,
						TruthTier:       model.TruthTierAxiomatic,
					},
					Score: 0.9,
				},
			}, nil
		}
		return nil, nil
	}
	knowledgeRepo.SearchBySimilarityFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}

	rag := NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1}})

	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: "claim A"}, {Text: "claim B"}},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(req *data.JudgeRequest) (*data.JudgeVerdict, error) {
			switch req.Claim {
			case "claim A":
				return &data.JudgeVerdict{
					Verdict:       model.VerdictSupported,
					CitedEvidence: []int{0},
				}, nil
			case "claim B":
				return &data.JudgeVerdict{
					Verdict:       model.VerdictContradicted,
					CitedEvidence: []int{0},
				}, nil
			default:
				t.Fatalf("unexpected claim %q", req.Claim)
				return nil, nil
			}
		},
	}

	s := NewClaimService(analyzer, judge, rag)
	result, err := s.CheckClaim(t.Context(), &dto.CheckClaimDTO{Text: "claim A and claim B"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.InScope {
		t.Fatal("expected InScope true")
	}
	if len(result.SubClaims) != 2 {
		t.Fatalf("expected 2 sub-claim results, got %d", len(result.SubClaims))
	}
	// Order must match the analyzer's SubClaims order regardless of which
	// goroutine finished first.
	if result.SubClaims[0].Claim != "claim A" ||
		result.SubClaims[0].Verdict != model.VerdictSupported {
		t.Errorf("expected sub-claim 0 = claim A/Supported, got %+v", result.SubClaims[0])
	}
	if result.SubClaims[1].Claim != "claim B" ||
		result.SubClaims[1].Verdict != model.VerdictContradicted {
		t.Errorf("expected sub-claim 1 = claim B/Contradicted, got %+v", result.SubClaims[1])
	}
	if result.OverallSummary != "contient des inexactitudes" {
		t.Errorf(
			"expected overall summary to reflect the contradiction, got %q",
			result.OverallSummary,
		)
	}

	wantMessage := "❌ contient des inexactitudes — 2 affirmations vérifiées\n\n" +
		"✅ 1. claim A\n\n" +
		"❌ 2. claim B"
	if result.FormattedMessage != wantMessage {
		t.Errorf(
			"expected formatted message %q, got %q",
			wantMessage, result.FormattedMessage,
		)
	}
}

func TestClaimService_CheckClaim_IsLongControlsFormattedMessageVerbosity(t *testing.T) {
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
					ParsedChunk: model.ParsedChunk{Text: "supporting text"},
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

	rag := NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1}})
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{InScope: true, SubClaims: []data.SubClaim{{Text: "claim A"}}},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			return &data.JudgeVerdict{
				Verdict:        model.VerdictSupported,
				Reasoning:      "the full multi-sentence reasoning",
				BriefReasoning: "a short paraphrase",
				CitedEvidence:  []int{0},
			}, nil
		},
	}
	s := NewClaimService(analyzer, judge, rag)

	short, err := s.CheckClaim(t.Context(), &dto.CheckClaimDTO{Text: "claim A"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(short.FormattedMessage, "a short paraphrase") {
		t.Errorf(
			"expected short FormattedMessage to contain the brief reasoning, got %q",
			short.FormattedMessage,
		)
	}
	if strings.Contains(short.FormattedMessage, "the full multi-sentence reasoning") {
		t.Errorf(
			"expected short FormattedMessage to omit the full reasoning, got %q",
			short.FormattedMessage,
		)
	}
	if strings.Contains(short.FormattedMessage, "supporting text") {
		t.Errorf(
			"expected short FormattedMessage to omit citations, got %q",
			short.FormattedMessage,
		)
	}

	long, err := s.CheckClaim(t.Context(), &dto.CheckClaimDTO{Text: "claim A", IsLong: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(long.FormattedMessage, "the full multi-sentence reasoning") {
		t.Errorf(
			"expected long FormattedMessage to contain the full reasoning, got %q",
			long.FormattedMessage,
		)
	}
	if strings.Contains(long.FormattedMessage, "a short paraphrase") {
		t.Errorf(
			"expected long FormattedMessage to omit the brief reasoning, got %q",
			long.FormattedMessage,
		)
	}
	if !strings.Contains(long.FormattedMessage, "supporting text") {
		t.Errorf(
			"expected long FormattedMessage to include the cited evidence, got %q",
			long.FormattedMessage,
		)
	}

	// Structured data (SubClaims/Citations) is always fully populated
	// regardless of IsLong — including in the short result, which only
	// omits citations from FormattedMessage's text, not from the data.
	for _, r := range []*ClaimCheckResult{short, long} {
		if len(r.SubClaims) != 1 || len(r.SubClaims[0].Citations) != 1 {
			t.Fatalf("expected 1 sub-claim with 1 citation, got %+v", r.SubClaims)
		}
		if r.SubClaims[0].Citations[0].ChunkID != evidenceChunkID {
			t.Errorf(
				"expected citation ChunkID %s, got %s",
				evidenceChunkID, r.SubClaims[0].Citations[0].ChunkID,
			)
		}
	}
}

func TestClaimService_CheckClaim_OutOfScopeHasNoFormattedMessage(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{InScope: false, RefusalReason: "not health-related"},
	}
	s := NewClaimService(analyzer, &stubClaimJudge{}, newEmptyRAGService())

	result, err := s.CheckClaim(
		t.Context(),
		&dto.CheckClaimDTO{Text: "what's the capital of France?"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FormattedMessage != "" {
		t.Errorf(
			"expected no formatted message for out-of-scope input, got %q",
			result.FormattedMessage,
		)
	}
}

// assertHighRiskOverrideApplied is shared by the high-risk override tests
// below (response-rule.txt section 4's "whatever else is true" guarantee).
func assertHighRiskOverrideApplied(t *testing.T, sc SubClaimResult) {
	t.Helper()
	if sc.Verdict != model.VerdictContradicted {
		t.Errorf("expected Verdict forced to Contradicted, got %s", sc.Verdict)
	}
	if !sc.HighRiskCaution {
		t.Error("expected HighRiskCaution true")
	}
	if !sc.RecommendMedicalConsult {
		t.Error("expected RecommendMedicalConsult true")
	}
	if sc.Severity < model.SeveritySerious {
		t.Errorf("expected Severity >= Serious, got %s", sc.Severity)
	}
}

// A1: the override beats a favorable judge verdict — the core "whatever
// else is true" guarantee.
func TestClaimService_CheckClaim_HighRiskOverrideBeatsFavorableJudgeVerdict(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: "l'alcool augmente la fertilité"}},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			return &data.JudgeVerdict{
				Verdict:        model.VerdictSupported,
				Reasoning:      "the judge's own full reasoning",
				BriefReasoning: "the judge's own brief reasoning",
				CitedEvidence:  []int{0},
			}, nil
		},
	}
	s := NewClaimService(analyzer, judge, ragServiceWithAnyHit())

	result, err := s.CheckClaim(
		t.Context(),
		&dto.CheckClaimDTO{Text: "l'alcool augmente la fertilité"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SubClaims) != 1 {
		t.Fatalf("expected 1 sub-claim, got %d", len(result.SubClaims))
	}
	assertHighRiskOverrideApplied(t, result.SubClaims[0])

	wantCaution := highRiskCautionTemplates[model.LanguageFrench]
	if !strings.Contains(result.FormattedMessage, wantCaution) {
		t.Errorf(
			"expected FormattedMessage to contain caution template, got %q",
			result.FormattedMessage,
		)
	}
	if strings.Contains(result.FormattedMessage, "the judge's own") {
		t.Errorf(
			"expected FormattedMessage to omit the judge's own reasoning, got %q",
			result.FormattedMessage,
		)
	}
}

// A2: the override applies on the zero-evidence path too, where the judge
// is never called.
func TestClaimService_CheckClaim_HighRiskOverrideAppliesOnZeroEvidencePath(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: "le tabac guérit l'acné"}},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			t.Fatal("judge should never be called when retrieval returns no evidence")
			return nil, nil
		},
	}
	s := NewClaimService(analyzer, judge, newEmptyRAGService())

	result, err := s.CheckClaim(t.Context(), &dto.CheckClaimDTO{Text: "le tabac guérit l'acné"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SubClaims) != 1 {
		t.Fatalf("expected 1 sub-claim, got %d", len(result.SubClaims))
	}
	assertHighRiskOverrideApplied(t, result.SubClaims[0])
}

// A3: the override applies cleanly even when the judge independently also
// says Contradicted — no double-application weirdness.
func TestClaimService_CheckClaim_HighRiskOverrideOnAlreadyContradictedVerdict(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: "drinking alcohol increases fertility"}},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			return &data.JudgeVerdict{
				Verdict:       model.VerdictContradicted,
				Severity:      model.SeverityEmergency,
				Reasoning:     "the judge's own full reasoning",
				CitedEvidence: []int{0},
			}, nil
		},
	}
	s := NewClaimService(analyzer, judge, ragServiceWithAnyHit())

	result, err := s.CheckClaim(
		t.Context(),
		&dto.CheckClaimDTO{Text: "drinking alcohol increases fertility"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sc := result.SubClaims[0]
	assertHighRiskOverrideApplied(t, sc)
	// The override must not downgrade a judge-assigned Severity that's
	// already at/above Serious.
	if sc.Severity != model.SeverityEmergency {
		t.Errorf("expected Severity to stay Emergency, got %s", sc.Severity)
	}
	if strings.Count(
		result.FormattedMessage,
		highRiskCautionTemplates[model.LanguageFrench],
	) != 1 {
		t.Errorf(
			"expected caution template to appear exactly once, got %q",
			result.FormattedMessage,
		)
	}
}

// A4: the override fires from the analyzer's LLM-judged flag alone (an
// unregulated-mixture-style claim, section 4 bullet 2), independent of the
// keyword matcher — this exact text has no substance keyword in it.
func TestClaimService_CheckClaim_HighRiskOverrideFromAnalyzerFlag_Mixture(t *testing.T) {
	claim := "des bonbons fondus dans du lait chaud avec du miel guérissent les infections"
	if isHighRiskSubstanceMention(claim) {
		t.Fatalf("test setup invalid: claim unexpectedly matches the keyword list: %q", claim)
	}

	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: claim, HighRisk: true}},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			return &data.JudgeVerdict{Verdict: model.VerdictSupported, CitedEvidence: []int{0}}, nil
		},
	}
	s := NewClaimService(analyzer, judge, ragServiceWithAnyHit())

	result, err := s.CheckClaim(t.Context(), &dto.CheckClaimDTO{Text: claim})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertHighRiskOverrideApplied(t, result.SubClaims[0])
}

// A5: same as A4 but for a self-medication-style claim (section 4 bullet 4).
func TestClaimService_CheckClaim_HighRiskOverrideFromAnalyzerFlag_SelfMedication(t *testing.T) {
	claim := "prendre des antibiotiques sans ordonnance pour une infection vaginale"
	if isHighRiskSubstanceMention(claim) {
		t.Fatalf("test setup invalid: claim unexpectedly matches the keyword list: %q", claim)
	}

	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: claim, HighRisk: true}},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			return &data.JudgeVerdict{Verdict: model.VerdictSupported, CitedEvidence: []int{0}}, nil
		},
	}
	s := NewClaimService(analyzer, judge, ragServiceWithAnyHit())

	result, err := s.CheckClaim(t.Context(), &dto.CheckClaimDTO{Text: claim})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertHighRiskOverrideApplied(t, result.SubClaims[0])
}

// A6: bilingual keyword coverage end-to-end through CheckClaim, not just at
// the isHighRiskSubstanceMention unit level.
func TestClaimService_CheckClaim_HighRiskKeywordOverrideBilingual(t *testing.T) {
	for _, tt := range []struct {
		name  string
		claim string
	}{
		{"french", "l'alcool augmente la fertilité"},
		{"english", "alcohol increases fertility"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := &stubClaimAnalyzer{
				analysis: &data.ClaimAnalysis{
					InScope:   true,
					SubClaims: []data.SubClaim{{Text: tt.claim}},
				},
			}
			judge := &stubClaimJudge{
				judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
					return &data.JudgeVerdict{
						Verdict:       model.VerdictSupported,
						CitedEvidence: []int{0},
					}, nil
				},
			}
			s := NewClaimService(analyzer, judge, ragServiceWithAnyHit())

			result, err := s.CheckClaim(t.Context(), &dto.CheckClaimDTO{Text: tt.claim})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertHighRiskOverrideApplied(t, result.SubClaims[0])
		})
	}
}

// A7: mixed sub-claims — only the flagged one is overridden, the rollup
// still reports the high-risk category as top priority.
func TestClaimService_CheckClaim_HighRiskOverridePartialAcrossSubClaims(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope: true,
			SubClaims: []data.SubClaim{
				{Text: "claim A"},
				{Text: "l'alcool augmente la fertilité"},
			},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(req *data.JudgeRequest) (*data.JudgeVerdict, error) {
			return &data.JudgeVerdict{Verdict: model.VerdictSupported, CitedEvidence: []int{0}}, nil
		},
	}
	s := NewClaimService(analyzer, judge, ragServiceWithAnyHit())

	result, err := s.CheckClaim(
		t.Context(),
		&dto.CheckClaimDTO{Text: "claim A and l'alcool augmente la fertilité"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SubClaims) != 2 {
		t.Fatalf("expected 2 sub-claims, got %d", len(result.SubClaims))
	}
	if result.SubClaims[0].Verdict != model.VerdictSupported ||
		result.SubClaims[0].HighRiskCaution {
		t.Errorf(
			"expected sub-claim 0 to keep its own Supported verdict, got %+v",
			result.SubClaims[0],
		)
	}
	assertHighRiskOverrideApplied(t, result.SubClaims[1])
	if overallVerdictKey(result.SubClaims) != overallKeyHighRisk {
		t.Errorf(
			"expected overall rollup to report high-risk as top priority, got %q",
			overallVerdictKey(result.SubClaims),
		)
	}
}

// A8: regression guard for the resolved response-rule.txt contradiction —
// an unvalidated food/herbal remedy claim (section 4 bullet 3) must NOT be
// forced RED; it resolves to ordinary AMBER via the existing waterfall,
// matching worked example 1 (carrot juice / lubrication).
func TestClaimService_CheckClaim_UnvalidatedFoodRemedyClaimIsNotForcedRed(t *testing.T) {
	claim := "Le jus de carotte améliore la lubrification vaginale"
	if isHighRiskSubstanceMention(claim) {
		t.Fatalf("test setup invalid: claim unexpectedly matches the keyword list: %q", claim)
	}

	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []data.SubClaim{{Text: claim, HighRisk: false}},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			t.Fatal("judge should never be called when retrieval returns no evidence")
			return nil, nil
		},
	}
	s := NewClaimService(analyzer, judge, newEmptyRAGService())

	result, err := s.CheckClaim(t.Context(), &dto.CheckClaimDTO{Text: claim})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sc := result.SubClaims[0]
	if sc.Verdict != model.VerdictInsufficientEvidence {
		t.Errorf("expected InsufficientEvidence (AMBER), got %s", sc.Verdict)
	}
	if sc.HighRiskCaution {
		t.Error("expected HighRiskCaution false for an unvalidated food/herbal remedy claim")
	}
}
