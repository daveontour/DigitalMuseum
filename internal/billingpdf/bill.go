// Package billingpdf renders LLM usage statements as PDF (admin export).
package billingpdf

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/go-pdf/fpdf"
)

// BillMeta identifies the archive user and reporting window for the cover page.
type BillMeta struct {
	UserID          int64
	Email           string
	DisplayName     string
	FirstName       string
	FamilyName      string
	PeriodFrom      *time.Time
	PeriodTo        *time.Time
	GeneratedAt     time.Time
	EventsTruncated bool
	MaxEventRows    int // cap used when loading events (e.g. 50000); used in truncation note only
}

// RenderLLMUsageBill builds a PDF bytes slice with summary, per-event lines, and 5-minute buckets.
func RenderLLMUsageBill(
	meta BillMeta,
	sum repository.LLMUsageSummary,
	byProv []repository.ProviderBreakdown,
	byVis []repository.VisitorBreakdown,
	events []repository.LLMUsageEvent,
	buckets []repository.TimeseriesBucket,
) ([]byte, error) {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AddPage()
	pdf.SetTitle("LLM usage statement", false)

	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(0, 10, "LLM usage statement")
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf("User ID: %d", meta.UserID))
	pdf.Ln(6)
	if meta.Email != "" {
		pdf.Cell(0, 6, safeLine("Email: "+meta.Email))
		pdf.Ln(6)
	}
	if meta.DisplayName != "" {
		pdf.Cell(0, 6, safeLine("Display name: "+meta.DisplayName))
		pdf.Ln(6)
	}
	if meta.FirstName != "" || meta.FamilyName != "" {
		pdf.Cell(0, 6, safeLine("Name: "+strings.TrimSpace(meta.FirstName+" "+meta.FamilyName)))
		pdf.Ln(6)
	}
	pdf.Cell(0, 6, "Period (UTC): "+formatPeriod(meta.PeriodFrom, meta.PeriodTo))
	pdf.Ln(6)
	pdf.Cell(0, 6, "Generated (UTC): "+meta.GeneratedAt.UTC().Format(time.RFC3339))
	pdf.Ln(10)

	if meta.EventsTruncated {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.Cell(0, 6, fmt.Sprintf(
			"Note: event detail is limited to the first %d rows (chronological). Summary totals and five-minute buckets still include all usage in the selected period.",
			meta.MaxEventRows,
		))
		pdf.Ln(10)
		pdf.SetFont("Helvetica", "", 10)
	}

	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 8, "Summary")
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf("Total input tokens:  %s", formatInt(sum.TotalInputTokens)))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Total output tokens: %s", formatInt(sum.TotalOutputTokens)))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("API calls (events):   %s", formatInt(sum.EventCount)))
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "B", 11)
	pdf.Cell(0, 7, "By provider")
	pdf.Ln(7)
	pdf.SetFont("Helvetica", "", 9)
	tableHeader(pdf, []string{"Provider", "Input tokens", "Output tokens", "Calls"})
	for _, p := range byProv {
		tableRow(pdf, []string{
			safeLine(p.Provider),
			formatInt(p.InputTokens),
			formatInt(p.OutputTokens),
			formatInt(p.EventCount),
		})
	}
	if len(byProv) == 0 {
		pdf.Cell(0, 6, "(none)")
		pdf.Ln(6)
	}
	pdf.Ln(4)

	pdf.SetFont("Helvetica", "B", 11)
	pdf.Cell(0, 7, "By session (owner vs visitor)")
	pdf.Ln(7)
	pdf.SetFont("Helvetica", "", 9)
	tableHeader(pdf, []string{"Session", "Input tokens", "Output tokens", "Calls"})
	for _, v := range byVis {
		label := "Owner"
		if v.IsVisitor {
			label = "Visitor"
		}
		tableRow(pdf, []string{
			label,
			formatInt(v.InputTokens),
			formatInt(v.OutputTokens),
			formatInt(v.EventCount),
		})
	}
	if len(byVis) == 0 {
		pdf.Cell(0, 6, "(none)")
		pdf.Ln(6)
	}
	pdf.Ln(8)

	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Event detail (%d rows, chronological)", len(events)))
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "", 7)
	// Column widths sum to 273 mm (A4 landscape width minus 12 mm side margins).
	ws := []float64{32, 38, 20, 20, 16, 10, 12, 12, 12, 8, 63, 30}
	headers := []string{"Time (UTC)", "Email", "First", "Family", "Provider", "Visitor", "Key", "In", "Out", "OK", "Error", "Model"}
	headerRowCustom(pdf, ws, headers)
	for _, e := range events {
		vis := "No"
		if e.IsVisitor {
			vis = "Yes"
		}
		model := ""
		if e.ModelName != nil {
			model = *e.ModelName
		}
		keyCol := "-"
		if e.UsedServerLLMKey != nil {
			if *e.UsedServerLLMKey {
				keyCol = "Server"
			} else {
				keyCol = "User"
			}
		}
		okCol := "Y"
		if !e.Succeeded {
			okCol = "N"
		}
		errCol := ""
		if e.ErrorMessage != nil && *e.ErrorMessage != "" {
			errCol = safeLine(*e.ErrorMessage)
		}
		row := []string{
			e.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
			derefOrDash(e.UserEmail),
			derefOrDash(e.UserFirstName),
			derefOrDash(e.UserFamilyName),
			e.Provider,
			vis,
			keyCol,
			strconv.Itoa(e.InputTokens),
			strconv.Itoa(e.OutputTokens),
			okCol,
			errCol,
			safeLine(model),
		}
		dataRowEventDetail(pdf, ws, row, 5)
	}
	if len(events) == 0 {
		pdf.SetFont("Helvetica", "", 10)
		pdf.Cell(0, 6, "(no events in range)")
		pdf.Ln(6)
	}

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Five-minute buckets (%d)", len(buckets)))
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "", 9)
	tableHeader(pdf, []string{"Bucket start (UTC)", "Input tokens", "Output tokens"})
	for _, b := range buckets {
		ensureSpace(pdf, 6)
		tableRow(pdf, []string{
			b.BucketStart.UTC().Format(time.RFC3339),
			formatInt(b.InputTokens),
			formatInt(b.OutputTokens),
		})
	}
	if len(buckets) == 0 {
		pdf.Cell(0, 6, "(none)")
		pdf.Ln(6)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatPeriod(from, to *time.Time) string {
	switch {
	case from != nil && to != nil:
		return fmt.Sprintf("%s .. %s", from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	case from != nil:
		return "from " + from.UTC().Format(time.RFC3339)
	case to != nil:
		return "until " + to.UTC().Format(time.RFC3339)
	default:
		return "all time"
	}
}

func formatInt(n int64) string {
	return strconv.FormatInt(n, 10)
}

func safeLine(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 500 {
		s = s[:500] + "..."
	}
	return s
}

func derefOrDash(p *string) string {
	if p == nil || *p == "" {
		return "-"
	}
	return safeLine(*p)
}

func tableHeader(pdf *fpdf.Fpdf, cols []string) {
	pdf.SetFont("Helvetica", "B", 9)
	w := 273.0 / float64(len(cols)) // printable width: 297 - 12 - 12
	for i := range cols {
		pdf.CellFormat(w, 6, safeLine(cols[i]), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 9)
}

func tableRow(pdf *fpdf.Fpdf, cols []string) {
	w := 273.0 / float64(len(cols))
	for i := range cols {
		pdf.CellFormat(w, 6, safeLine(cols[i]), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(-1)
}

func headerRowCustom(pdf *fpdf.Fpdf, ws []float64, cols []string) {
	pdf.SetFont("Helvetica", "B", 7)
	for i := range cols {
		pdf.CellFormat(ws[i], 5, safeLine(cols[i]), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 7)
}

func dataRowCustom(pdf *fpdf.Fpdf, ws []float64, cols []string) {
	for i := range cols {
		pdf.CellFormat(ws[i], 5, safeLine(cols[i]), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(-1)
}

// dataRowEventDetail draws one event-detail row. Column 10 (error message) wraps using MultiCell;
// other columns use a matching cell height.
func dataRowEventDetail(pdf *fpdf.Fpdf, ws []float64, row []string, lineH float64) {
	pdf.SetFont("Helvetica", "", 7)
	errTxt := row[10]
	lines := pdf.SplitLines([]byte(errTxt), ws[10])
	nLines := len(lines)
	if nLines < 1 {
		nLines = 1
	}
	rowH := float64(nLines) * lineH
	ensureSpace(pdf, rowH+1)

	y0 := pdf.GetY()
	x0 := pdf.GetX()
	x := x0
	for i := 0; i < 10; i++ {
		pdf.SetXY(x, y0)
		pdf.CellFormat(ws[i], rowH, safeLine(row[i]), "1", 0, "L", false, 0, "")
		x += ws[i]
	}
	pdf.SetXY(x, y0)
	pdf.MultiCell(ws[10], lineH, errTxt, "1", "L", false)
	yEnd := pdf.GetY()
	xModel := x + ws[10]
	pdf.SetXY(xModel, y0)
	pdf.CellFormat(ws[11], rowH, safeLine(row[11]), "1", 0, "L", false, 0, "")
	nextY := math.Max(yEnd, y0+rowH)
	pdf.SetXY(x0, nextY)
}

func ensureSpace(pdf *fpdf.Fpdf, lineH float64) {
	_, pageH := pdf.GetPageSize()
	if pdf.GetY()+lineH > pageH-14 {
		pdf.AddPage()
	}
}
