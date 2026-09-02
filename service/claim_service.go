package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"golang.org/x/sync/errgroup"
)

// claimSearchLimit bounds how many fused hybrid-search results feed the
// judge per sub-claim — mirrors cmd/rag.go's ragDefaultLimit.
const claimSearchLimit = 10

// Citation is one piece of evidence a judge verdict cited. Index is the same
// 0-based position the evidence was shown at in the judge's prompt (see
// buildJudgePrompt in external/judge_client.go) — the judge is instructed to
// reference that same index inline in its Reasoning text as "[N]", so a
// caller can link a prose citation directly back to this entry.
type Citation struct {
	Index      int
	Source     string
	Text       string
	ChunkText     string
	TableMarkdown string
	TruthTier  string
	DocumentID uuid.UUID
	// ChunkID is the chunk this citation traces back to, mirroring
	// RAGResult.ChunkID — lets a caller link directly to this evidence (e.g.
	// the standalone /evidence PDF-bbox viewer).
	ChunkID uuid.UUID
	// Language is the source chunk's/fact's own language, mirroring
	// RAGResult.Language.
	Language model.Language
}

// SubClaimResult is one sub-claim's retrieval + judgment outcome.
type SubClaimResult struct {
	Claim                   string
	Verdict                 model.Verdict
	Severity                model.Severity
	RecommendMedicalConsult bool
	Reasoning               string
	// BriefReasoning is a short, self-contained paraphrase of Reasoning,
	// safe to show without the evidence list — see formatClaimMessage's
	// short mode. Empty whenever Verdict is InsufficientEvidence (the
	// zero-hits short-circuit in checkSubClaim never calls the judge).
	BriefReasoning string
	// Citations is the evidence the judge actually cited — empty whenever
	// Verdict is InsufficientEvidence.
	Citations []Citation
}

// ClaimCheckResult is the overall outcome of checking one input message,
// which may decompose into multiple sub-claims.
type ClaimCheckResult struct {
	InScope        bool
	RefusalReason  string
	OverallSummary string
	// FormattedMessage is a human-facing rendering of OverallSummary and
	// every sub-claim's verdict/reasoning, color-coded with an emoji per
	// verdict — meant to be displayed as-is by a chat client (e.g. the
	// WhatsApp bot) without it having to reimplement verdict-to-emoji
	// logic itself. Plain text, no HTML/Markdown markup, since this API is
	// transport-agnostic. Empty when InScope is false.
	FormattedMessage string
	SubClaims        []SubClaimResult
	// Language is the original input message's detected language
	// (data.ClaimAnalysis.Language) — OverallSummary and every sub-claim's
	// Reasoning are written in this language, regardless of what language
	// the cited evidence happens to be stored in.
	Language model.Language
}

// ClaimService orchestrates claim-checking as a fixed, bounded pipeline —
// not an open-ended agentic loop: scope+decompose the raw input once, then
// for every resulting sub-claim, concurrently retrieve evidence and judge
// it, and aggregate the results.
type ClaimService struct {
	analyzer data.ClaimAnalyzer
	judge    data.ClaimJudge
	rag      *RAGService
}

// NewClaimService is a constructor for ClaimService.
func NewClaimService(
	analyzer data.ClaimAnalyzer,
	judge data.ClaimJudge,
	rag *RAGService,
) *ClaimService {
	return &ClaimService{analyzer: analyzer, judge: judge, rag: rag}
}

