package external_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/model"
)

func fakeJudgeResponseBody(t *testing.T, verdict map[string]any) []byte {
	t.Helper()
	content, err := json.Marshal(verdict)
	if err != nil {
		t.Fatalf("failed to marshal fake verdict: %v", err)
	}
	return chatResponseBody(t, string(content))
}

func TestJudgeClient_Judge_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJudgeResponseBody(t, map[string]any{
			"verdict":                   "supported",
			"severity":                  "routine",
			"recommend_medical_consult": false,
			"reasoning":                 "the cited evidence backs the claim",
			"brief_reasoning":           "vitamin C helps a bit",
			"cited_evidence":            []int{0},
		}))
	}))
	defer server.Close()

	client := external.NewJudgeClient(server.URL, "test-model", "test-api-key")
	verdict, err := client.Judge(t.Context(), &data.JudgeRequest{
		Claim: "vitamin C prevents colds",
		Evidence: []data.JudgeEvidence{
			{Source: "chunk", Text: "some supporting evidence"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.Verdict != model.VerdictSupported {
		t.Errorf("expected VerdictSupported, got %v", verdict.Verdict)
	}
	if verdict.Severity != model.SeverityRoutine {
		t.Errorf("expected SeverityRoutine, got %v", verdict.Severity)
	}
	if len(verdict.CitedEvidence) != 1 || verdict.CitedEvidence[0] != 0 {
		t.Errorf("expected CitedEvidence [0], got %v", verdict.CitedEvidence)
	}
	if verdict.BriefReasoning != "vitamin C helps a bit" {
		t.Errorf(
			"expected BriefReasoning %q, got %q",
			"vitamin C helps a bit",
			verdict.BriefReasoning,
		)
	}
}

func TestJudgeClient_Judge_ForcesInsufficientEvidenceWhenNoCitations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJudgeResponseBody(t, map[string]any{
			"verdict":        "supported",
			"severity":       "routine",
			"reasoning":      "I'm confident even though I can't point to specific evidence",
			"cited_evidence": []int{},
		}))
	}))
	defer server.Close()

	client := external.NewJudgeClient(server.URL, "test-model", "test-api-key")
	verdict, err := client.Judge(t.Context(), &data.JudgeRequest{
		Claim:    "some claim",
		Evidence: []data.JudgeEvidence{{Source: "chunk", Text: "unrelated text"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.Verdict != model.VerdictInsufficientEvidence {
		t.Errorf(
			"expected the citation-less verdict to be forced to InsufficientEvidence, got %v",
			verdict.Verdict,
		)
	}
}

func TestJudgeClient_Judge_DropsOutOfRangeCitationsAndForcesInsufficientEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJudgeResponseBody(t, map[string]any{
			"verdict":        "contradicted",
			"severity":       "routine",
			"cited_evidence": []int{5}, // only 1 evidence item exists, index 5 is hallucinated
		}))
	}))
	defer server.Close()

	client := external.NewJudgeClient(server.URL, "test-model", "test-api-key")
	verdict, err := client.Judge(t.Context(), &data.JudgeRequest{
		Claim:    "some claim",
		Evidence: []data.JudgeEvidence{{Source: "chunk", Text: "the only evidence"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verdict.CitedEvidence) != 0 {
		t.Errorf("expected the out-of-range citation to be dropped, got %v", verdict.CitedEvidence)
	}
	if verdict.Verdict != model.VerdictInsufficientEvidence {
		t.Errorf(
			"expected verdict forced to InsufficientEvidence once citations were dropped, got %v",
			verdict.Verdict,
		)
	}
}

func TestJudgeClient_Judge_UnrecognizedSeverityDefaultsToSeriousAndForcesConsult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJudgeResponseBody(t, map[string]any{
			"verdict":                   "supported",
			"severity":                  "not-a-real-severity",
			"recommend_medical_consult": false,
			"cited_evidence":            []int{0},
		}))
	}))
	defer server.Close()

	client := external.NewJudgeClient(server.URL, "test-model", "test-api-key")
	verdict, err := client.Judge(t.Context(), &data.JudgeRequest{
		Claim:    "some claim",
		Evidence: []data.JudgeEvidence{{Source: "chunk", Text: "some evidence"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.Severity != model.SeveritySerious {
		t.Errorf("expected the safe default SeveritySerious, got %v", verdict.Severity)
	}
	if !verdict.RecommendMedicalConsult {
		t.Error("expected recommend_medical_consult forced to true when severity can't be parsed")
	}
}
