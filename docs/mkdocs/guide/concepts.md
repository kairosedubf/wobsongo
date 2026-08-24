# Concepts

The vocabulary used throughout this documentation, in the order you'll encounter it.

## Document

The source file you ingest — a PDF, Word doc, or similar. Every document is tagged with a required **language** (`en` or `fr`) at ingestion time — this is never auto-detected, since a wrong guess would silently mis-tag content in a bilingual deployment.

## Chunk

A parsed, page-positioned piece of a document — roughly a paragraph, table, or image, classified by layout type (title, section header, paragraph, list item, table, picture, chart, ...). Chunks are the unit that gets embedded and searched; a document is broken into many chunks during ingestion.

## Atomic knowledge

A single extracted fact — a subject/predicate/object triple — derived from a chunk's text. One chunk can yield multiple atomic-knowledge entries. Each is tagged with:

- a **truth tier**: axiomatic, temporal, probabilistic, subjective, unknown, or invalid — how epistemically reliable the statement is.
- a **category**: clinical (substantive health content) vs. metadata (administrative/bibliographic content about the document itself, e.g. authorship).

## Claim check

The core feature: submit a claim, question, or a sentence pulled from a transcript, and get back a verdict for each sub-claim it decomposes into:

- **Verdict** — `supported`, `contradicted`, `partially_supported`, `mixed`, or `insufficient_evidence`.
- **Severity** — `routine`, `serious`, or `emergency` — signals whether the subject matter warrants recommending a real medical consult.

**`insufficient_evidence` means the knowledge base had nothing relevant to check the claim against — it does not mean the claim itself is right or wrong.** This is the single most common point of confusion for a new integrator; keep it in mind when interpreting results (see also [Claims](../api/claims.md)).

## Citation

A specific chunk or atomic-knowledge fact that a verdict is based on. Citations link back to the [evidence viewer](web-ui.md), which shows the exact passage in its original document context.

## Retrieval

The hybrid search (full-text + vector similarity, across both chunks and atomic-knowledge facts, bilingual) that claim-checking runs internally to find relevant evidence. It's also exposed directly as its own search/inspection tool in the web UI — useful for understanding *why* a verdict came out the way it did, or for exploring the knowledge base independent of claim-checking.
