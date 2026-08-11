package external_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kairosedubf/wobsongo/external"
)

func fakeAnalysisResponseBody(t *testing.T, analysis map[string]any) []byte {
	t.Helper()
	content, err := json.Marshal(analysis)
	if err != nil {
		t.Fatalf("failed to marshal fake analysis: %v", err)
	}
	return chatResponseBody(t, string(content))
}

func TestClaimAnalyzerClient_Analyze_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeAnalysisResponseBody(t, map[string]any{
			"in_scope": true,
			"sub_claims": []map[string]any{
				{"text": "vitamin C prevents colds", "high_risk": false},
			},
		}))
	}))
	defer server.Close()

	client := external.NewClaimAnalyzerClient(server.URL, "test-model", "test-api-key")
	analysis, err := client.Analyze(t.Context(), "does vitamin C prevent colds?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !analysis.InScope {
		t.Error("expected InScope true")
	}
	if len(analysis.SubClaims) != 1 || analysis.SubClaims[0].Text != "vitamin C prevents colds" {
		t.Errorf("expected 1 sub-claim, got %v", analysis.SubClaims)
	}
	if analysis.SubClaims[0].HighRisk {
		t.Error("expected HighRisk false")
	}
}

func TestClaimAnalyzerClient_Analyze_ParsesHighRiskFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeAnalysisResponseBody(t, map[string]any{
			"in_scope": true,
			"sub_claims": []map[string]any{
				{"text": "boire de l'alcool augmente la fertilité", "high_risk": true},
				{"text": "le folate prévient les anomalies du tube neural", "high_risk": false},
			},
		}))
	}))
	defer server.Close()

	client := external.NewClaimAnalyzerClient(server.URL, "test-model", "test-api-key")
	analysis, err := client.Analyze(
		t.Context(),
		"l'alcool augmente la fertilité et le folate prévient les anomalies",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(analysis.SubClaims) != 2 {
		t.Fatalf("expected 2 sub-claims, got %d", len(analysis.SubClaims))
	}
	if !analysis.SubClaims[0].HighRisk {
		t.Error("expected first sub-claim HighRisk true")
	}
	if analysis.SubClaims[1].HighRisk {
		t.Error("expected second sub-claim HighRisk false")
	}
}

func TestClaimAnalyzerClient_Analyze_MissingHighRiskDefaultsFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeAnalysisResponseBody(t, map[string]any{
			"in_scope": true,
			"sub_claims": []map[string]any{
				{"text": "vitamin C prevents colds"},
			},
		}))
	}))
	defer server.Close()

	client := external.NewClaimAnalyzerClient(server.URL, "test-model", "test-api-key")
	analysis, err := client.Analyze(t.Context(), "does vitamin C prevent colds?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(analysis.SubClaims) != 1 {
		t.Fatalf("expected 1 sub-claim, got %d", len(analysis.SubClaims))
	}
	if analysis.SubClaims[0].HighRisk {
		t.Error(
			"expected HighRisk to default false when the field is missing from the wire response",
		)
	}
}

func TestClaimAnalyzerClient_Analyze_OutOfScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeAnalysisResponseBody(t, map[string]any{
			"in_scope":       false,
			"refusal_reason": "not health-related",
			"sub_claims":     []map[string]any{},
		}))
	}))
	defer server.Close()

	client := external.NewClaimAnalyzerClient(server.URL, "test-model", "test-api-key")
	analysis, err := client.Analyze(t.Context(), "what's the capital of France?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if analysis.InScope {
		t.Error("expected InScope false")
	}
	if analysis.RefusalReason != "not health-related" {
		t.Errorf("expected refusal reason to propagate, got %q", analysis.RefusalReason)
	}
}

func TestClaimAnalyzerClient_Analyze_CapsSubClaimsAtMax(t *testing.T) {
	// Mirrors external.analyzerMaxSubClaims (5) — this test package is
	// black-box (external_test), so it can't reference the unexported
	// constant directly; the expected cap is hardcoded here instead.
	const maxSubClaims = 5

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeAnalysisResponseBody(t, map[string]any{
			"in_scope": true,
			"sub_claims": []map[string]any{
				{"text": "a"},
				{"text": "b"},
				{"text": "c"},
				{"text": "d"},
				{"text": "e"},
				{"text": "f"},
				{"text": "g"},
			},
		}))
	}))
	defer server.Close()

	client := external.NewClaimAnalyzerClient(server.URL, "test-model", "test-api-key")
	analysis, err := client.Analyze(t.Context(), "a compound claim with many parts")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(analysis.SubClaims) != maxSubClaims {
		t.Errorf(
			"expected sub-claims capped at %d, got %d: %v",
			maxSubClaims,
			len(analysis.SubClaims),
			analysis.SubClaims,
		)
	}
}
