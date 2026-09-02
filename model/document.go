// Package model contains the data structures and types used in the application.
package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// unknownLabel is the canonical string form for an out-of-range TruthTier or
// FactCategory value.
const unknownLabel = "unknown"

type TruthTier int

const (
	// TruthTierAxiomatic represents axiomatic truth,
	// indicating high factual accuracy and reliability.
	TruthTierAxiomatic TruthTier = iota

	// TruthTierTemporal represents temporal truth,
	// some may no longer be considered true, but they are still based on observable evidence.
	TruthTierTemporal

	// TruthTierProbabilistic represents probabilistic truth,
	// highly dependent on context and subject to change, indicating lower factual accuracy.
	TruthTierProbabilistic

	// TruthTierSubjective represents subjective truth,
	// does not have a strong basis in fact and is highly dependent on personal opinion or perspective.
	TruthTierSubjective

	// TruthTierUnknown represents unknown truth, needing further verification or context.
	TruthTierUnknown

	// TruthTierInvalid represents invalid truth, indicating false or misleading information.
	TruthTierInvalid
)

// truthTierNames is the canonical string form of each TruthTier, used both
// for String() and ParseTruthTier — the wire format an LLM extraction
// response communicates tiers in, independent of how they're persisted.
var truthTierNames = map[TruthTier]string{
	TruthTierAxiomatic:     "axiomatic",
	TruthTierTemporal:      "temporal",
	TruthTierProbabilistic: "probabilistic",
	TruthTierSubjective:    "subjective",
	TruthTierUnknown:       unknownLabel,
	TruthTierInvalid:       "invalid",
}

// String returns t's canonical lowercase name, or "unknown" for an
// out-of-range value.
func (t TruthTier) String() string {
	if name, ok := truthTierNames[t]; ok {
		return name
	}
	return unknownLabel
}

// ParseTruthTier parses s (case-insensitive) into a TruthTier, matching the
// names String() produces. Returns an error for anything else.
func ParseTruthTier(s string) (TruthTier, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for tier, name := range truthTierNames {
		if name == s {
			return tier, nil
		}
	}
	return TruthTierUnknown, fmt.Errorf("unrecognized truth tier %q", s)
}

// FactCategory classifies whether an extracted fact is substantive clinical
// content or administrative/bibliographic metadata about the document
// itself — an axis orthogonal to TruthTier (epistemic reliability). Coarse
// by design: a fine-grained taxonomy (bibliographic vs. authorship vs.
// administrative vs. document-structure) is harder for an LLM to apply
// consistently than a binary clinical/not distinction, and the actual
// filtering need is binary.
type FactCategory int

const (
	// FactCategoryClinical represents a substantive clinical/scientific
	// claim, finding, or recommendation.
	FactCategoryClinical FactCategory = iota

	// FactCategoryMetadata represents administrative/bibliographic content
	// about the document itself — authorship, affiliations, citations,
	// guideline-development process, document structure — not clinical
	// content.
	FactCategoryMetadata

	// FactCategoryUnknown represents a fact whose category is genuinely
	// unclear. Unlike FactCategoryMetadata, this is never discarded —
	// erring toward recall for ambiguous cases.
	FactCategoryUnknown
)

// factCategoryNames is the canonical string form of each FactCategory, used
// both for String() and ParseFactCategory — the wire format an LLM
// extraction response communicates categories in, independent of how
// they're persisted.
var factCategoryNames = map[FactCategory]string{
	FactCategoryClinical: "clinical",
	FactCategoryMetadata: "metadata",
	FactCategoryUnknown:  unknownLabel,
}

// String returns c's canonical lowercase name, or "unknown" for an
// out-of-range value.
func (c FactCategory) String() string {
	if name, ok := factCategoryNames[c]; ok {
		return name
	}
	return unknownLabel
}

// ParseFactCategory parses s (case-insensitive) into a FactCategory,
// matching the names String() produces. Returns an error for anything else.
func ParseFactCategory(s string) (FactCategory, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for category, name := range factCategoryNames {
		if name == s {
			return category, nil
		}
	}
	return FactCategoryUnknown, fmt.Errorf("unrecognized fact category %q", s)
}

// Language identifies the language a document (and everything derived from
// it — chunks, facts) is written in. Only English/French are supported —
// see internal/model/document.go's Document.Language doc comment for why
// this is set explicitly at ingestion rather than auto-detected.
type Language int

