package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
)

// analyzerMaxTokens bounds the response length for scope/decomposition — a
// short structured decision plus a handful of short sub-claim strings, not a
// long-form generation, so a small budget is enough.
const analyzerMaxTokens = 1000

// analyzerHTTPTimeout bounds a single analyzer call — much shorter than
// ExtractionClient's, since the input (one message) and expected output
// (a short JSON decision) are both small.
const analyzerHTTPTimeout = 2 * time.Minute

// analyzerMaxSubClaims caps how many sub-claims Analyze returns, regardless
// of how many the model proposes — bounds worst-case downstream retrieval/
// judge cost per request.
const analyzerMaxSubClaims = 5

// analyzerChatRoleUser is the chat-completion message role for the caller's input.
const analyzerChatRoleUser = "user"

// ClaimAnalyzerClient implements data.ClaimAnalyzer against a generic
// OpenAI-compatible text chat-completions API — same shape as ExtractionClient.
type ClaimAnalyzerClient struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

// Ensure ClaimAnalyzerClient implements data.ClaimAnalyzer.
var _ data.ClaimAnalyzer = (*ClaimAnalyzerClient)(nil)

// NewClaimAnalyzerClient creates a new ClaimAnalyzerClient targeting the
// given base URL/model. apiKey may be empty — self-hosted servers often need
// no auth.
func NewClaimAnalyzerClient(baseURL, model, apiKey string) *ClaimAnalyzerClient {
	return &ClaimAnalyzerClient{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: analyzerHTTPTimeout,
		},
	}
}

// claimAnalysisJSON is the wire shape the LLM is instructed to respond with.
type claimAnalysisJSON struct {
	InScope       bool           `json:"in_scope"`
	RefusalReason string         `json:"refusal_reason"`
	SubClaims     []subClaimJSON `json:"sub_claims"`
	Language      string         `json:"language"`
}

// subClaimJSON is one sub_claims entry's wire shape.
type subClaimJSON struct {
	Text     string `json:"text"`
	HighRisk bool   `json:"high_risk"`
}

// Analyze implements data.ClaimAnalyzer.
func (c *ClaimAnalyzerClient) Analyze(
	ctx context.Context,
	message string,
) (*data.ClaimAnalysis, error) {
	payload := extractionCompletionRequest{
		Model: c.model,
		Messages: []extractionChatMessage{
			{Role: analyzerChatRoleUser, Content: buildAnalyzerPrompt(message)},
		},
		MaxTokens: analyzerMaxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal analyzer request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create analyzer request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call analyzer endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read analyzer response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"analyzer endpoint returned error status: %d. Body: %s",
			resp.StatusCode,
			string(respBytes),
		)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal analyzer response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("analyzer response contained no choices: %s", respBytes)
	}

	content := stripJSONCodeFence(parsed.Choices[0].Message.Content)
	var raw claimAnalysisJSON
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal claim analysis JSON: %w: %s", err, content)
	}

	rawSubClaims := raw.SubClaims
	if len(rawSubClaims) > analyzerMaxSubClaims {
		rawSubClaims = rawSubClaims[:analyzerMaxSubClaims]
	}
	subClaims := make([]data.SubClaim, len(rawSubClaims))
	for i, sc := range rawSubClaims {
		subClaims[i] = data.SubClaim{Text: sc.Text, HighRisk: sc.HighRisk}
	}

	language, err := model.ParseLanguage(raw.Language)
	if err != nil {
		// Defaults to English (ParseLanguage's own zero-value fallback)
		// rather than failing the whole analysis over a language-detection
		// miss — the worst case is an English-language summary/reasoning
		// for a non-English query, not a broken response.
		log.Printf(
			"[ClaimAnalyzerClient] unrecognized language %q, defaulting to English",
			raw.Language,
		)
	}

	return &data.ClaimAnalysis{
		InScope:       raw.InScope,
		RefusalReason: raw.RefusalReason,
		SubClaims:     subClaims,
		Language:      language,
	}, nil
}

// buildAnalyzerPrompt builds the scope+decomposition instruction.
func buildAnalyzerPrompt(message string) string {
	var b strings.Builder
	b.WriteString("You are the scope gate for a Sexual and Reproductive Health (SRH) ")
	b.WriteString("fact-checking system. Given a raw input message (which may be a casual ")
	b.WriteString("sentence, a claim from a video transcript, or a direct question), decide:\n\n")
	b.WriteString("1. Is this a health-related inquiry? This system only checks health claims ")
	b.WriteString("— reject anything unrelated to health (e.g. general trivia, requests to do ")
	b.WriteString("unrelated tasks, small talk).\n")
	b.WriteString("2. If it is health-related, decompose it into one or more short, independently ")
	b.WriteString(
		"checkable factual claims, rephrased as crisp, self-contained propositions — not ",
	)
	b.WriteString(
		"the original casual/compound phrasing. A simple claim decomposes to one item; a ",
	)
	b.WriteString("compound claim (\"X treats Y and also prevents Z\") decomposes to multiple.\n")
	b.WriteString(
		"3. What language is the input message written in — exactly one of \"en\" or \"fr\".\n",
	)
	b.WriteString(
		"4. For each sub-claim, does it promote or ask about: tobacco, alcohol, or a ",
	)
	b.WriteString(
		"recreational or illicit drug; an unregulated homemade mixture or potion to ingest or ",
	)
	b.WriteString(
		"apply; or self-medication (e.g. antibiotics or hormones without a prescription) or ",
	)
	b.WriteString(
		"anything else that could delay proper medical care? Set high_risk to true if so — this ",
	)
	b.WriteString(
		"never depends on whether the claim turns out to be true or false, only on the subject ",
	)
	b.WriteString("matter itself.\n\n")
	b.WriteString(
		"Respond with ONLY a JSON object (no markdown, no commentary), with this shape:\n",
	)
	b.WriteString(
		`{"in_scope": true/false, "refusal_reason": "...", ` +
			`"sub_claims": [{"text": "...", "high_risk": true/false}], "language": "en"}` + "\n\n",
	)
	b.WriteString("refusal_reason is only used when in_scope is false (briefly explain why); ")
	b.WriteString(
		"otherwise use \"\". sub_claims is only used when in_scope is true; otherwise use [].\n",
	)
	b.WriteString(
		"Always write each sub-claim's text and refusal_reason in French, regardless of what ",
	)
	b.WriteString(
		"language you detected the input message to be written in (the \"language\" field is for ",
	)
	b.WriteString(
		"detection only) — the system currently only replies in French, even if you expect the ",
	)
	b.WriteString("knowledge base evidence to be in a different language.\n\n")
	b.WriteString("Message:\n\"\"\"\n")
	b.WriteString(message)
	b.WriteString("\n\"\"\"\n")
	return b.String()
}
