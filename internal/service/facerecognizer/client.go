// Package facerecognizer is the Go client for the bundled facerecognizer
// subprocess (cmd/facerecognizer, not yet implemented — this package only
// defines and tests the protocol/client side so the rest of the face
// recognition pipeline can be built and tested against it).
//
// Unlike the spawn-per-call ImageMagick/exiftool pattern used elsewhere in
// this codebase (see internal/import/thumbnails/processor.go), the
// facerecognizer binary loads ONNX models at startup, which is not free like
// ImageMagick's near-instant startup. So the protocol here is a persistent
// process started once per background-job batch ("facerecognizer.exe
// --serve") and reused for every image in that batch: the Go side writes one
// length-prefixed frame per image (4-byte little-endian length + raw image
// bytes) to the subprocess's stdin, and reads one length-prefixed JSON frame
// back from its stdout per image.
package facerecognizer

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// DefaultBundledPath returns the conventional relative path to the bundled
// facerecognizer executable, matching how bin/ImageMagick and bin/exiftool
// are resolved elsewhere (relative to the process's working directory, which
// Electron sets to the project root in dev or the install root when
// packaged). The file need not exist — Start reports ErrNotAvailable if it
// doesn't.
func DefaultBundledPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("bin", "FaceRecognizer", "facerecognizer.exe")
	}
	return filepath.Join("bin", "FaceRecognizer", "facerecognizer")
}

// ErrNotAvailable is returned by NewClient/Start when the bundled binary is
// missing (e.g. during development, before the face-recognition model files
// and cmd/facerecognizer executable have been staged under bin/FaceRecognizer/).
// Callers should treat this as "face recognition isn't set up yet", not a
// hard failure — jobs report it as a clear status message.
var ErrNotAvailable = errors.New("facerecognizer: bundled executable not found")

// MaxFrameBytes bounds both directions of the framed protocol against a
// corrupt/malicious length prefix causing an unbounded allocation. 64MiB
// comfortably covers a full-resolution photo in one direction and a JSON
// response with several faces' 512-float embeddings in the other.
const MaxFrameBytes = 64 * 1024 * 1024

// BBox is a face bounding box as fractions (0..1) of the source image's
// width/height, so it's resolution-independent.
type BBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// DetectedFace is one face found in an image by the subprocess.
type DetectedFace struct {
	BBox       BBox         `json:"bbox"`
	Confidence float64      `json:"confidence"`
	Landmarks  [][2]float64 `json:"landmarks,omitempty"`
	Embedding  []float32    `json:"embedding"`
}

// Client manages one long-lived facerecognizer subprocess and serializes
// Detect calls into it (the subprocess is not safe for concurrent stdin
// writes — callers fanning out across multiple goroutines must share one
// Client, not spawn one each).
type Client struct {
	exePath string

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

// NewClient creates a client for the facerecognizer binary at exePath. It
// does not start the subprocess — call Start.
func NewClient(exePath string) *Client {
	return &Client{exePath: exePath}
}

// Start spawns "exePath --serve" and connects its stdin/stdout. Returns
// ErrNotAvailable (wrapped) if exePath does not exist, so callers can
// distinguish "not set up yet" from a real launch failure.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil {
		return fmt.Errorf("facerecognizer: already started")
	}
	if _, err := os.Stat(c.exePath); err != nil {
		return fmt.Errorf("%w: %s", ErrNotAvailable, c.exePath)
	}

	cmd := exec.CommandContext(ctx, c.exePath, "--serve")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("facerecognizer: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("facerecognizer: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("facerecognizer: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("facerecognizer: start: %w", err)
	}

	go logSubprocessStderr(stderr)

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = bufio.NewReader(stdout)
	return nil
}

// logSubprocessStderr forwards the subprocess's stderr to slog, line by
// line, for diagnostics (model load errors, ONNX Runtime warnings, etc.).
func logSubprocessStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// Model-load or ONNX error messages can be long; raise the default 64KiB scan limit.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		slog.Warn("facerecognizer", "stderr", scanner.Text())
	}
}

// Detect sends one image to the subprocess and returns its detected faces.
// Safe to call from multiple goroutines — calls are serialized internally.
func (c *Client) Detect(imageBytes []byte) ([]DetectedFace, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stdin == nil || c.stdout == nil {
		return nil, fmt.Errorf("facerecognizer: client not started")
	}

	if err := WriteFrame(c.stdin, imageBytes); err != nil {
		return nil, fmt.Errorf("facerecognizer: write request frame: %w", err)
	}

	respBytes, err := ReadFrame(c.stdout, MaxFrameBytes)
	if err != nil {
		return nil, fmt.Errorf("facerecognizer: read response frame: %w", err)
	}

	var resp struct {
		Faces []DetectedFace `json:"faces"`
		Error string         `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("facerecognizer: decode response: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("facerecognizer: %s", resp.Error)
	}
	return resp.Faces, nil
}

// Close closes stdin (signaling the subprocess to exit its serve loop) and
// waits for it to exit. Safe to call even if Start failed or was never called.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd == nil {
		return nil
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	err := c.cmd.Wait()
	c.cmd = nil
	c.stdin = nil
	c.stdout = nil
	if err != nil {
		return fmt.Errorf("facerecognizer: subprocess exit: %w", err)
	}
	return nil
}

// WriteFrame writes a 4-byte little-endian length prefix followed by payload.
func WriteFrame(w io.Writer, payload []byte) error {
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame reads a 4-byte little-endian length prefix followed by that many
// bytes. Returns an error if the declared length exceeds maxSize, guarding
// against a corrupt length prefix causing an unbounded allocation.
func ReadFrame(r io.Reader, maxSize int) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(lenBuf[:])
	if int(n) > maxSize {
		return nil, fmt.Errorf("frame length %d exceeds max %d", n, maxSize)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
