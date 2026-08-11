package service

import (
	"regexp"
	"strings"
)

// highRiskSubstanceKeywords names tobacco, alcohol, and recreational/illicit
// drugs (response-rule.txt section 4, bullet 1) — the one high-risk category
// concrete enough for a fixed bilingual (EN/FR) keyword list. Seeded here for
// now; swap the backing list for a DB-backed lookup once Stephanie's
// risk-classified document has an ingestion pipeline — callers only depend
// on isHighRiskSubstanceMention's signature, not this list directly.
var highRiskSubstanceKeywords = []string{
	// English — tobacco
	"tobacco", "cigarette", "cigarettes", "cigar", "cigars", "nicotine", "vape", "vaping",
	// English — alcohol
	"alcohol", "beer", "wine", "liquor", "whisky", "whiskey", "vodka",
	// English — recreational/illicit drugs
	"drug", "drugs", "cannabis", "marijuana", "weed", "cocaine", "heroin", "ecstasy", "meth",
	// French — tabac
	"tabac", "cigarette", "cigarettes", "cigare", "cigares", "nicotine", "vapoter",
	// French — alcool
	"alcool", "bière", "vin", "whisky", "vodka",
	// French — drogues récréatives/illicites
	"drogue", "drogues", "cannabis", "marijuana", "cocaïne", "héroïne",
}

// isHighRiskSubstanceMentionPattern is compiled once from
// highRiskSubstanceKeywords with \b word boundaries so short/common terms
// (e.g. French "vin") don't false-positive as substrings of unrelated words
// (e.g. "vinaigre", "province").
var isHighRiskSubstanceMentionPattern = compileHighRiskPattern(highRiskSubstanceKeywords)

func compileHighRiskPattern(keywords []string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString(`(?i)\b(`)
	for i, kw := range keywords {
		if i > 0 {
			b.WriteString("|")
		}
		b.WriteString(regexp.QuoteMeta(kw))
	}
	b.WriteString(`)\b`)
	return regexp.MustCompile(b.String())
}

// isHighRiskSubstanceMention reports whether text names tobacco, alcohol, or
// a recreational/illicit drug — the deterministic fail-safe half of the
// high-risk override (internal/service/claim_service.go), guaranteed to fire
// regardless of LLM behavior. Semantic categories (unregulated mixtures,
// self-medication) aren't enumerable this way — those rely on the claim
// analyzer's own judgment instead (see data.SubClaim.HighRisk).
func isHighRiskSubstanceMention(text string) bool {
	return isHighRiskSubstanceMentionPattern.MatchString(text)
}
