// Package importer provides RunSubprocess for imports that still run as a separate
// import-processor binary (contacts). WhatsApp, iMessage, Facebook Messenger,
// Instagram, Facebook All (Messenger+Albums+Places+Posts), filesystem images,
// and thumbnails run in-process instead.
package importer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// splitOnCROrLF is a bufio.SplitFunc that yields a token on each \r or \n.
// This allows progress lines that use \r for in-place updates to be forwarded to SSE clients.
func splitOnCROrLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// FindBinary locates the import-processor binary.
// It checks IMPORT_PROCESSOR_PATH env var first, then looks for the binary
// in the ../import-processor/ directory relative to the current working directory.
func FindBinary() (string, error) {
	if env := os.Getenv("IMPORT_PROCESSOR_PATH"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up from cwd looking for import-processor dir
	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "import-processor")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			binaryName := "import-processor"
			if runtime.GOOS == "windows" {
				binaryName = "import-processor.exe"
			}
			binary := filepath.Join(candidate, binaryName)
			if _, err := os.Stat(binary); err == nil {
				return binary, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", fmt.Errorf("import-processor binary not found; set IMPORT_PROCESSOR_PATH or build it in the import-processor/ directory")
}

// RunSubprocess runs the import-processor binary with the given args.
// It streams stderr lines to the job's SSE channel and returns stdout when done.
// cancel is watched during stderr streaming.
func RunSubprocess(job *ImportJob, args []string) (stdout string, returnCode int, err error) {
	binary, err := FindBinary()
	if err != nil {
		return "", -1, err
	}

	fullArgs := append([]string(nil), args...)
	cmd := exec.Command(binary, fullArgs...)
	cmd.Dir = filepath.Dir(binary)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", -1, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", -1, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", -1, fmt.Errorf("start: %w", err)
	}

	// Heartbeat: re-broadcast current state every second so clients receive
	// updates even when subprocess stderr is buffered (e.g. pipe, not TTY).
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ticker.C:
				if job.InProgress() && !job.IsCancelled() {
					job.Broadcast("progress", job.GetState())
				}
			}
		}
	}()

	var wg sync.WaitGroup
	var stdoutBuf []byte

	// Read stdout in background
	wg.Add(1)
	go func() {
		defer wg.Done()
		stdoutBuf, _ = io.ReadAll(stdoutPipe)
	}()

	// Read stderr line-by-line, broadcasting each line.
	// Import-processor uses \r (carriage return) for in-place progress updates;
	// we must split on both \r and \n so the client receives intermediate updates.
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Split(splitOnCROrLF)
		for scanner.Scan() {
			if job.IsCancelled() {
				_ = cmd.Process.Kill()
				return
			}
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				job.UpdateState(map[string]any{"status_line": line})
				job.Broadcast("progress", job.GetState())
			}
		}
	}()

	wg.Wait()
	close(heartbeatDone)
	_ = cmd.Wait()
	returnCode = cmd.ProcessState.ExitCode()
	return string(stdoutBuf), returnCode, nil
}
