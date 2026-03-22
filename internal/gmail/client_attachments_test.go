package gmail

import (
	"testing"

	gmailv1 "google.golang.org/api/gmail/v1"
)

func hdr(name, value string) *gmailv1.MessagePartHeader {
	return &gmailv1.MessagePartHeader{Name: name, Value: value}
}

func TestShouldSaveGmailPartAsAttachment(t *testing.T) {
	tests := []struct {
		name string
		part *gmailv1.MessagePart
		want bool
	}{
		{
			name: "nil part",
			part: nil,
			want: false,
		},
		{
			name: "multipart mixed skipped",
			part: &gmailv1.MessagePart{MimeType: "multipart/mixed"},
			want: false,
		},
		{
			name: "inline image skipped",
			part: &gmailv1.MessagePart{
				MimeType: "image/png",
				Headers: []*gmailv1.MessagePartHeader{
					hdr("Content-Disposition", `inline; filename="cid.png"`),
				},
			},
			want: false,
		},
		{
			name: "attachment disposition",
			part: &gmailv1.MessagePart{
				MimeType: "application/pdf",
				Headers: []*gmailv1.MessagePartHeader{
					hdr("Content-Disposition", `attachment; filename="doc.pdf"`),
				},
			},
			want: true,
		},
		{
			name: "plain text as attachment disposition still true",
			part: &gmailv1.MessagePart{
				MimeType: "text/plain",
				Headers: []*gmailv1.MessagePartHeader{
					hdr("Content-Disposition", `attachment; filename="notes.txt"`),
				},
			},
			want: true,
		},
		{
			name: "body text plain no disposition skipped",
			part: &gmailv1.MessagePart{MimeType: "text/plain"},
			want: false,
		},
		{
			name: "body text html no disposition skipped",
			part: &gmailv1.MessagePart{MimeType: "text/html"},
			want: false,
		},
		{
			name: "pdf with Filename field",
			part: &gmailv1.MessagePart{
				MimeType: "application/pdf",
				Filename: "report.pdf",
			},
			want: true,
		},
		{
			name: "binary without filename but non text type",
			part: &gmailv1.MessagePart{MimeType: "application/octet-stream"},
			want: true,
		},
		{
			name: "message rfc822 container not leaf rule",
			part: &gmailv1.MessagePart{MimeType: "message/rfc822"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldSaveGmailPartAsAttachment(tt.part)
			if got != tt.want {
				t.Errorf("ShouldSaveGmailPartAsAttachment() = %v, want %v", got, tt.want)
			}
		})
	}
}
