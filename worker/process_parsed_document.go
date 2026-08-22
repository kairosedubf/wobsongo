package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
)

// processParsedDocumentTimeout bounds how long the worker waits for image
// uploads to complete. Much shorter than ParseDocumentWorker's Docling
// timeout since this job never calls Docling — everything here is S3/DB I/O.
const processParsedDocumentTimeout = 5 * time.Minute

// noiseLayoutTypes are Docling layout types dropped before persistence — never
// embedded, never shown to an LLM. Per the ingestion pipeline design: filter
// at ingestion time, not query time.
var noiseLayoutTypes = map[model.LayoutType]bool{
	model.LayoutTypePageHeader:    true,
	model.LayoutTypePageFooter:    true,
	model.LayoutTypeDocumentIndex: true,
}

const shortSectionHeaderMaxWords = 3

var emptyTextNoiseLayoutTypes = map[model.LayoutType]bool{
	model.LayoutTypeParagraph:     true,
	model.LayoutTypeText:          true,
	model.LayoutTypeTitle:         true,
	model.LayoutTypeSectionHeader: true,
	model.LayoutTypeListItem:      true,
	model.LayoutTypeCaption:       true,
	model.LayoutTypeFootnote:      true,
	model.LayoutTypeReference:     true,
}

// contentTypeImageJPEG is shared between imageContentTypeExtensions and
// caption_image_chunks.go's extensionContentTypes to avoid duplicating the
// "image/jpeg" literal.
const contentTypeImageJPEG = "image/jpeg"

// imageContentTypeExtensions maps a decoded image's declared MIME type to
// the file extension used in its S3 key — must stay a subset of
// validation.regexSHA256Filename's extension whitelist.
var imageContentTypeExtensions = map[string]string{
	"image/png":          "png",
	contentTypeImageJPEG: "jpeg",
	"image/webp":         "webp",
	"image/avif":         "avif",
}

// ProcessParsedDocumentWorker is a River worker that turns a document's raw
// Docling output (fetched && stored by ParseDocumentWorker) into stored
// chunks: it never calls Docling itself, so it can be retried freely
// without re-paying for that external call.
type ProcessParsedDocumentWorker struct {
	// Embedding River's default worker behavior for the specific DTO.
	river.WorkerDefaults[queue.ProcessParsedDocumentDTO]
	// RawStore reads the raw Docling response back from S3, && stores any
	// images extracted from it.
	RawStore data.RawObjectStore
	// DocumentService backfills the document's page count && (if blank)
	// title once Docling has actually parsed it.
	DocumentService *service.DocumentService
	// ChunkRepo stores the chunks that survive filtering.
	ChunkRepo data.DocumentChunkRepoer
}

// NewProcessParsedDocumentWorker is a constructor for ProcessParsedDocumentWorker.
func NewProcessParsedDocumentWorker(
	rawStore data.RawObjectStore,
	documentService *service.DocumentService,
	chunkRepo data.DocumentChunkRepoer,
) *ProcessParsedDocumentWorker {
	return &ProcessParsedDocumentWorker{
		RawStore:        rawStore,
		DocumentService: documentService,
		ChunkRepo:       chunkRepo,
	}
}

// Timeout overrides River's default job timeout.
func (w *ProcessParsedDocumentWorker) Timeout(
	*river.Job[queue.ProcessParsedDocumentDTO],
) time.Duration {
	return processParsedDocumentTimeout
}

