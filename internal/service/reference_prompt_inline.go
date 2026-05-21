package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/daveontour/aimuseum/internal/appctx"
	appcrypto "github.com/daveontour/aimuseum/internal/crypto"
)

func refDocDisplayTitle(title *string, filename string) string {
	if title != nil && strings.TrimSpace(*title) != "" {
		return strings.TrimSpace(*title)
	}
	return strings.TrimSpace(filename)
}

// appendInlinedReferenceDocumentsToSystemPrompt appends decrypted/plain text from
// reference_documents rows with include_in_system_prompt set.
//
// Visitor-key restricted sessions skip entirely (aligned with reference-document listing tools).
func (s *ChatService) appendInlinedReferenceDocumentsToSystemPrompt(ctx context.Context, r *http.Request, base string) string {
	if s.docRepo == nil {
		return base
	}
	if appctx.VisitorAccessFromCtx(ctx).Restricted {
		return base
	}
	rows, err := s.docRepo.ListForSystemPromptInclusion(ctx)
	if err != nil || len(rows) == 0 {
		return base
	}
	masterPassword, haveMaster := "", false
	if s.sessionStore != nil && r != nil {
		masterPassword, haveMaster = s.sessionStore.Get(r)
	}

	var sb strings.Builder
	sb.WriteString(base)
	sb.WriteString("\n\n---\n\n**Reference documents (always included in this session):**\n")
	sb.WriteString("The following material is part of your context; you do not need to call tools to retrieve it.\n\n")

	for _, row := range rows {
		title := refDocDisplayTitle(row.Title, row.Filename)
		data := row.Data
		if row.IsEncrypted {
			if !haveMaster || masterPassword == "" {
				sb.WriteString(fmt.Sprintf("### %s\n\n[Encrypted reference document — master key not unlocked in this browser session.]\n\n", title))
				continue
			}
			plain, err := appcrypto.DecryptDocumentData(ctx, s.pool, masterPassword, data, s.pepper)
			if err != nil || len(plain) == 0 {
				sb.WriteString(fmt.Sprintf("### %s\n\n[Encrypted reference document — decryption failed.]\n\n", title))
				continue
			}
			data = plain
		}
		ct := strings.TrimSpace(row.ContentType)
		if ct == "application/pdf" {
			sb.WriteString(fmt.Sprintf("### %s\n\n[PDF document — not renderable as text.]\n\n", title))
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", title, string(data)))
	}
	return sb.String()
}
