package imagetagsync

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var ErrNothingToWrite = errors.New("no metadata to write")

func defaultBundledExifToolRel() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("bin", "exiftool", "exiftool.exe")
	}
	return filepath.Join("bin", "exiftool", "exiftool")
}

// ResolveExifTool returns the exiftool executable path (explicit, bundled, or next to os.Executable).
func ResolveExifTool(explicit string) (string, error) {
	if explicit != "" {
		if st, err := os.Stat(explicit); err != nil || st.IsDir() {
			return "", fmt.Errorf("exiftool not found at %q", explicit)
		}
		return explicit, nil
	}

	seen := make(map[string]struct{})
	var candidates []string
	add := func(p string) {
		if p == "" {
			return
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		candidates = append(candidates, p)
	}

	add(defaultBundledExifToolRel())
	if exe, err := os.Executable(); err == nil {
		root := filepath.Dir(exe)
		add(filepath.Join(root, defaultBundledExifToolRel()))
	}

	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("bundled exiftool not found (expected %s); place ExifTool there or pass -exiftool", defaultBundledExifToolRel())
}

// ExportMetadata is metadata to embed in one exported image file.
type ExportMetadata struct {
	FilePath  string
	Source    *string
	Latitude  *float64
	Longitude *float64
	HasGPS    bool
	Tags      *string
	CreatedAt *time.Time
}

// TagsForExport returns comma-split tags plus source (deduped, case-insensitive).
func TagsForExport(tags, source *string) []string {
	out := splitTags(tags)
	if source != nil {
		s := strings.TrimSpace(*source)
		if s != "" {
			out = appendUniqueTag(out, s)
		}
	}
	return out
}

func splitTags(raw *string) []string {
	if raw == nil {
		return nil
	}
	parts := strings.Split(*raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = appendUniqueTag(out, p)
		}
	}
	return out
}

func appendUniqueTag(tags []string, tag string) []string {
	lower := strings.ToLower(tag)
	for _, existing := range tags {
		if strings.ToLower(existing) == lower {
			return tags
		}
	}
	return append(tags, tag)
}

func buildWriteArgs(meta ExportMetadata) ([]string, bool) {
	args := []string{"-overwrite_original", "-P"}

	if meta.Source != nil {
		if s := strings.TrimSpace(*meta.Source); s != "" {
			args = append(args, "-IPTC:Source="+s)
		}
	}

	tagList := TagsForExport(meta.Tags, meta.Source)
	if len(tagList) > 0 {
		args = append(args, "-IPTC:Keywords=", "-XMP-dc:Subject=")
		for _, tag := range tagList {
			args = append(args, "-IPTC:Keywords="+tag, "-XMP-dc:Subject="+tag)
		}
	}

	if meta.HasGPS && meta.Latitude != nil && meta.Longitude != nil {
		args = append(args,
			"-GPSLatitude="+formatGPSCoord(*meta.Latitude),
			"-GPSLongitude="+formatGPSCoord(*meta.Longitude),
		)
	}

	if meta.CreatedAt != nil {
		args = append(args, "-DateTimeOriginal="+meta.CreatedAt.UTC().Format("2006:01:02 15:04:05"))
	}

	if len(args) == 2 {
		return nil, true
	}
	args = append(args, meta.FilePath)
	return args, false
}

func formatGPSCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// WriteExportMetadata writes tags, source, GPS, and created_at onto one exported image file.
func WriteExportMetadata(exifTool string, meta ExportMetadata) error {
	args, skip := buildWriteArgs(meta)
	if skip {
		return ErrNothingToWrite
	}
	return runExifTool(exifTool, args)
}

// WriteTags runs exiftool to replace IPTC/XMP keyword tags on one image file.
func WriteTags(exifTool string, imagePath string, tags []string) error {
	if len(tags) == 0 {
		return fmt.Errorf("no tags to write")
	}
	args := []string{
		"-overwrite_original",
		"-P",
		"-IPTC:Keywords=",
		"-XMP-dc:Subject=",
	}
	for _, tag := range tags {
		args = append(args, "-IPTC:Keywords="+tag, "-XMP-dc:Subject="+tag)
	}
	args = append(args, imagePath)
	return runExifTool(exifTool, args)
}

func runExifTool(exifTool string, args []string) error {
	cmd := exec.Command(exifTool, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, trimExifToolOut(out))
	}
	return nil
}

func trimExifToolOut(b []byte) string {
	const max = 400
	s := string(b)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
