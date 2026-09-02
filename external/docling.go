package external

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
)

// DoclingClient implements data.DocumentProcessor using a Docling Serve HTTP API instance.
type DoclingClient struct {
	baseURL    string
	httpClient *http.Client
}

// Ensure DoclingClient implements data.DocumentProcessor.
var _ data.DocumentProcessor = (*DoclingClient)(nil)

// doclingHTTPTimeout bounds the HTTP call to Docling Serve. Kept below the
// hosting deployment's own server-side ceiling (e.g. a serverless function
// timeout) so a slow-but-still-succeeding conversion doesn't outrace it in
// the other direction — this client giving up before the server does would
// just mean paying for the conversion without getting the result. Must stay
// less than parseDocumentTimeout (internal/worker/parse_document.go), which
// wraps this call plus S3 storage and enqueueing afterward.
const doclingHTTPTimeout = 9 * time.Minute

// NewDoclingClient creates a new DoclingClient targeting the given Docling Serve base URL.
func NewDoclingClient(baseURL string) *DoclingClient {
	return &DoclingClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: doclingHTTPTimeout,
		},
	}
}

// doclingConvertRequest is the request payload for Docling Serve's /v1/convert/source endpoint.
type doclingConvertRequest struct {
	Sources []doclingSource `json:"sources"`
	Options doclingOptions  `json:"options"`
}

type doclingSource struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

type doclingOptions struct {
	ToFormats       []string `json:"to_formats"`
	ImageExportMode string   `json:"image_export_mode"`

	// IncludeImages controls per-picture crop embedding — required for image
	// captioning to receive real image data. Sent explicitly even though it
	// matches the documented default, since getting this wrong silently
	// breaks captioning (every picture back with a null "image" field) rather
	// than erroring: this deployment ran an older docling-serve version
	// (v1.18.0) where include_images/include_page_images weren't independent
	// controls at all — image generation was gated entirely by
	// image_export_mode, so "embedded" could only ever produce full-page
	// renders, never picture crops. Fixed upstream in docling-jobkit's
	// include_page_images introduction (docling-serve v1.22.1+); confirmed
	// working correctly after this deployment was upgraded to v1.26.0.
	IncludeImages bool `json:"include_images"`

	// IncludePageImages must be false — full-page renders are never read
	// (doclingDocument has no Pages field) and are the overwhelming majority
	// of response size otherwise. Only meaningful as an independent control
	// from IncludeImages on docling-serve v1.22.1+; see IncludeImages's
	// comment for why this deployment needed upgrading before that was true.
	IncludePageImages bool `json:"include_page_images"`

	// TableMode trades TableFormer's extra accuracy for speed. "accurate"
	// (Docling's default) is markedly slower than "fast" — worth paying for
	// only once something actually consumes the extra precision. Right now
	// nothing does: doclingTableItem deliberately doesn't map table
	// cell/grid data at all yet (see its doc comment), so "accurate" mode's
	// benefit is entirely wasted today.
	TableMode string `json:"table_mode"`
}

// doclingServeResponse represents the root JSON object returned by Docling Serve.
type doclingServeResponse struct {
	Document struct {
		JSONContent doclingDocument `json:"json_content"`
	} `json:"document"`
	Status string `json:"status"`
}

// doclingDocument is the native unified document format exported by docling-core.
type doclingDocument struct {
	Name     string               `json:"name"`
	Texts    []doclingTextItem    `json:"texts"`
	Tables   []doclingTableItem   `json:"tables"`
	Pictures []doclingPictureItem `json:"pictures"`
}

type doclingTextItem struct {
	Text  string        `json:"text"`
	Label string        `json:"label"`
	Prov  []doclingProv `json:"prov"`
}

type doclingTableItem struct {
	Label string           `json:"label"`
	Prov  []doclingProv    `json:"prov"`
	Data  doclingTableData `json:"data"`
}

type doclingTableData struct {
	TableCells []doclingTableCell `json:"table_cells"`
}

