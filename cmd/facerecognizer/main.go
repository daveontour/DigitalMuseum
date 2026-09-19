// Command facerecognizer is the bundled local face-detection and
// face-recognition subprocess for Digital Museum (see
// internal/service/facerecognizer for the Go client and wire protocol this
// binary implements, and internal/database/migrate.go for how detected
// faces are stored).
//
// It runs two ONNX models via ONNX Runtime (github.com/yalue/onnxruntime_go,
// which dlopens a bundled onnxruntime.dll rather than requiring a static
// link):
//   - detection.onnx: SCRFD (buffalo_l's det_10g, the "bnkps" variant with
//     5-point landmarks) — finds face bounding boxes + landmarks.
//   - recognition.onnx: ArcFace (buffalo_l's w600k_r50, ResNet50@WebFace600K)
//     — turns a landmark-aligned 112x112 face crop into a 512-d embedding.
//
// Both model files' actual input/output tensor names and shapes were
// confirmed by loading them with onnxruntime_go's GetInputOutputInfo before
// writing the decode logic below, rather than assumed from documentation —
// see scrfd.go and recognize.go for the exact names this code depends on.
//
// Usage: facerecognizer --serve [--dir <bin/FaceRecognizer folder>]
//
// In --serve mode, the process loads both models once, then loops reading
// one length-prefixed image frame at a time from stdin (internal/service/facerecognizer.ReadFrame)
// and writing one length-prefixed JSON response frame to stdout
// (internal/service/facerecognizer.WriteFrame) per image, until stdin closes
// (the Go client's Close() closes its write end of the pipe to signal this).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"os"
	"path/filepath"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/daveontour/aimuseum/internal/service/facerecognizer"
)

func main() {
	serve := flag.Bool("serve", false, "run the persistent stdin/stdout serve loop")
	dir := flag.String("dir", "", "path to the bin/FaceRecognizer folder (models/ + onnxruntime.dll); defaults to the directory this executable is in")
	flag.Parse()

	baseDir := *dir
	if baseDir == "" {
		exe, err := os.Executable()
		if err != nil {
			log.Fatalf("facerecognizer: resolve executable path: %v", err)
		}
		baseDir = filepath.Dir(exe)
	}

	if !*serve {
		fmt.Fprintln(os.Stderr, "facerecognizer: pass --serve to run (this binary has no other mode)")
		os.Exit(2)
	}

	if err := run(baseDir); err != nil {
		log.Fatalf("facerecognizer: %v", err)
	}
}

func run(baseDir string) error {
	ort.SetSharedLibraryPath(filepath.Join(baseDir, "onnxruntime.dll"))
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("initialize onnxruntime: %w", err)
	}
	defer func() { _ = ort.DestroyEnvironment() }()

	detector, err := newDetector(filepath.Join(baseDir, "models", "detection.onnx"))
	if err != nil {
		return fmt.Errorf("load detection model: %w", err)
	}
	defer detector.Close()

	recognizer, err := newRecognizer(filepath.Join(baseDir, "models", "recognition.onnx"))
	if err != nil {
		return fmt.Errorf("load recognition model: %w", err)
	}
	defer recognizer.Close()

	log.Printf("facerecognizer: models loaded, serving on stdin/stdout")
	return serveLoop(os.Stdin, os.Stdout, detector, recognizer)
}

func serveLoop(r io.Reader, w io.Writer, detector *detector, recognizer *recognizer) error {
	for {
		imgBytes, err := facerecognizer.ReadFrame(r, facerecognizer.MaxFrameBytes)
		if err == io.EOF {
			return nil // client closed stdin: normal shutdown
		}
		if err != nil {
			return fmt.Errorf("read request frame: %w", err)
		}

		faces, procErr := processImage(imgBytes, detector, recognizer)

		var resp struct {
			Faces []facerecognizer.DetectedFace `json:"faces"`
			Error string                        `json:"error,omitempty"`
		}
		if procErr != nil {
			resp.Error = procErr.Error()
			log.Printf("facerecognizer: error processing image: %v", procErr)
		} else {
			resp.Faces = faces
		}

		respBytes, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
		if err := facerecognizer.WriteFrame(w, respBytes); err != nil {
			return fmt.Errorf("write response frame: %w", err)
		}
	}
}

// processImage runs the full pipeline for one image: decode, detect faces,
// align + embed each one. A single malformed face's alignment/recognition
// failure does not fail the whole image — that face is just skipped, so one
// bad detection doesn't lose the others.
func processImage(imgBytes []byte, det *detector, rec *recognizer) ([]facerecognizer.DetectedFace, error) {
	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	detections, err := det.Detect(img)
	if err != nil {
		return nil, fmt.Errorf("detect: %w", err)
	}

	b := img.Bounds()
	origW, origH := float64(b.Dx()), float64(b.Dy())

	out := make([]facerecognizer.DetectedFace, 0, len(detections))
	for _, d := range detections {
		aligned := alignFace(img, d.Landmarks)
		embedding, err := rec.Embed(aligned)
		if err != nil {
			log.Printf("facerecognizer: skipping one face (alignment/recognition failed): %v", err)
			continue
		}

		landmarksFrac := make([][2]float64, 5)
		for i, p := range d.Landmarks {
			landmarksFrac[i] = [2]float64{p[0] / origW, p[1] / origH}
		}

		out = append(out, facerecognizer.DetectedFace{
			BBox: facerecognizer.BBox{
				X: d.X1 / origW,
				Y: d.Y1 / origH,
				W: (d.X2 - d.X1) / origW,
				H: (d.Y2 - d.Y1) / origH,
			},
			Confidence: d.Score,
			Landmarks:  landmarksFrac,
			Embedding:  embedding,
		})
	}
	return out, nil
}