// CheckClaim scopes/decomposes req.Text and, if in scope, judges every
// resulting sub-claim concurrently against knowledge-base evidence.
func (s *ClaimService) CheckClaim(
	ctx context.Context,
	req *dto.CheckClaimDTO,
) (*ClaimCheckResult, error) {
	message := req.Text
	analysis, err := s.analyzer.Analyze(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze claim: %w", err)
	}
	if !analysis.InScope {
		return &ClaimCheckResult{
			InScope:       false,
			RefusalReason: analysis.RefusalReason,
			Language:      analysis.Language,
		}, nil
	}

	subClaims := analysis.SubClaims
	if len(subClaims) == 0 {
		// The analyzer said in-scope but produced no decomposition — check
		// the original message directly rather than returning nothing.
		subClaims = []string{message}
	}

	results := make([]SubClaimResult, len(subClaims))
	// errgroup.WithContext, not a plain group: this is a synchronous,
	// user-facing request — a partial result (some sub-claims judged, one
	// errored) is worse than failing the whole request cleanly so the
	// caller can retry, unlike the background document-ingestion workers
	// where one chunk's failure shouldn't cancel its siblings.
	g, gctx := errgroup.WithContext(ctx)
	for i, claim := range subClaims {
		g.Go(func() error {
			result, err := s.checkSubClaim(gctx, claim, analysis.Language)
			if err != nil {
				return err
			}
			results[i] = *result
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("failed to check sub-claims: %w", err)
	}

	overallSummary := summarizeVerdicts(results, analysis.Language)
	return &ClaimCheckResult{
		InScope:        true,
		OverallSummary: overallSummary,
		FormattedMessage: formatClaimMessage(
			results,
			overallSummary,
			analysis.Language,
			req.IsLong,
		),
		SubClaims: results,
		Language:  analysis.Language,
	}, nil
}

// checkSubClaim retrieves evidence for one sub-claim and judges it. A
// sub-claim with no retrieved evidence never reaches the judge — there's
// nothing to cite, so InsufficientEvidence is returned directly rather than
// spending an LLM call to reach the same conclusion.
func (s *ClaimService) checkSubClaim(
	ctx context.Context,
	claim string,
	language model.Language,
) (*SubClaimResult, error) {
	hits, err := s.rag.Search(ctx, claim, claimSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to search evidence for sub-claim %q: %w", claim, err)
	}

	if len(hits) == 0 {
		return &SubClaimResult{
			Claim:   claim,
			Verdict: model.VerdictInsufficientEvidence,
		}, nil
	}

	evidence := make([]data.JudgeEvidence, len(hits))
	for i := range hits {
		h := &hits[i]
		evidence[i] = data.JudgeEvidence{
			Source:     h.Source,
			Text:       h.Text,
			ChunkText:  h.ChunkText,
			TableMarkdown: h.TableMarkdown,
			TruthTier:  h.TruthTier,
			DocumentID: h.DocumentID,
			ChunkID:    h.ChunkID,
			Language:   h.Language,
		}
	}

	verdict, err := s.judge.Judge(ctx, &data.JudgeRequest{
		Claim:            claim,
		Evidence:         evidence,
		ResponseLanguage: language,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to judge sub-claim %q: %w", claim, err)
	}

	citations := make([]Citation, 0, len(verdict.CitedEvidence))
	for _, idx := range verdict.CitedEvidence {
		if idx >= 0 && idx < len(evidence) {
			e := evidence[idx]
			citations = append(citations, Citation{
				Index:      idx,
				Source:     e.Source,
				Text:       e.Text,
				ChunkText:  e.ChunkText,
				TableMarkdown: e.TableMarkdown,
				TruthTier:  e.TruthTier,
				DocumentID: e.DocumentID,
				ChunkID:    e.ChunkID,
				Language:   e.Language,
			})
		}
	}

	return &SubClaimResult{
		Claim:                   claim,
		Verdict:                 verdict.Verdict,
		Severity:                verdict.Severity,
		RecommendMedicalConsult: verdict.RecommendMedicalConsult,
		Reasoning:               verdict.Reasoning,
		BriefReasoning:          verdict.BriefReasoning,
		Citations:               citations,
	}, nil
}

// Overall rollup categories produced by overallVerdictKey — shared between
// overallSummaryTemplates and overallEmoji so both stay keyed consistently.
const (
	overallKeyContradicted = "contradicted"
	overallKeySupported    = "supported"
	overallKeyInsufficient = "insufficient"
	overallKeyPartial      = "partial"
)

// overallSummaryTemplates gives each of summarizeVerdicts' four outcomes in
// both supported languages — mirrors languageDisplayNames' pattern
// (external/translation_client.go) of a language-keyed lookup rather than an
// LLM call, since this is a deterministic rollup of already-computed
// verdicts, not something that needs its own model call.
var overallSummaryTemplates = map[string]map[model.Language]string{
	overallKeyContradicted: {
		model.LanguageEnglish: "contains inaccuracies",
		model.LanguageFrench:  "contient des inexactitudes",
	},
	overallKeySupported: {
		model.LanguageEnglish: "supported",
		model.LanguageFrench:  "confirmé",
	},
	overallKeyInsufficient: {
		model.LanguageEnglish: "partially verified — some aspects could not be checked against the knowledge base",
		model.LanguageFrench:  "partiellement vérifié — certains aspects n'ont pas pu être vérifiés dans la base de connaissances",
	},
	overallKeyPartial: {
		model.LanguageEnglish: "partially supported",
		model.LanguageFrench:  "partiellement confirmé",
	},
}

// overallVerdictKey classifies a set of sub-claim verdicts into one of
// summarizeVerdicts' four rollup categories — extracted so
// summarizeVerdicts and formatClaimMessage can share the same
// classification without deriving it twice.
func overallVerdictKey(results []SubClaimResult) string {
	anyContradicted := false
	anyInsufficient := false
	allSupported := true

	for _, r := range results {
		switch r.Verdict {
		case model.VerdictContradicted, model.VerdictMixed:
			anyContradicted = true
			allSupported = false
		case model.VerdictInsufficientEvidence:
			anyInsufficient = true
			allSupported = false
		case model.VerdictPartiallySupported:
			allSupported = false
		case model.VerdictSupported:
			// No-op: allSupported already starts true and this verdict
			// doesn't change any rollup flag.
		}
	}

	switch {
	case anyContradicted:
		return overallKeyContradicted
	case allSupported:
		return overallKeySupported
	case anyInsufficient:
		return overallKeyInsufficient
	default:
		return overallKeyPartial
	}
}

// summarizeVerdicts rolls up per-sub-claim verdicts into one overall
// description of the original message, in language (the original input
// message's detected language) rather than always in English.
func summarizeVerdicts(results []SubClaimResult, language model.Language) string {
	return overallSummaryTemplates[overallVerdictKey(results)][language]
}

// overallEmoji is the color-coding shown alongside the overall rollup in a
// FormattedMessage, keyed the same way as overallSummaryTemplates.
var overallEmoji = map[string]string{
	overallKeySupported:    "✅",
	overallKeyContradicted: "❌",
	overallKeyInsufficient: "❓",
	overallKeyPartial:      "⚠️",
}

// claimsCheckedLabelTemplates gives the pluralized "N claims checked" label
// in both supported languages, mirroring overallSummaryTemplates' pattern.
var claimsCheckedLabelTemplates = map[model.Language]struct{ one, many string }{
	model.LanguageEnglish: {one: "claim checked", many: "claims checked"},
	model.LanguageFrench:  {one: "affirmation vérifiée", many: "affirmations vérifiées"},
}

// formatClaimMessage renders results and overallSummary into a human-facing,
// color-coded (emoji-per-verdict) plain-text message meant to be displayed
// as-is by a chat client — mirrors wobsongo-verify's Telegram bot output,
// adapted to this API's 5-value verdict taxonomy and bilingual support.
// Plain text rather than HTML/Markdown, since this API is transport-agnostic
// unlike a bot that sends directly to one chat platform.
//
// isLong selects between two shapes: false (short, the default) shows each
// sub-claim's BriefReasoning only; true shows the full Reasoning plus, when
// present, the evidence/citations it's actually based on.
func formatClaimMessage(
	results []SubClaimResult,
	overallSummary string,
	language model.Language,
	isLong bool,
) string {
	label := claimsCheckedLabelTemplates[language].many
	if len(results) == 1 {
		label = claimsCheckedLabelTemplates[language].one
	}

	var b strings.Builder
	b.WriteString(overallEmoji[overallVerdictKey(results)])
	b.WriteString(" ")
	b.WriteString(overallSummary)
	b.WriteString(" — ")
	b.WriteString(strconv.Itoa(len(results)))
	b.WriteString(" ")
	b.WriteString(label)
	b.WriteString("\n\n")

	for i, r := range results {
		b.WriteString(r.Verdict.Emoji())
		b.WriteString(" ")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(r.Claim)

		explainer := r.BriefReasoning
		if isLong {
			explainer = r.Reasoning
		}
		if explainer != "" {
			b.WriteString("\n")
			b.WriteString(explainer)
		}

		if isLong && len(r.Citations) > 0 {
			b.WriteString("\n")
			for _, c := range r.Citations {
				fmt.Fprintf(&b, "  [%d] (%s) %s\n", c.Index, c.Source, truncateForMessage(c.Text))
			}
		}

		b.WriteString("\n\n")
	}

	return strings.TrimSpace(b.String())
}

// truncateForMessage caps a citation's text at 200 runes for display in
// FormattedMessage's long form — mirrors cmd/rag.go's truncateForDisplay
// convention (there capped at 300, for a wider terminal listing).
func truncateForMessage(text string) string {
	const maxLen = 200
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}