const (
	// LanguageEnglish is the default/zero value, matching existing
	// pre-bilingual-support data (all English) without a backfill migration.
	LanguageEnglish Language = iota
	LanguageFrench
)

// languageNames is the canonical string form of each Language, used both for
// String() and ParseLanguage — the wire format the CLI/API communicate
// languages in, independent of how they're persisted (a plain INTEGER,
// matching this schema's existing convention for TruthTier/FactCategory
// rather than a native Postgres ENUM type).
var languageNames = map[Language]string{
	LanguageEnglish: "en",
	LanguageFrench:  "fr",
}

// String returns l's canonical ISO-639-1 code, or "en" for an out-of-range value.
func (l Language) String() string {
	if name, ok := languageNames[l]; ok {
		return name
	}
	return "en"
}

// ParseLanguage parses s (case-insensitive) into a Language, matching the
// names String() produces. Returns an error for anything else.
func ParseLanguage(s string) (Language, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for language, name := range languageNames {
		if name == s {
			return language, nil
		}
	}
	return LanguageEnglish, fmt.Errorf("unrecognized language %q", s)
}

// Other returns the other of the two supported languages. Only meaningful
// while exactly two languages are supported — see the bilingual-support
// plan's documented migration path for a third language, which would need a
// side-table rather than this kind of binary toggle.
func (l Language) Other() Language {
	if l == LanguageEnglish {
		return LanguageFrench
	}
	return LanguageEnglish
}

// LayoutType represents the structural classification assigned by Docling
// to a specific parsed document element.
type LayoutType string

const (
	// Structural Text Elements
	LayoutTypeTitle         LayoutType = "title"
	LayoutTypeSectionHeader LayoutType = "section_header"
	LayoutTypeParagraph     LayoutType = "paragraph"
	LayoutTypeListItem      LayoutType = "list_item"
	LayoutTypeCode          LayoutType = "code"
	LayoutTypeCaption       LayoutType = "caption"
	LayoutTypeFootnote      LayoutType = "footnote"
	LayoutTypeReference     LayoutType = "reference"

	// Tabular and Visual Elements
	LayoutTypeTable   LayoutType = "table"
	LayoutTypePicture LayoutType = "picture"
	LayoutTypeChart   LayoutType = "chart"
	LayoutTypeFormula LayoutType = "formula"

	// Noise and Artifacts
	LayoutTypePageHeader    LayoutType = "page_header"
	LayoutTypePageFooter    LayoutType = "page_footer"
	LayoutTypeDocumentIndex LayoutType = "document_index"

	// Forms and Key-Value Pairs
	LayoutTypeKeyValueRegion     LayoutType = "key_value_region"
	LayoutTypeCheckboxSelected   LayoutType = "checkbox_selected"
	LayoutTypeCheckboxUnselected LayoutType = "checkbox_unselected"
	LayoutTypeForm               LayoutType = "form"
)

// Document represents a document ingested into the knowledge base.
type Document struct {
	// ID is the unique identifier for the document.
	ID uuid.UUID `json:"id" binding:"required"`

	// CreatedAt is the timestamp when the document was created.
	CreatedAt time.Time `json:"created_at" binding:"required"`

	// ModifiedAt is the timestamp when the document was last modified.
	ModifiedAt time.Time `json:"modified_at" binding:"required"`

	// IngestedAt is the timestamp when the document was ingested into the system.
	IngestedAt *time.Time `json:"ingested_at,omitempty"`

	// FileURL is the object storage link (e.g., MinIO/S3) to the document file,
	// but it only stores the S3 key, not the full URL.
	FileURL S3Key `json:"-" binding:"required"`

	// FileURLPresigned is the presigned URL for accessing the document file.
	FileURLPresigned string `json:"file_url_presigned,omitempty"`

	// SHA256 is the SHA256 hash of the document content.
	SHA256 string `json:"sha256" binding:"required"`

	// Title is the title of the document.
	Title string `json:"title" binding:"required"`

	// Filename is the name of the document file.
	Filename string `json:"filename" binding:"required"`

	// Filetype is the mime type of the document file (e.g., "application/pdf", "text/plain").
	Filetype string `json:"filetype" binding:"required"`

	// Filesize is the size of the document file in bytes.
	Filesize int64 `json:"filesize" binding:"required"`

	// PageCount is the number of pages in the document (if applicable).
	PageCount int `json:"page_count" binding:"required"`

	// PublisherName is the name of the publisher of the document.
	PublisherName string `json:"publisher_name"`

	// PublicationYear is the year the document was published.
	PublicationYear int `json:"publication_year"`

	// Language is the language this document is written in, set explicitly
	// at ingestion (e.g. `wob document insert --language fr`) rather than
	// auto-detected — the cheapest, most reliable option for now.
	Language Language `json:"language" binding:"required"`
}