type doclingTableCell struct {
	Text              string `json:"text"`
	StartRowOffsetIdx int    `json:"start_row_offset_idx"`
	StartColOffsetIdx int    `json:"start_col_offset_idx"`
}

func tableToText(table doclingTableItem) string {
	rows := make(map[int][]doclingTableCell)
	for _, cell := range table.Data.TableCells {
		rows[cell.StartRowOffsetIdx] = append(rows[cell.StartRowOffsetIdx], cell)
	}
	rowIndexes := make([]int, 0, len(rows))
	for row := range rows {
		rowIndexes = append(rowIndexes, row)
	}
	sort.Ints(rowIndexes)

	var b strings.Builder
	b.WriteString("Table:")
	if len(rowIndexes) == 0 {
		return b.String()
	}

	headers := make(map[int]string)
	for _, cell := range rows[rowIndexes[0]] {
		headers[cell.StartColOffsetIdx] = strings.TrimSpace(cell.Text)
	}

	for rowPosition, rowIndex := range rowIndexes[1:] {
		cells := rows[rowIndex]
		sort.SliceStable(cells, func(i, j int) bool {
			return cells[i].StartColOffsetIdx < cells[j].StartColOffsetIdx
		})
		b.WriteString("\n\n- Row ")
		b.WriteString(strconv.Itoa(rowPosition + 2))
		b.WriteString(": ")
		for i, cell := range cells {
			if i > 0 {
				b.WriteString("; ")
			}
			header := headers[cell.StartColOffsetIdx]
			if header != "" {
				b.WriteString(header)
				b.WriteString(": ")
			}
			b.WriteString(strings.TrimSpace(cell.Text))
		}
	}

	return b.String()
}

