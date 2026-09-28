package chatbotdocs

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// extractDocxText reads plain text from a .docx file's bytes using only the standard library —
// a .docx is a zip archive containing word/document.xml, which is walked here to pull out every
// <w:t> run's text, joined with a newline at each </w:p> (paragraph) close.
func extractDocxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("could not read .docx (not a valid zip archive): %w", err)
	}
	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmt.Errorf("could not find word/document.xml inside .docx")
	}
	rc, err := docFile.Open()
	if err != nil {
		return "", fmt.Errorf("could not open word/document.xml: %w", err)
	}
	defer func() { _ = rc.Close() }()

	var b strings.Builder
	dec := xml.NewDecoder(rc)
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("could not parse word/document.xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				b.WriteString("\n")
			case "tab":
				b.WriteString("\t")
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}