// BoundingBox defines the coordinates of a text element on a PDF page layout.
// Docling uses [left, top, right, bottom] relative to the page dimensions.
type BoundingBox [4]float64

// ParsedChunk represents a chunk of parsed content from a document,
// typically paragraph-sized text with positioning information for context.
// This struct is not stored in the database but is
// used for processing and analysis of document content.
type ParsedChunk struct {
	// Text is the actual text content of the chunk.
	Text string `json:"text" binding:"required"`

	// TableMarkdown is the complete Markdown representation of a table. Text
	// remains the descriptive representation used by retrieval and judging.
	TableMarkdown string `json:"table_markdown,omitempty"`

	// Page is the page number in the document where this chunk was found.
	Page int `json:"page" binding:"required"`

	// Chapter is optional chapter information for the chunk, if applicable.
	Chapter string `json:"chapter,omitempty"`

	// LayoutType indicates the structural classification of the chunk.
	LayoutType LayoutType `json:"layout_type" binding:"required"`

	// BoundingBox defines the coordinates of the text chunk on the page,
	BoundingBox BoundingBox `json:"bounding_box" binding:"required"`

	// AssetURL stores the object storage link (e.g., MinIO/S3) to the extracted image file.
	// This is only populated if LayoutType is LayoutTypePicture or LayoutTypeChart.
	AssetURL string `json:"asset_url,omitempty"`

	// RawImageData holds the decoded image bytes for LayoutTypePicture/
	// LayoutTypeChart chunks immediately after parsing, before they've been
	// uploaded to object storage and AssetURL populated. Never persisted —
	// purely an in-memory handoff between Docling-response mapping and the
	// image-upload step; cleared before the chunk is written to the DB.
	RawImageData []byte `json:"-"`

	// RawImageContentType is the MIME type (e.g. "image/png") declared by
	// Docling's data: URI for RawImageData. Same lifecycle as RawImageData.
	RawImageContentType string `json:"-"`
}

// DocumentChunk represents a chunk of a document that is stored in the database.
type DocumentChunk struct {
	// ID is the unique identifier for the document chunk.
	ID uuid.UUID `json:"id" binding:"required"`

	// CreatedAt is the timestamp when the data was created.
	CreatedAt time.Time `json:"created_at" binding:"required"`

	// UpdatedAt is the timestamp when the data was last modified.
	UpdatedAt time.Time `json:"updated_at" binding:"required"`

	// DocumentID is the unique identifier of the document to which this chunk belongs.
	DocumentID uuid.UUID `json:"document_id" binding:"required"`

	// SequenceNumber tracks the absolute reading order of this chunk
	// within the parent document (0-indexed), as produced by the parser.
	SequenceNumber int `json:"sequence_number" binding:"required"`

	// Topics is a list of topics associated with this document chunk,
	// used for categorization and retrieval.
	Topics []string `json:"topics" binding:"required"`

	// FactualityScore is a score representing the factual accuracy of the content in this chunk,
	// indicating how reliable the information is based on the source and context.
	// Text with little to no factual basis will have a low score,
	// while text with strong factual support will have a high score.
	FactualityScore float64 `json:"factuality_score" binding:"required"`

	// Embedding is a vector representation of the chunk's content,
	// used for semantic search and similarity comparisons in the knowledge base.
	Embedding []float32 `json:"-"`

	// KnowledgeExtractedAt is when atomic-knowledge extraction last ran for
	// this chunk, or nil if it hasn't run yet. Set even when extraction
	// found zero facts, so "not yet processed" stays distinguishable from
	// "processed, found nothing."
	KnowledgeExtractedAt *time.Time `json:"-"`

	// Language is copied down from the parent Document at chunk-insert time
	// (an immutable parent attribute, not redundant denormalization) —
	// needed on this row directly because Postgres generated columns can't
	// reference another table, and text_fts_en/text_fts_fr need to know
	// which language `text` itself is in.
	Language Language `json:"language" binding:"required"`

	// TextTranslated is a translation of Text into whichever of English/French
	// isn't Language, populated once at ingestion by TranslateChunksWorker.
	// Empty until translated — never shown in citations/display, which
	// always use Text; purely feeds the "other" language's full-text search.
	TextTranslated string `json:"-"`

	ParsedChunk
}