// Work is the main method that gets called when a job is dequeued.
func (w *ProcessParsedDocumentWorker) Work(
	ctx context.Context,
	job *river.Job[queue.ProcessParsedDocumentDTO],
) error {
	rc, err := w.RawStore.GetObject(ctx, job.Args.RawOutputKey)
	if err != nil {
		return fmt.Errorf(
			"failed to fetch raw docling output for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}
	raw, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return fmt.Errorf(
			"failed to read raw docling output for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}

	result, err := external.ParseRaw(raw)
	if err != nil {
		return fmt.Errorf(
			"failed to parse raw docling output for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}

	if err := w.DocumentService.UpdateAfterParse(
		ctx,
		job.Args.DocumentID,
		result.PageCount,
		result.Title,
	); err != nil {
		return fmt.Errorf(
			"failed to update document %s after parsing: %w",
			job.Args.DocumentID,
			err,
		)
	}

	doc, err := w.DocumentService.GetByID(ctx, job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf("failed to fetch document %s: %w", job.Args.DocumentID, err)
	}

	kept, dropped := filterNoiseChunks(result.Chunks)
	log.Printf(
		"[ProcessParsedDocumentWorker] document=%s title=%q page_count=%d chunks_kept=%d chunks_dropped=%d",
		job.Args.DocumentID,
		result.Title,
		result.PageCount,
		len(kept),
		dropped,
	)

	var stored int
	var imageChunkIDs []uuid.UUID
	err = w.ChunkRepo.WithTx(ctx, func(tx data.DocumentChunkRepoer) error {
		now := time.Now()
		toStore := make([]model.DocumentChunk, 0, len(kept))
		for i := range kept {
			c := &kept[i]
			if len(c.RawImageData) > 0 {
				assetURL, err := w.uploadImage(ctx, c.RawImageContentType, c.RawImageData)
				if err != nil {
					return fmt.Errorf("failed to upload image for chunk %d: %w", i, err)
				}
				c.AssetURL = assetURL
				c.RawImageData = nil
				c.RawImageContentType = ""
			}

			chunk := model.DocumentChunk{
				ID:             uuid.New(),
				CreatedAt:      now,
				UpdatedAt:      now,
				DocumentID:     job.Args.DocumentID,
				SequenceNumber: i,
				// Topics must be a non-nil empty slice, not the zero value: the
				// batch insert below goes through Postgres's COPY protocol
				// (sqlc :copyfrom), which sends every column's literal value —
				// a nil slice encodes as SQL NULL over COPY && violates
				// topics' NOT NULL constraint, since COPY never falls back to
				// a column's DEFAULT the way a plain INSERT would.
				Topics:      []string{},
				Language:    doc.Language,
				ParsedChunk: *c,
			}
			ok, err := tx.ShouldBeStored(ctx, *doc, chunk)
			if err != nil {
				return fmt.Errorf("failed to evaluate chunk %d for storage: %w", i, err)
			}
			if !ok {
				continue
			}
			toStore = append(toStore, chunk)
			if chunk.AssetURL != "" {
				imageChunkIDs = append(imageChunkIDs, chunk.ID)
			}
		}
		stored = len(toStore)
		if err := tx.CreateBatch(ctx, toStore); err != nil {
			return fmt.Errorf("failed to store chunks: %w", err)
		}

		if len(imageChunkIDs) > 0 {
			if err := tx.Enqueue(ctx, queue.CaptionImageChunksDTO{
				DocumentID: job.Args.DocumentID,
				ChunkIDs:   imageChunkIDs,
			}); err != nil {
				return fmt.Errorf("failed to enqueue image captioning: %w", err)
			}
		} else {
			// No images to caption, so every stored chunk's text is already
			// final — embed && extract knowledge now rather than waiting on
			// a captioning job that will never run. If there were images,
			// CaptionImageChunksWorker enqueues both jobs itself once
			// captioning finishes.
			if err := tx.Enqueue(ctx, queue.EmbedChunksDTO{
				DocumentID: job.Args.DocumentID,
			}); err != nil {
				return fmt.Errorf("failed to enqueue chunk embedding: %w", err)
			}
			if err := tx.Enqueue(ctx, queue.ExtractKnowledgeDTO{
				DocumentID: job.Args.DocumentID,
			}); err != nil {
				return fmt.Errorf("failed to enqueue knowledge extraction: %w", err)
			}
			if err := tx.Enqueue(ctx, queue.TranslateChunksDTO{
				DocumentID: job.Args.DocumentID,
			}); err != nil {
				return fmt.Errorf("failed to enqueue chunk translation: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to store chunks for document %s: %w", job.Args.DocumentID, err)
	}

	log.Printf(
		"[ProcessParsedDocumentWorker] document=%s stored=%d/%d chunks, %d awaiting captions",
		job.Args.DocumentID, stored, len(kept), len(imageChunkIDs),
	)
	return nil
}

// uploadImage stores imageBytes under the document_image intent's prefix,
// keyed by content hash (matching the sha256hex.<ext> convention used for
// uploaded documents, so the result stays presigned-GET-able later), and
// returns the S3 key to store as the chunk's AssetURL.
func (w *ProcessParsedDocumentWorker) uploadImage(
	ctx context.Context,
	contentType string,
	imageBytes []byte,
) (string, error) {
	ext, ok := imageContentTypeExtensions[contentType]
	if !ok {
		return "", fmt.Errorf("unsupported image content type %q", contentType)
	}

	hash := sha256.Sum256(imageBytes)
	key := fmt.Sprintf(
		"%s%x.%s",
		data.ObjectPrefixForIntent(data.DocumentImageUploadIntent),
		hash,
		ext,
	)

	if err := w.RawStore.PutObject(
		ctx,
		key,
		bytes.NewReader(imageBytes),
		int64(len(imageBytes)),
		contentType,
	); err != nil {
		return "", err
	}
	return key, nil
}

// filterNoiseChunks drops chunks whose layout type is considered noise
// (page headers/footers, table of contents), returning the kept chunks and
// a count of how many were dropped.
func filterNoiseChunks(chunks []model.ParsedChunk) (kept []model.ParsedChunk, dropped int) {
	for i := 0; i < len(chunks); i++ {
		chunk := chunks[i]
		if chunk.LayoutType == model.LayoutTypeSectionHeader &&
			strings.TrimSpace(chunk.Text) != "" &&
			i+1 < len(chunks) &&
			chunks[i+1].LayoutType == model.LayoutTypeListItem {
			headerText := chunk.Text
			for i+1 < len(chunks) && chunks[i+1].LayoutType == model.LayoutTypeListItem {
				merged := chunks[i+1]
				merged.Text = headerText + "\n" + merged.Text
				kept = append(kept, merged)
				i++
			}
			continue
		}
		if chunk.LayoutType == model.LayoutTypeSectionHeader &&
			strings.TrimSpace(chunk.Text) != "" &&
			i+1 < len(chunks) &&
			chunks[i+1].LayoutType == model.LayoutTypeText &&
			strings.TrimSpace(chunks[i+1].Text) != "" {
			merged := chunks[i+1]
			merged.Text = chunk.Text + "\n" + merged.Text
			kept = append(kept, merged)
			i++
			continue
		}
		if noiseLayoutTypes[chunk.LayoutType] {
			dropped++
			continue
		}
		if emptyTextNoiseLayoutTypes[chunk.LayoutType] && strings.TrimSpace(chunk.Text) == "" {
			dropped++
			continue
		}
		if chunk.LayoutType == model.LayoutTypeSectionHeader &&
			len(strings.Fields(chunk.Text)) <= shortSectionHeaderMaxWords {
			dropped++
			continue
		}
		kept = append(kept, chunk)
	}
	return kept, dropped
}
