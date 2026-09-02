package dto

import "github.com/google/uuid"

// CheckClaimDTO represents the payload for checking a claim against the knowledge base.
type CheckClaimDTO struct {
	// Text is the claim or message to check — may be a direct question, a
	// casual claim, or a sentence pulled from a video transcript.
	Text string `json:"text" validate:"required"`

	// IsLong controls FormattedMessage's verbosity: false (the default —
	// note the zero-value already means short, no need to set this at all
	// for the common case) shows a brief paraphrased explainer per
	// sub-claim; true shows the full reasoning plus its evidence/citations.
	// Only affects FormattedMessage — SubClaims/Citations are always fully
	// populated in the structured response either way.
	IsLong bool `json:"is_long,omitempty"`
}

// ClaimCheckResponse represents the outcome of checking a claim.
type ClaimCheckResponse struct {
	// InScope is false if the input wasn't a health-related inquiry at all —
	// this system doesn't take inquiries unrelated to health.
	InScope bool `json:"in_scope"`

	// RefusalReason explains why InScope is false. Empty when InScope is true.
	RefusalReason string `json:"refusal_reason,omitempty"`

	// OverallSummary is a short rollup across every sub-claim's verdict.
	// Empty when InScope is false.
	OverallSummary string `json:"overall_summary,omitempty"`

	// FormattedMessage is a human-facing, color-coded (emoji-per-verdict)
	// rendering of OverallSummary and every sub-claim's verdict/reasoning —
	// meant to be displayed as-is by a chat client. Plain text, no
	// HTML/Markdown markup. Empty when InScope is false.
	FormattedMessage string `json:"formatted_message,omitempty"`

	// SubClaims is the per-sub-claim breakdown. Empty when InScope is false.
	SubClaims []SubClaimResponse `json:"sub_claims,omitempty"`
}

// SubClaimResponse is one sub-claim's verdict and supporting citations.
type SubClaimResponse struct {
	// Claim is the normalized sub-claim text that was actually checked.
	Claim string `json:"claim"`

	// Verdict is one of: supported, contradicted, partially_supported, mixed, insufficient_evidence.
	// insufficient_evidence never implies the claim itself is right or wrong
	// — only that the knowledge base had nothing relevant to check it against.
	Verdict string `json:"verdict"`

	// Severity is one of: routine, serious, emergency.
	Severity string `json:"severity"`

	// RecommendMedicalConsult is true whenever the claim's subject matter
	// warrants recommending a real doctor — this system provides quick
	// access to reliable information, it does not replace one.
	RecommendMedicalConsult bool `json:"recommend_medical_consult"`

	// Reasoning is the judge's explanation for the verdict.
	Reasoning string `json:"reasoning"`

	// BriefReasoning is a short, self-contained paraphrase of Reasoning,
	// safe to display without the citations list alongside it.
	BriefReasoning string `json:"brief_reasoning"`

	// Citations are the specific pieces of knowledge-base evidence the
	// verdict is based on. Empty whenever Verdict is insufficient_evidence.
	Citations []CitationResponse `json:"citations"`
}

// CitationResponse is a single piece of knowledge-base evidence cited by a verdict.
type CitationResponse struct {
	// Index is the same 0-based number the judge's Reasoning text references
	// inline as "[N]" — use it to link a prose citation back to this entry.
	Index int `json:"index"`

	// Source is "chunk" or "fact".
	Source string `json:"source"`

	// DocumentID is the source document this evidence came from.
	DocumentID uuid.UUID `json:"document_id"`

	// ChunkID is the chunk this evidence traces back to — combine with
	// DocumentID to link directly to this evidence (e.g. the standalone
	// /evidence PDF viewer: /evidence?document_id=...&chunk_id=...).
	ChunkID uuid.UUID `json:"chunk_id"`

	// Text is the matched text — a chunk's text, or a fact's subject-predicate-object.
	Text string `json:"text"`

	// TableMarkdown is present for table-backed citations and is intended for
	// display; Text remains the matched text used by retrieval/judging.
	TableMarkdown string `json:"table_markdown,omitempty"`
}