// AtomicKnowledge represents a single unit of knowledge in the system,
// parsed from a specific document chunk and associated with a truth tier.
// One chunk may contain multiple atomic knowledge entries, each
// representing a distinct fact or piece of information.
type AtomicKnowledge struct {
	// ID is the unique identifier for the atomic knowledge entry.
	ID uuid.UUID `json:"id" binding:"required"`

	// CreatedAt is the timestamp when the data was created.
	CreatedAt time.Time `json:"created_at" binding:"required"`

	// UpdatedAt is the timestamp when the data was last modified.
	UpdatedAt time.Time `json:"updated_at" binding:"required"`

	// DocumentID is the unique identifier of the document from which this knowledge was derived.
	DocumentID uuid.UUID `json:"document_id" binding:"required"`

	// DocumentChunkID is the unique identifier of the document chunk from which this knowledge was derived.
	DocumentChunkID uuid.UUID `json:"document_chunk_id" binding:"required"`

	// TruthTier represents the level of factual accuracy and reliability of the knowledge statement.
	TruthTier TruthTier `json:"truth_tier" binding:"required"`

	// Category distinguishes substantive clinical content from
	// administrative/bibliographic metadata about the document itself.
	Category FactCategory `json:"category" binding:"required"`

	// Topics is a list of topics associated with this atomic knowledge entry.
	Topics []string `json:"topics" binding:"required"`

	// Subject is the subject of the knowledge statement, representing the entity or concept being described.
	Subject string `json:"subject" binding:"required"`

	// Predicate is the predicate of the knowledge statement, representing the relationship or action associated with the subject.
	Predicate string `json:"predicate" binding:"required"`

	// Object is the object of the knowledge statement, representing the entity or concept that is related to the subject through the predicate.
	Object string `json:"object" binding:"required"`

	// Note is an optional note providing additional context or information about the knowledge statement.
	Note string `json:"note,omitempty"`

	// Embedding is a vector representation of the knowledge statement.
	Embedding []float32 `json:"-"`

	// MarkedAsInvalid indicates whether the knowledge statement has been marked as invalid.
	MarkedAsInvalid bool `json:"marked_as_invalid" binding:"required"`

	// MarkedAsIrrelevant indicates whether the knowledge statement has been marked as irrelevant.
	MarkedAsIrrelevant bool `json:"marked_as_irrelevant" binding:"required"`

	// Language is copied down from the parent DocumentChunk at extraction
	// time — see DocumentChunk.Language's doc comment for why this can't
	// just be a join to the parent document/chunk.
	Language Language `json:"language" binding:"required"`

	// SearchTextTranslated is a translation of SPOText() into whichever of
	// English/French isn't Language — a single concatenated blob purely for
	// full-text search, not per-field, and never shown in citations/display
	// (which always use Subject/Predicate/Object/Note verbatim). Empty until
	// translated (folded into the same extraction call, see
	// ExtractedFact.TranslatedSearchText).
	SearchTextTranslated string `json:"-"`
}

// SPOText builds the canonical text representation of a fact: "{Subject}
// {Predicate} {Object}", with Note appended if non-empty. Single source of
// truth for what gets embedded (internal/worker/embed_knowledge.go) and what
// gets displayed for a fact search hit (internal/service/rag.go) — these
// must stay identical, or search results won't reflect what was actually
// embedded.
func (k *AtomicKnowledge) SPOText() string {
	var b strings.Builder
	b.WriteString(k.Subject)
	b.WriteByte(' ')
	b.WriteString(k.Predicate)
	b.WriteByte(' ')
	b.WriteString(k.Object)
	if k.Note != "" {
		b.WriteByte(' ')
		b.WriteString(k.Note)
	}
	return b.String()
}
