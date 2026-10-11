package worker

import (
	"flag"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/model"
)

var reportFixture = flag.String("fixture", "parsed_document.json", "JSON fixture in worker/testdata to include in the HTML report")

// Run explicitly with:
// go test ./worker -run '^TestGenerateParsedDocumentReport$' -count=1 -v
func TestGenerateParsedDocumentReport(t *testing.T) {
	if os.Getenv("GENERATE_REPORT") == "" {
		t.Skip("set GENERATE_REPORT=1 to run — writes worker/testdata/parsed_document_report.html")
	}
	raw, err := os.ReadFile(filepath.Join("testdata", *reportFixture))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, err := external.ParseRaw(raw)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	kept, dropped, decisions := filterNoiseChunksDetailed(parsed.Chunks)
	rows := make([]reportRow, 0, len(decisions))
	for _, decision := range decisions {
		statusClass := strings.ToLower(decision.Status)
		rows = append(rows, reportRow{
			Index:       decision.OriginalIndex,
			KeptIndex:   decision.KeptIndex,
			Page:        decision.Chunk.Page,
			LayoutType:  string(decision.Chunk.LayoutType),
			Text:        decision.Chunk.Text,
			BoundingBox: decision.Chunk.BoundingBox,
			Status:      decision.Status,
			StatusClass: statusClass,
			Reason:      decision.Reason,
		})
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
