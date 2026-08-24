# Claims

`POST /api/v1/claims/check` is the main entry point for third-party integrations — check a claim (a question, a casual statement, a sentence pulled from a video transcript) against the knowledge base and get back a cited verdict.

## Request

```json
{
  "text": "Does drinking cold water cause a cold?",
  "is_long": false
}
```

- `text` (required) — the claim/message to check. Can be a direct question or an arbitrary sentence.
- `is_long` (optional, default `false`) — controls the verbosity of `formatted_message` only: `false` gives a brief paraphrased explainer per sub-claim, `true` gives full reasoning plus citations. `sub_claims`/`citations` in the structured response are always fully populated either way.

## Response

**Important:** out-of-scope input (not health-related) is a normal `200` with `in_scope: false` — not an error. This system only takes health-related inquiries.

```json
{
  "data": {
    "in_scope": true,
    "overall_summary": "...",
    "formatted_message": "...",
    "sub_claims": [
      {
        "claim": "Cold water causes a cold",
        "verdict": "contradicted",
        "severity": "routine",
        "recommend_medical_consult": false,
        "reasoning": "...",
        "brief_reasoning": "...",
        "citations": [
          {
            "index": 0,
            "source": "chunk",
            "document_id": "...",
            "chunk_id": "...",
            "text": "..."
          }
        ]
      }
    ]
  }
}
```

When `in_scope` is `false`, `refusal_reason` explains why and every other field (`overall_summary`, `formatted_message`, `sub_claims`) is empty.

### Verdict values

`supported`, `contradicted`, `partially_supported`, `mixed`, `insufficient_evidence` — the last one means the knowledge base had nothing relevant to check the claim against, not that the claim itself is right or wrong.

### Severity values

`routine`, `serious`, `emergency` — combined with `recommend_medical_consult`, this signals when a sub-claim's subject matter warrants pointing the user at a real doctor rather than just an information lookup.

### Citations

Each citation is either a document `chunk` or an extracted `fact` (atomic knowledge). `index` matches the inline `[N]` markers in `reasoning`. Combine `document_id` + `chunk_id` to deep-link into the evidence viewer: `/evidence?document_id=...&chunk_id=...` on the web UI.

## Errors

`400` for a malformed body, `422` if `text` is missing, `500` otherwise — see [Errors](errors.md).
