package data

import (
	"context"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/model"
)

// JudgeEvidence is a single piece of retrieved knowledge-base evidence handed
// to the judge for one sub-claim — either a document chunk hit, or an
// atomic-knowledge fact hit hydrated with its parent chunk's text for fuller
// context (see service.RAGResult.ChunkText).
type JudgeEvidence struct {
	// Source is "chunk" or "fact", mirroring service.RAGResult.Source.
	Source string
	// Text is the chunk's text, or the fact's SPOText() — the precise match.
	Text string
	// ChunkText is the source chunk's full text, for a fact hit — empty for
	// a chunk hit, which already IS the chunk.
	ChunkText string
	// TableMarkdown is carried through for citation display; the judge uses
	// Text and ChunkText, which retain the search-oriented representation.
	TableMarkdown string
	// TruthTier is set for fact hits only (empty for chunk hits), mirroring
	// service.RAGResult.TruthTier — carried as the display string directly,
	// not re-parsed into model.TruthTier, since chunk hits have no truth
	// tier at all (not merely an "unknown" one).
	TruthTier  string
	DocumentID uuid.UUID
	// ChunkID is the chunk this evidence traces back to, mirroring
	// service.RAGResult.ChunkID — pass-through data only, like DocumentID;
	// neither is shown to the judge in buildJudgePrompt, both just ride
	// along so citations built from CitedEvidence can reference them.
	ChunkID uuid.UUID
	// Language is the source chunk's/fact's own language, mirroring
	// service.RAGResult.Language.
	Language model.Language
}

// JudgeRequest bundles one sub-claim with the evidence retrieved for it. The
// judge must decide using only this evidence — never the model's own
// outside/training knowledge.
type JudgeRequest struct {
	Claim    string
	Evidence []JudgeEvidence
	// ResponseLanguage is the language the judge's Reasoning must be written
	// in — the original query's detected language (data.ClaimAnalysis.Language),
	// not any individual evidence item's language. Evidence is never
	// translated; it's shown to the judge exactly as stored, in whatever
	// language it happens to be in.
	ResponseLanguage model.Language
}

// JudgeVerdict is the judge's decision for one sub-claim.
type JudgeVerdict struct {
	Verdict  model.Verdict
	Severity model.Severity
	// RecommendMedicalConsult is true whenever Severity warrants recommending
	// a real doctor (Serious/Emergency) — never used to influence Verdict itself.
	RecommendMedicalConsult bool
	Reasoning               string
	// BriefReasoning is a short, self-contained one-sentence paraphrase of
	// Reasoning — unlike Reasoning, it must never use inline "[N]" citation
	// markers, since it's meant to be shown on its own, without the evidence
	// list alongside it (see service.formatClaimMessage's short mode).
	BriefReasoning string
	// CitedEvidence indexes into JudgeRequest.Evidence. Required for any
	// Verdict other than InsufficientEvidence; a caller receiving an empty
	// CitedEvidence alongside any other Verdict must force it to
	// InsufficientEvidence rather than trust the model's own confidence.
	CitedEvidence []int
}

// ClaimJudge decides how a claim relates to the evidence retrieved for it —
// strictly from that evidence, never from the model's own outside/training
// knowledge. Provider-agnostic, same pattern as KnowledgeExtractor; see
// external.JudgeClient for the concrete implementation.
type ClaimJudge interface {
	// Judge returns the verdict for req.Claim given req.Evidence.
	Judge(ctx context.Context, req *JudgeRequest) (*JudgeVerdict, error)
}
