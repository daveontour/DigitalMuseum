package imagetagsync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func defaultBundledExifToolRel() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("bin", "ExifTool", "exiftool.exe")
	}
	return filepath.Join("bin", "ExifTool", "exiftool")
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
