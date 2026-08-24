# Use Cases

Concrete scenarios for deploying Wobsongo, grounded in capabilities documented elsewhere in this site.

## Health guidance chatbot for an NGO or ministry of health

Ingest a set of official guidance documents (WHO/national health authority publications, vaccination fact sheets, outbreak advisories) and expose claim-checking behind a messaging-bot integration — the codebase itself anticipates this pattern: `ClaimCheckWorker`'s own documentation notes that async callback support "e.g. WhatsApp bot notifications" is meant to be built by wrapping the claim-checking service directly. Users ask questions in natural language; every answer traces back to a cited passage in an authoritative source document rather than an unsourced model response. See [Claims](../api/claims.md) for the response shape a bot integration would consume.

## Research tool for a fact-checking newsroom

Ingest a body of source documents (scientific literature, official statements, prior fact-checks) and use [Retrieval](web-ui.md#retrieval) as a research aid — journalists can search the knowledge base directly to find and verify supporting or contradicting evidence for a claim under investigation, independent of running a full claim check. The [evidence viewer](web-ui.md#evidence-viewer) lets them pull up the exact page a piece of evidence came from for citation in their own reporting.

## Severity-aware public-health triage

Because every claim-check verdict carries both a truth verdict and a **severity** (routine/serious/emergency) plus a `recommend_medical_consult` flag (see [Concepts](concepts.md)), a hotline or self-service tool built on the API can route responses differently by risk: routine misinformation gets an informational correction, while a claim whose subject matter is flagged serious or emergency-severity can be steered toward recommending professional medical attention rather than treating the automated answer as sufficient on its own.

## Multilingual public information portal

Bilingual (English/French) ingestion and search means a single knowledge base serves both languages without maintaining separate content — a document tagged in one language is discoverable via full-text search in the other automatically (see [Database](../developer/database.md#language-bilingual-search)). Useful for organizations serving mixed-language populations without duplicating their document-ingestion effort per language.