func tableToMarkdown(table doclingTableItem) string {
	rows := make(map[int][]doclingTableCell)
	maxColumn := 0
	for _, cell := range table.Data.TableCells {
		rows[cell.StartRowOffsetIdx] = append(rows[cell.StartRowOffsetIdx], cell)
		maxColumn = max(maxColumn, cell.StartColOffsetIdx)
	}
	rowIndexes := make([]int, 0, len(rows))
	for row := range rows {
		rowIndexes = append(rowIndexes, row)
	}
	sort.Ints(rowIndexes)
	if len(rowIndexes) == 0 {
		return ""
	}

	columns := maxColumn + 1
	var b strings.Builder
	writeRow := func(rowIndex int) {
		cells := make(map[int]string)
		for _, cell := range rows[rowIndex] {
			cells[cell.StartColOffsetIdx] = strings.TrimSpace(cell.Text)
		}
		b.WriteString("| ")
		for column := 0; column < columns; column++ {
			if column > 0 {
				b.WriteString(" | ")
			}
			value := strings.ReplaceAll(cells[column], "|", "\\|")
			b.WriteString(strings.ReplaceAll(value, "\n", " "))
		}
		b.WriteString(" |\n")
	}

	writeRow(rowIndexes[0])
	b.WriteString("| ")
	for column := 0; column < columns; column++ {
		if column > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString(" |\n")
	for _, rowIndex := range rowIndexes[1:] {
		writeRow(rowIndex)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

type doclingPictureItem struct {
	Label string        `json:"label"`
	Prov  []doclingProv `json:"prov"`
	Image *doclingImage `json:"image,omitempty"`
}

// doclingProv contains the physical location of an element on the PDF.
// An element can span multiple pages, but only the first entry is used here.
type doclingProv struct {
	PageNo int         `json:"page_no"`
	BBox   doclingBBox `json:"bbox"`
}

// doclingBBox is docling-core's real BoundingBox JSON shape: an object with
// named l/t/r/b fields (plus coord_origin, unused here) — not a bare
// 4-element array. Confirmed against docling-core's own schema
// (docs/DoclingDocument.json) after a real document surfaced an unmarshal
// error against the previous (wrong) []float64 assumption.
type doclingBBox struct {
	Left   float64 `json:"l"`
	Top    float64 `json:"t"`
	Right  float64 `json:"r"`
	Bottom float64 `json:"b"`
}

// doclingImage holds the base64 representation of an extracted image, as a
// "data:<mime-type>;base64,<payload>" URI. Decoded by decodeDataURI in
// mapDoclingDocument.
type doclingImage struct {
	URI string `json:"uri"`
}

// FetchRawFromURL fetches the document at documentURL via Docling Serve's
// synchronous /v1/convert/source endpoint and returns the raw, unparsed
// response body — implements data.DocumentProcessor. The HTTP fetch and the
// response interpretation (ParseRaw) are deliberately separate, independently
// retryable steps: a raw response can be large (embedded images push some
// past 100MB) and expensive to re-fetch, so callers persist it once (e.g. to
// S3) and parse it out of band.
func (c *DoclingClient) FetchRawFromURL(ctx context.Context, documentURL string) ([]byte, error) {
	return c.convertFromURL(ctx, documentURL)
}

// ParseRaw unmarshals Docling Serve's raw JSON response (as returned by
// FetchRawFromURL or ConvertFileRaw) into data.ProcessedDocument. Filtering
// by layout type is left to the caller.
func ParseRaw(raw []byte) (*data.ProcessedDocument, error) {
	var parsed doclingServeResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal docling response: %w", err)
	}

	return mapDoclingDocument(&parsed.Document.JSONContent), nil
}

// ConvertFileRaw uploads r's content directly to Docling Serve's
// /v1/convert/file endpoint and returns the raw, unparsed response body.
// Unlike ConvertFromURLRaw, this doesn't require the document to already be
// reachable via a URL — needed when the caller can't hand Docling Serve a
// fetchable URL (e.g. a local dev S3/MinIO that a cloud-hosted Docling
// instance can't reach).
func (c *DoclingClient) ConvertFileRaw(
	ctx context.Context,
	filename string,
	r io.Reader,
) ([]byte, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("to_formats", "json"); err != nil {
		return nil, fmt.Errorf("failed to write to_formats field: %w", err)
	}
	if err := writer.WriteField("image_export_mode", "embedded"); err != nil {
		return nil, fmt.Errorf("failed to write image_export_mode field: %w", err)
	}
	if err := writer.WriteField("include_images", "true"); err != nil {
		return nil, fmt.Errorf("failed to write include_images field: %w", err)
	}
	if err := writer.WriteField("include_page_images", "false"); err != nil {
		return nil, fmt.Errorf("failed to write include_page_images field: %w", err)
	}
	if err := writer.WriteField("table_mode", "fast"); err != nil {
		return nil, fmt.Errorf("failed to write table_mode field: %w", err)
	}
	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return c.doRequest(ctx, "/v1/convert/file", writer.FormDataContentType(), body)
}

// convertFromURL performs the actual Docling Serve /v1/convert/source call
// and returns the raw response body.
func (c *DoclingClient) convertFromURL(ctx context.Context, documentURL string) ([]byte, error) {
	payload := doclingConvertRequest{
		Sources: []doclingSource{
			{Kind: "http", URL: documentURL},
		},
		Options: doclingOptions{
			ToFormats:         []string{"json"},
			ImageExportMode:   "embedded",
			IncludeImages:     true,
			IncludePageImages: false,
			TableMode:         "fast",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal docling request: %w", err)
	}

	return c.doRequest(ctx, "/v1/convert/source", "application/json", bytes.NewReader(body))
}

// doRequest POSTs body to path on Docling Serve and returns the raw
// response body, shared by every convert variant (URL-based, file-upload).
func (c *DoclingClient) doRequest(
	ctx context.Context,
	path, contentType string,
	body io.Reader,
) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create docling request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call docling serve: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read docling response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"docling serve returned error status: %d. Body: %s",
			resp.StatusCode,
			string(respBytes),
		)
	}

	return respBytes, nil
}

