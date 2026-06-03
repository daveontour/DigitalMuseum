package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/daveontour/aimuseum/internal/filedupe/paths"
)

var (
	imageExtensions = map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
		".bmp":  "image/bmp",
		".heic": "image/jpeg",
		".heif": "image/jpeg",
	}

	heicExtensions = map[string]struct{}{
		".heic": {},
		".heif": {},
	}
)

type Result struct {
	Reader      io.ReadCloser
	ContentType string
	Cleanup     func()
}

func Open(allowedRoots []string, sourcePath string) (Result, error) {
	roots := paths.CleanRoots(allowedRoots)
	if len(roots) == 0 {
		return Result{}, fmt.Errorf("at least one allowed directory root is required")
	}

	sourcePath = filepath.Clean(strings.TrimSpace(sourcePath))
	if sourcePath == "" {
		return Result{}, fmt.Errorf("path is required")
	}

	if err := paths.ValidateUnderRoots(sourcePath, roots); err != nil {
		return Result{}, err
	}

	ext := strings.ToLower(filepath.Ext(sourcePath))
	contentType, ok := imageExtensions[ext]
	if !ok {
		return Result{}, fmt.Errorf("unsupported image type %q", ext)
	}

	if _, isHeic := heicExtensions[ext]; isHeic {
		return openConvertedHEIC(sourcePath, contentType)
	}

	file, err := os.Open(sourcePath)
	if err != nil {
		return Result{}, fmt.Errorf("open image: %w", err)
	}

	return Result{
		Reader:      file,
		ContentType: contentType,
		Cleanup:     func() { _ = file.Close() },
	}, nil
}

func IsImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	_, ok := imageExtensions[ext]
	return ok
}

func openConvertedHEIC(sourcePath, contentType string) (Result, error) {
	magick, err := magickExecutable()
	if err != nil {
		return Result{}, err
	}

	cachePath, err := heicCachePath(sourcePath)
	if err != nil {
		return Result{}, err
	}

	if err := ensureHEICConverted(magick, sourcePath, cachePath); err != nil {
		return Result{}, err
	}

	file, err := os.Open(cachePath)
	if err != nil {
		return Result{}, fmt.Errorf("open converted preview: %w", err)
	}

	return Result{
		Reader:      file,
		ContentType: contentType,
		Cleanup:     func() { _ = file.Close() },
	}, nil
}

func magickExecutable() (string, error) {
	candidates := []string{
		filepath.Join("bin", "ImageMagick", "magick.exe"),
		filepath.Join("bin", "ImageMagick", "magick"),
	}

	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(base, "bin", "ImageMagick", "magick.exe"),
			filepath.Join(base, "bin", "ImageMagick", "magick"),
		)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate, nil
			}
			return abs, nil
		}
	}

	return "", fmt.Errorf("ImageMagick not found in bin/ImageMagick")
}

func heicCachePath(sourcePath string) (string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("stat source image: %w", err)
	}

	hash := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", sourcePath, info.ModTime().UnixNano(), info.Size())))
	name := hex.EncodeToString(hash[:]) + ".jpg"

	cacheDir := filepath.Join(os.TempDir(), "digitalmuseum-filedupe-preview")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create preview cache: %w", err)
	}

	return filepath.Join(cacheDir, name), nil
}

func ensureHEICConverted(magick, sourcePath, cachePath string) error {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("stat source image: %w", err)
	}

	if cacheInfo, err := os.Stat(cachePath); err == nil {
		if cacheInfo.ModTime().After(sourceInfo.ModTime()) {
			return nil
		}
	}

	tempPath := strings.TrimSuffix(cachePath, filepath.Ext(cachePath)) + ".tmp.jpg"
	_ = os.Remove(tempPath)

	cmd := exec.Command(magick, sourcePath+"[0]", tempPath)
	cmd.Dir = filepath.Dir(magick)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("convert HEIC preview: %w: %s", err, strings.TrimSpace(string(output)))
	}

	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("save converted preview: %w", err)
	}

	return nil
}
