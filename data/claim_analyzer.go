package data

import (
	"context"

	"github.com/kairosedubf/wobsongo/model"
)

// ClaimAnalysis is the result of scoping and decomposing a raw input message,
// before any retrieval happens.
type ClaimAnalysis struct {
	// InScope is false if the input isn't a health-related inquiry at all —
	// the system explicitly doesn't take inquiries unrelated to health.
	InScope bool

	// RefusalReason explains why InScope is false. Empty when InScope is true.
	RefusalReason string

	// SubClaims are the normalized, individually-checkable propositions
	// extracted from the raw input — a claim decomposed from potentially
	// messy source phrasing (e.g. a video transcript sentence) into one or
	// more crisp assertions a retrieval query can match well against. Empty
	// when InScope is false.
	SubClaims []SubClaim

	// Language is the detected language of the original input message —
	// threaded through to the judge (data.JudgeRequest.ResponseLanguage) and
	// to ClaimCheckResult.OverallSummary, so the whole response matches
	// whatever language the user actually asked in, regardless of what
	// language the retrieved evidence happens to be stored in.
	Language model.Language
}

// SubClaim is one normalized, individually-checkable proposition produced by
// decomposition, along with whether it falls under response-rule.txt
// section 4's high-risk safety override (unregulated mixtures/potions, or
// self-medication/anything that could delay proper care — tobacco/alcohol/
// illicit drugs are instead caught deterministically by a keyword fail-safe,
// see internal/service/high_risk_keywords.go, independent of this flag).
type SubClaim struct {
	Text     string
	HighRisk bool
}

// ClaimAnalyzer scopes and decomposes a raw input message before retrieval:
// rejects non-health-related input, and splits a compound or loosely-phrased
// claim into normalized sub-claims. Provider-agnostic, same pattern as
// KnowledgeExtractor; see external.ClaimAnalyzerClient for the concrete
// implementation.
type ClaimAnalyzer interface {
	// Analyze returns the scoping/decomposition result for message.
	Analyze(ctx context.Context, message string) (*ClaimAnalysis, error)
}
