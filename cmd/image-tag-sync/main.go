// image-tag-sync writes comma-separated archive tags from a metadata JSON export
// onto image files in a directory tree using bundled ExifTool.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/daveontour/aimuseum/internal/imagetagsync"
)

func main() {
	jsonPath := flag.String("json", "", "path to image metadata JSON export file (required)")
	rootDir := flag.String("dir", "", "root directory to walk for image files (required)")
	exifTool := flag.String("exiftool", "", "path to exiftool executable (default: bundled bin/ExifTool/)")
	flag.Parse()

	if *jsonPath == "" || *rootDir == "" {
		fmt.Fprintln(os.Stderr, "Usage: image-tag-sync -json <export.json> -dir <image-root> [-exiftool <path>]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)
	res, err := imagetagsync.Run(imagetagsync.Options{
		JSONPath: *jsonPath,
		RootDir:  *rootDir,
		ExifTool: *exifTool,
		Logger:   logger,
	})
	if err != nil {
		logger.Fatalf("failed: %v", err)
	}

	logger.Printf("done: %d image(s) seen, %d tagged, %d skipped, %d error(s)",
		res.FilesSeen, res.TagsWritten, res.Skipped, res.Errors)
	if res.Errors > 0 {
		os.Exit(1)
	}
}
