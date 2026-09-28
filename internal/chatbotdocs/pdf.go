package chatbotdocs

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ledongthuc/pdf"
)

// extractPDFText reads plain text from a PDF's bytes using a pure-Go reader (no CGO, no
// external process). A scanned/image-only PDF legitimately yields empty text — that's returned
// as a normal (possibly empty) result, not an error; only a structurally unreadable PDF errors.
func extractPDFText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("could not read PDF: %w", err)
	}
	textReader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("could not extract PDF text: %w", err)
	}
	b, err := io.ReadAll(textReader)
	if err != nil {
		return "", fmt.Errorf("could not read extracted PDF text: %w", err)
	}
	return string(b), nil
}