// mapDoclingDocument maps a doclingDocument into data.ProcessedDocument.
//
// Known simplification: Docling returns texts/tables/pictures as three
// separate arrays, not one true cross-type reading-order array, so
// concatenating them in this fixed order is an approximation of the real
// reading order, not a faithful global one. True interleaved ordering (by
// page + position) is deferred until a future sub-task needs it.
func mapDoclingDocument(doc *doclingDocument) *data.ProcessedDocument {
	chunks := make([]model.ParsedChunk, 0, len(doc.Texts)+len(doc.Tables)+len(doc.Pictures))
	maxPage := 0

	for _, t := range doc.Texts {
		page, bbox := firstProv(t.Prov)
		maxPage = max(maxPage, page)
		chunks = append(chunks, model.ParsedChunk{
			Text:        t.Text,
			Page:        page,
			LayoutType:  model.LayoutType(t.Label),
			BoundingBox: bbox,
		})
	}

	for _, t := range doc.Tables {
		page, bbox := firstProv(t.Prov)
		maxPage = max(maxPage, page)
		chunks = append(chunks, model.ParsedChunk{
			Text:          tableToText(t),
			TableMarkdown: tableToMarkdown(t),
			Page:          page,
			LayoutType:    model.LayoutType(t.Label),
			BoundingBox:   bbox,
		})
	}

	for _, p := range doc.Pictures {
		page, bbox := firstProv(p.Prov)
		maxPage = max(maxPage, page)
		chunk := model.ParsedChunk{
			Page:        page,
			LayoutType:  model.LayoutType(p.Label),
			BoundingBox: bbox,
		}
		if p.Image != nil {
			contentType, decoded, err := decodeDataURI(p.Image.URI)
			if err != nil {
				// A malformed/undecodable image shouldn't sink the whole
				// document parse — keep the chunk (page/bbox/layout-type
				// still useful) just without image data to upload later.
				log.Printf("[docling] failed to decode embedded image: %v", err)
			} else {
				chunk.RawImageData = decoded
				chunk.RawImageContentType = contentType
			}
		}
		chunks = append(chunks, chunk)
	}

	return &data.ProcessedDocument{
		Title:     documentTitle(doc),
		PageCount: maxPage,
		Chunks:    chunks,
	}
}

// documentTitle derives a real, content-based title for doc — doc.Name is
// not one: it's Docling's echo of whatever filename/URL it was given to
// parse, and this system always uploads under a content-hash filename (see
// internal/repo/media.go), so doc.Name is never anything but that hash.
// Confirmed against a real document: Docling didn't tag anything "title"
// for it, but its actual title was reliably present as the very first
// "section_header"-labeled chunk — hence that two-step fallback here before
// giving up and using doc.Name as a last resort.
func documentTitle(doc *doclingDocument) string {
	for _, t := range doc.Texts {
		if t.Label == string(model.LayoutTypeTitle) && strings.TrimSpace(t.Text) != "" {
			return t.Text
		}
	}
	for _, t := range doc.Texts {
		if t.Label == string(model.LayoutTypeSectionHeader) && strings.TrimSpace(t.Text) != "" {
			return t.Text
		}
	}
	return doc.Name
}

// firstProv extracts the page number and bounding box from the first
// provenance entry, defaulting to zero values when prov is empty.
func firstProv(prov []doclingProv) (int, model.BoundingBox) {
	if len(prov) == 0 {
		return 0, model.BoundingBox{}
	}
	b := prov[0].BBox
	return prov[0].PageNo, model.BoundingBox{b.Left, b.Top, b.Right, b.Bottom}
}

// decodeDataURI splits a "data:<mime-type>;base64,<payload>" URI (Docling's
// embedded-image format) into its declared content type and decoded bytes.
func decodeDataURI(uri string) (contentType string, decoded []byte, err error) {
	rest, ok := strings.CutPrefix(uri, "data:")
	if !ok {
		return "", nil, errors.New("missing \"data:\" prefix")
	}
	meta, payload, ok := strings.Cut(rest, ",")
	if !ok {
		return "", nil, errors.New("missing comma separator")
	}
	mimeType, ok := strings.CutSuffix(meta, ";base64")
	if !ok {
		return "", nil, fmt.Errorf("unsupported data URI encoding %q (only base64 supported)", meta)
	}
	decoded, err = base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, fmt.Errorf("failed to base64-decode image data: %w", err)
	}
	return mimeType, decoded, nil
}
