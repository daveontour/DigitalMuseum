// Package chatbotdocs extracts plain text from ChatBot document attachments (pdf, txt, csv, md,
// docx) so it can be folded into the LLM's system prompt. Deliberately CGO-free — see
// internal/service/chatbot_service.go and CLAUDE.md's ChatBot feature notes for how the result is
// used. Image attachments are not handled here; they're forwarded to the LLM as multimodal
// content instead (internal/ai/openrouter.go).
package chatbotdocs

import (
	"strings"
)

// Kind values an attachment can be classified as.
const (
	KindImage    = "image"
	KindDocument = "document"
)

// maxExtractedChars caps how much text a single document contributes, so one huge upload can't
// blow out the system prompt's context budget. Mirrors the per-document cap
// ChatBotService applies when weaving extracted text into the prompt.
const maxExtractedChars = 200_000

// Extract classifies an uploaded file by contentType/filename and, for document kinds, extracts
// its plain text. extractedText is nil for images (kind == KindImage) and for a document kind
// whose extraction failed (extractionErr is set instead, not a hard error — the attachment row
// is still created so the user sees why nothing could be read).
func Extract(contentType, filename string, data []byte) (kind string, extractedText, extractionErr *string) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	ext := strings.ToLower(extOf(filename))

	if strings.HasPrefix(ct, "image/") {
		return KindImage, nil, nil
	}

	switch {
	case ct == "application/pdf" || ext == ".pdf":
		text, err := extractPDFText(data)
		if err != nil {
			msg := err.Error()
			return KindDocument, nil, &msg
		}
		text = truncate(text)
		return KindDocument, &text, nil

	case isDocxContentType(ct) || ext == ".docx":
		text, err := extractDocxText(data)
		if err != nil {
			msg := err.Error()
			return KindDocument, nil, &msg
		}
		text = truncate(text)
		return KindDocument, &text, nil

	case ext == ".doc":
		msg := "legacy .doc format is not supported — please save as .docx and re-attach"
		return KindDocument, nil, &msg

	case strings.HasPrefix(ct, "text/") || ext == ".txt" || ext == ".csv" || ext == ".md":
		text := truncate(string(data))
		return KindDocument, &text, nil

	default:
		msg := "unsupported file type for text extraction"
		return KindDocument, nil, &msg
	}
}

// IsSupportedUpload reports whether contentType/filename is something Extract knows how to
// classify — used by the upload handler to reject anything else (including images, which the
// handler routes to Extract too, but only after this check passes) at upload time, before an
// attachment row is ever created for a type nobody can use.
func IsSupportedUpload(contentType, filename string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	ext := strings.ToLower(extOf(filename))
	if strings.HasPrefix(ct, "image/") {
		return true
	}
	if ct == "application/pdf" || ext == ".pdf" {
		return true
	}
	if isDocxContentType(ct) || ext == ".docx" {
		return true
	}
	if strings.HasPrefix(ct, "text/") || ext == ".txt" || ext == ".csv" || ext == ".md" {
		return true
	}
	return false
}

func isDocxContentType(ct string) bool {
	return ct == "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

func extOf(filename string) string {
	i := strings.LastIndex(filename, ".")
	if i < 0 {
		return ""
	}
	return filename[i:]
}

func truncate(s string) string {
	if len(s) <= maxExtractedChars {
		return s
	}
	return s[:maxExtractedChars] + "\n\n[... truncated, document too large ...]"
}
