package worker

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/model"
)

// Run explicitly with:
// go test ./worker -run '^TestGenerateParsedDocumentReport$' -count=1 -v
func TestGenerateParsedDocumentReport(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "parsed_document.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, err := external.ParseRaw(raw)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	kept, dropped := filterNoiseChunks(parsed.Chunks)
	rows := make([]reportRow, 0, len(parsed.Chunks))
	keptIndex := 0
	for i := 0; i < len(parsed.Chunks); i++ {
		chunk := parsed.Chunks[i]
		if chunk.LayoutType == model.LayoutTypeSectionHeader &&
			strings.TrimSpace(chunk.Text) != "" &&
			i+1 < len(parsed.Chunks) &&
			parsed.Chunks[i+1].LayoutType == model.LayoutTypeListItem {
			for i+1 < len(parsed.Chunks) && parsed.Chunks[i+1].LayoutType == model.LayoutTypeListItem {
				item := parsed.Chunks[i+1]
				rows = append(rows, reportRow{
					Index: i, Page: item.Page, LayoutType: string(item.LayoutType),
					Text: chunk.Text + "\n" + item.Text, BoundingBox: item.BoundingBox,
					KeptIndex: keptIndex, Status: "MERGE", StatusClass: "merge",
				})
				keptIndex++
				i++
			}
			continue
		}
		row := reportRow{
			Index:       i,
			Page:        chunk.Page,
			LayoutType:  string(chunk.LayoutType),
			Text:        chunk.Text,
			BoundingBox: chunk.BoundingBox,
		}
		if chunk.LayoutType == model.LayoutTypeSectionHeader &&
			strings.TrimSpace(chunk.Text) != "" &&
			i+1 < len(parsed.Chunks) &&
			parsed.Chunks[i+1].LayoutType == model.LayoutTypeText &&
			strings.TrimSpace(parsed.Chunks[i+1].Text) != "" {
			paragraph := parsed.Chunks[i+1]
			row.Page = paragraph.Page
			row.LayoutType = string(paragraph.LayoutType)
			row.Text = chunk.Text + "\n" + paragraph.Text
			row.BoundingBox = paragraph.BoundingBox
			row.Status, row.StatusClass, row.KeptIndex = "MERGE", "merge", keptIndex
			keptIndex++
			i++
		} else if noiseLayoutTypes[chunk.LayoutType] {
			row.Status, row.StatusClass, row.Reason = "DROP", "drop", "layout type is configured as noise"
		} else if emptyTextNoiseLayoutTypes[chunk.LayoutType] && strings.TrimSpace(chunk.Text) == "" {
			row.Status, row.StatusClass, row.Reason = "DROP", "drop", "text is empty for a text layout"
		} else if chunk.LayoutType == model.LayoutTypeSectionHeader && len(strings.Fields(chunk.Text)) <= shortSectionHeaderMaxWords {
			row.Status, row.StatusClass, row.Reason = "DROP", "drop", "standalone section header has at most three words"
		} else {
			row.Status, row.StatusClass, row.KeptIndex = "KEEP", "keep", keptIndex
			keptIndex++
		}
		rows = append(rows, row)
	}

	file, err := os.Create(filepath.Join("testdata", "parsed_document_report.html"))
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	defer file.Close()
	data := reportData{Title: parsed.Title, Pages: parsed.PageCount, Total: len(parsed.Chunks), Kept: len(kept), Dropped: dropped, Rows: rows}
	if err := parsedDocumentReportTemplate.Execute(file, data); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("wrote report: %d chunks, %d kept, %d dropped", data.Total, data.Kept, data.Dropped)
}

type reportData struct {
	Title                       string
	Pages, Total, Kept, Dropped int
	Rows                        []reportRow
}
type reportRow struct {
	Index, KeptIndex, Page      int
	LayoutType, Text            string
	BoundingBox                 model.BoundingBox
	Status, StatusClass, Reason string
}

var parsedDocumentReportTemplate = template.Must(template.New("report").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>{{.Title}} — chunk report</title>
<style>
body{font:15px system-ui,sans-serif;margin:32px;background:#111827;color:#f3f4f6}
h1{margin-bottom:4px}.summary{color:#cbd5e1;margin-bottom:24px}
.chunk{background:#1f2937;border:1px solid #374151;border-left:6px solid #6b7280;border-radius:6px;margin:12px 0;padding:14px 18px;white-space:pre-wrap}
.chunk.keep{border-left-color:#34d399}.chunk.merge{border-left-color:#a78bfa;background:#292342}.chunk.drop{border-left-color:#fb7185;background:#351d25}
.meta{font-family:ui-monospace,monospace;color:#cbd5e1;font-size:13px;margin-bottom:8px}
.status{font-weight:700}.keep .status{color:#16803c}.drop .status{color:#c9362b}.reason{color:#c9362b;font-style:italic}
</style></head><body>
<h1>{{.Title}}</h1><div class="summary">{{.Pages}} pages · {{.Total}} chunks · <b>{{.Kept}} kept</b> · <b>{{.Dropped}} dropped</b></div>
{{range .Rows}}<article class="chunk {{.StatusClass}}"><div class="meta"><span class="status">{{.Status}}</span> · original #{{.Index}} {{if eq .Status "KEEP"}}· kept #{{.KeptIndex}}{{end}} · page {{.Page}} · layout <b>{{.LayoutType}}</b> · bbox {{.BoundingBox}}</div>{{if .Reason}}<div class="reason">{{.Reason}}</div>{{end}}<div>{{.Text}}</div></article>{{end}}
</body></html>`))
