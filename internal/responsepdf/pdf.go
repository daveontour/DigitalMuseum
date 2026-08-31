// Package responsepdf renders a single saved chat response as a downloadable PDF.
package responsepdf

import (
	"bytes"
	"regexp"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// Meta describes the saved response to export.
type Meta struct {
	Title       string
	Content     string
	Voice       *string
	LLMProvider *string
	CreatedAt   time.Time
}

// Render builds PDF bytes for one saved response. Content is treated as lightly
// formatted Markdown (headers, bullet/numbered lists, code fences, bold/italic/inline
// code markers) — not a full CommonMark renderer, but enough to produce a clean,
// readable document without dragging in a Markdown dependency.
func Render(meta Meta) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(18, 18, 18)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()
	pdf.SetTitle(sanitize(meta.Title), true)

	// Core Helvetica/Courier fonts are Latin-1 based; translate UTF-8 page content
	// through the cp1252 approximation so accented characters and smart quotes
	// still render instead of embedding a custom TTF just for this export.
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	pdf.SetFont("Helvetica", "B", 16)
	pdf.MultiCell(0, 8, tr(sanitize(meta.Title)), "", "L", false)
	pdf.Ln(2)

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(90, 90, 90)
	metaParts := []string{"Saved: " + meta.CreatedAt.Format("2 Jan 2006, 15:04")}
	if v := strings.TrimSpace(derefStr(meta.Voice)); v != "" {
		metaParts = append(metaParts, "Voice: "+v)
	}
	if p := strings.TrimSpace(derefStr(meta.LLMProvider)); p != "" {
		metaParts = append(metaParts, "AI model: "+p)
	}
	pdf.MultiCell(0, 5, tr(sanitize(strings.Join(metaParts, "   ·   "))), "", "L", false)
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(4)

	renderMarkdownBody(pdf, tr, meta.Content)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

var (
	reHeader   = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reBullet   = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
	reOrdered  = regexp.MustCompile(`^\s*(\d+)\.\s+(.*)$`)
	reInlineMD = regexp.MustCompile("\\*\\*(.+?)\\*\\*|__(.+?)__|\\*(.+?)\\*|_(.+?)_|`(.+?)`")
)

// stripInlineMarkdown removes bold/italic/inline-code markers, keeping the inner text —
// fpdf's Cell/MultiCell can't mix styles within one call, so this favours clean plain
// text over dangling asterisks/backticks rather than attempting inline style runs.
func stripInlineMarkdown(s string) string {
	return reInlineMD.ReplaceAllStringFunc(s, func(m string) string {
		sub := reInlineMD.FindStringSubmatch(m)
		for _, g := range sub[1:] {
			if g != "" {
				return g
			}
		}
		return m
	})
}

func renderMarkdownBody(pdf *fpdf.Fpdf, tr func(string) string, content string) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	inCodeBlock := false
	pdf.SetFont("Helvetica", "", 11)
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.HasPrefix(strings.TrimSpace(trimmed), "```") {
			inCodeBlock = !inCodeBlock
			if inCodeBlock {
				pdf.SetFont("Courier", "", 9)
			} else {
				pdf.SetFont("Helvetica", "", 11)
			}
			continue
		}
		if inCodeBlock {
			pdf.MultiCell(0, 5, tr(sanitize(line)), "", "L", false)
			continue
		}
		if strings.TrimSpace(trimmed) == "" {
			pdf.Ln(3)
			continue
		}
		if m := reHeader.FindStringSubmatch(trimmed); m != nil {
			level := len(m[1])
			size := 14.0 - float64(level)*1.2
			if size < 10 {
				size = 10
			}
			pdf.Ln(2)
			pdf.SetFont("Helvetica", "B", size)
			pdf.MultiCell(0, size/1.7, tr(stripInlineMarkdown(sanitize(m[2]))), "", "L", false)
			pdf.SetFont("Helvetica", "", 11)
			pdf.Ln(1)
			continue
		}
		if m := reBullet.FindStringSubmatch(trimmed); m != nil {
			pdf.MultiCell(0, 5.5, tr("   •  "+stripInlineMarkdown(sanitize(m[1]))), "", "L", false)
			continue
		}
		if m := reOrdered.FindStringSubmatch(trimmed); m != nil {
			pdf.MultiCell(0, 5.5, tr("   "+m[1]+".  "+stripInlineMarkdown(sanitize(m[2]))), "", "L", false)
			continue
		}
		pdf.MultiCell(0, 5.5, tr(stripInlineMarkdown(sanitize(trimmed))), "", "L", false)
	}
}

func sanitize(s string) string {
	s = strings.ToValidUTF8(s, "")
	if len(s) > 20000 {
		s = s[:20000] + "..."
	}
	return s
}
