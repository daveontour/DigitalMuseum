package facerecognizer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestWriteReadFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte(`{"faces":[{"bbox":{"x":0.1,"y":0.2,"w":0.3,"h":0.4},"confidence":0.99,"embedding":[1,2,3]}]}`)

	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	got, err := ReadFrame(&buf, MaxFrameBytes)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("round-trip mismatch:\n got=%s\nwant=%s", got, payload)
	}
}

func TestWriteReadFrameEmptyPayload(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, []byte{}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got, err := ReadFrame(&buf, MaxFrameBytes)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty payload, got %d bytes", len(got))
	}
}

func TestReadFrameRejectsOversizedLength(t *testing.T) {
	var buf bytes.Buffer
	// Claim a huge payload without actually providing the bytes — ReadFrame
	// must reject based on the length prefix alone, not attempt the read.
	if err := WriteFrame(&buf, make([]byte, 0)); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	// Overwrite the length prefix we just wrote with an oversized value.
	oversized := buf.Bytes()
	oversized[0], oversized[1], oversized[2], oversized[3] = 0xFF, 0xFF, 0xFF, 0x7F

	if _, err := ReadFrame(bytes.NewReader(oversized), MaxFrameBytes); err == nil {
		t.Fatal("want error for oversized frame length, got nil")
	}
}

func TestMultipleFramesSequentially(t *testing.T) {
	var buf bytes.Buffer
	payloads := [][]byte{
		[]byte("first"),
		[]byte("second-longer-payload"),
		[]byte(""),
		[]byte("fourth"),
	}
	for _, p := range payloads {
		if err := WriteFrame(&buf, p); err != nil {
			t.Fatalf("WriteFrame: %v", err)
		}
	}
	for i, want := range payloads {
		got, err := ReadFrame(&buf, MaxFrameBytes)
		if err != nil {
			t.Fatalf("ReadFrame[%d]: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("frame %d mismatch: got=%q want=%q", i, got, want)
		}
	}
}

// TestDetectedFaceJSONShape locks in the wire format cmd/facerecognizer must
// produce, since that binary doesn't exist yet in this codebase — this test
// is the executable spec for its JSON response shape.
func TestDetectedFaceJSONShape(t *testing.T) {
	raw := `{"faces":[{"bbox":{"x":0.1,"y":0.2,"w":0.3,"h":0.4},"confidence":0.987,"landmarks":[[0.15,0.25],[0.3,0.25],[0.22,0.32],[0.17,0.4],[0.28,0.4]],"embedding":[0.01,-0.02,0.03]}]}`

	var resp struct {
		Faces []DetectedFace `json:"faces"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Faces) != 1 {
		t.Fatalf("want 1 face, got %d", len(resp.Faces))
	}
	f := resp.Faces[0]
	if f.BBox != (BBox{X: 0.1, Y: 0.2, W: 0.3, H: 0.4}) {
		t.Errorf("unexpected bbox: %+v", f.BBox)
	}
	if f.Confidence != 0.987 {
		t.Errorf("unexpected confidence: %v", f.Confidence)
	}
	if len(f.Landmarks) != 5 {
		t.Errorf("want 5 landmarks, got %d", len(f.Landmarks))
	}
	if len(f.Embedding) != 3 {
		t.Errorf("want 3 embedding values in this fixture, got %d", len(f.Embedding))
	}
}

func TestClientStart_MissingBinaryReturnsErrNotAvailable(t *testing.T) {
	c := NewClient(`Z:\definitely\does\not\exist\facerecognizer.exe`)
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("want error for missing binary, got nil")
	}
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("want ErrNotAvailable, got: %v", err)
	}
}
