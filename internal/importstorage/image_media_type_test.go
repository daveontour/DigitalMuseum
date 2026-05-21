package importstorage

import "testing"

func TestNormalizeFilesystemUploadMediaType(t *testing.T) {
	heicFTYP := []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0}

	tests := []struct {
		name     string
		ref      string
		header   string
		data     []byte
		wantMIME string
	}{
		{"heic extension octet", "Photos/IMG.heic", "application/octet-stream", []byte{1}, "image/heic"},
		{"heif extension octet", "x.heif", "application/octet-stream", []byte{1}, "image/heif"},
		{"preserve image/jpeg", "x.jpg", "image/jpeg", nil, "image/jpeg"},
		{"sniff heic from ftyp", "unknown.bin", "application/octet-stream", heicFTYP, "image/heic"},
		{"empty data no ext", "file", "application/octet-stream", nil, "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeFilesystemUploadMediaType(tt.ref, tt.header, tt.data)
			if got != tt.wantMIME {
				t.Fatalf("got %q, want %q", got, tt.wantMIME)
			}
		})
	}
}
