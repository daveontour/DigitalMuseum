package facerecognizer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// repoBundledPath resolves bin/FaceRecognizer/<file> relative to the repo
// root, not the current package directory (go test always runs with the
// package directory as its working directory).
func repoBundledPath(elem ...string) string {
	parts := append([]string{"..", "..", "..", "bin", "FaceRecognizer"}, elem...)
	return filepath.Join(parts...)
}

// TestClient_RealSubprocess_EndToEnd exercises the actual bundled
// facerecognizer.exe (built from cmd/facerecognizer) and its onnxruntime.dll
// + ONNX models against a real image, through the exact same Client this
// package ships for production use — not a mock. It's skipped automatically
// when those files aren't staged (a fresh checkout, or CI, won't have the
// gitignored bin/FaceRecognizer/ contents), so it only runs as a genuine
// integration check in a dev environment that has sourced the model files.
func TestClient_RealSubprocess_EndToEnd(t *testing.T) {
	exePath := repoBundledPath("facerecognizer.exe")
	if _, err := os.Stat(exePath); err != nil {
		t.Skipf("bin/FaceRecognizer/facerecognizer.exe not staged, skipping: %v", err)
	}
	if _, err := os.Stat(repoBundledPath("onnxruntime.dll")); err != nil {
		t.Skipf("bin/FaceRecognizer/onnxruntime.dll not staged, skipping: %v", err)
	}

	// A real photo-ish image with a face-like subject already checked into
	// the repo (a voice persona avatar), so this test needs no external
	// downloads or fixtures of its own.
	imgPath := filepath.Join("..", "..", "..", "static", "images", "female-friend.png")
	imgBytes, err := os.ReadFile(imgPath)
	if err != nil {
		t.Fatalf("read test image: %v", err)
	}

	client := NewClient(exePath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("Close: %v", err)
		}
	}()

	faces, err := client.Detect(imgBytes)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	t.Logf("detected %d face(s)", len(faces))
	for i, f := range faces {
		t.Logf("face %d: bbox=%+v confidence=%.4f landmarks=%d embedding_len=%d",
			i, f.BBox, f.Confidence, len(f.Landmarks), len(f.Embedding))

		if f.Confidence < 0 || f.Confidence > 1 {
			t.Errorf("face %d: confidence %.4f out of [0,1] range", i, f.Confidence)
		}
		if f.BBox.X < 0 || f.BBox.Y < 0 || f.BBox.X+f.BBox.W > 1.0001 || f.BBox.Y+f.BBox.H > 1.0001 {
			t.Errorf("face %d: bbox %+v not within normalized [0,1] image bounds", i, f.BBox)
		}
		if f.BBox.W <= 0 || f.BBox.H <= 0 {
			t.Errorf("face %d: non-positive bbox size %+v", i, f.BBox)
		}
		if len(f.Embedding) != 512 {
			t.Errorf("face %d: want 512-d embedding, got %d", i, len(f.Embedding))
		}
		if len(f.Landmarks) != 5 {
			t.Errorf("face %d: want 5 landmarks, got %d", i, len(f.Landmarks))
		}
		for _, lm := range f.Landmarks {
			if lm[0] < -0.5 || lm[0] > 1.5 || lm[1] < -0.5 || lm[1] > 1.5 {
				t.Errorf("face %d: landmark %v far outside image bounds", i, lm)
			}
		}
	}

	// Run a second image through the same still-running subprocess, to
	// confirm the --serve loop correctly handles more than one request per
	// process lifetime (the whole point of the persistent-process protocol).
	imgPath2 := filepath.Join("..", "..", "..", "static", "images", "bar-girl.png")
	imgBytes2, err := os.ReadFile(imgPath2)
	if err != nil {
		t.Fatalf("read second test image: %v", err)
	}
	faces2, err := client.Detect(imgBytes2)
	if err != nil {
		t.Fatalf("Detect (second image, same subprocess): %v", err)
	}
	t.Logf("second image: detected %d face(s)", len(faces2))
}
