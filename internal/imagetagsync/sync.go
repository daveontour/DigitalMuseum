package imagetagsync

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
)

var imageExtensions = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {}, ".webp": {},
	".heic": {}, ".heif": {}, ".tif": {}, ".tiff": {}, ".bmp": {},
}

// Options configures a tag sync run.
type Options struct {
	JSONPath  string
	RootDir   string
	ExifTool  string
	Logger    *log.Logger
}

// Result summarizes a tag sync run.
type Result struct {
	FilesSeen    int
	TagsWritten  int
	Skipped      int
	Errors       int
}

// Run walks rootDir, matches each image stem to JSON entries, and writes tags via exiftool.
func Run(opts Options) (Result, error) {
	if opts.Logger == nil {
		opts.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	jsonPath := strings.TrimSpace(opts.JSONPath)
	rootDir := strings.TrimSpace(opts.RootDir)
	if jsonPath == "" {
		return Result{}, fmt.Errorf("json file path is required")
	}
	if rootDir == "" {
		return Result{}, fmt.Errorf("directory path is required")
	}

	exifTool, err := ResolveExifTool(opts.ExifTool)
	if err != nil {
		return Result{}, err
	}

	index, dupes, err := loadIndex(jsonPath, opts.Logger)
	if err != nil {
		return Result{}, err
	}
	if dupes > 0 {
		opts.Logger.Printf("index: skipped %d duplicate lookup key(s)", dupes)
	}

	var res Result
	walkErr := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			res.Errors++
			opts.Logger.Printf("skip %s: walk error: %v", path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if _, ok := imageExtensions[ext]; !ok {
			return nil
		}
		res.FilesSeen++

		stem := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		rec, ok := index[stem]
		if !ok {
			res.Skipped++
			opts.Logger.Printf("skip %s: no JSON entry for stem %q", path, stem)
			return nil
		}
		tags := tagsForRecord(rec)
		if len(tags) == 0 {
			res.Skipped++
			opts.Logger.Printf("skip %s: no tags or source in JSON entry", path)
			return nil
		}
		if err := WriteTags(exifTool, path, tags); err != nil {
			res.Errors++
			opts.Logger.Printf("error %s: %v", path, err)
			return nil
		}
		res.TagsWritten++
		opts.Logger.Printf("tagged %s (%d tag(s))", path, len(tags))
		return nil
	})
	if walkErr != nil {
		return res, walkErr
	}
	return res, nil
}

func loadIndex(jsonPath string, logger *log.Logger) (map[string]model.ImageMetadataJSONRecord, int, error) {
	f, err := os.Open(jsonPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open json: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, fmt.Errorf("read json: %w", err)
	}

	var records []model.ImageMetadataJSONRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, 0, fmt.Errorf("parse json: %w", err)
	}

	index := make(map[string]model.ImageMetadataJSONRecord, len(records)*2)
	dupes := 0
	addKey := func(key string, rec model.ImageMetadataJSONRecord) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if _, exists := index[key]; exists {
			dupes++
			logger.Printf("index: duplicate lookup key %q (keeping first entry)", key)
			return
		}
		index[key] = rec
	}

	for _, rec := range records {
		addKey(strconv.FormatInt(rec.ID, 10), rec)
		if rec.SourceReference != nil {
			addKey(*rec.SourceReference, rec)
		}
	}
	return index, dupes, nil
}

func splitTags(raw *string) []string {
	if raw == nil {
		return nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// tagsForRecord returns comma-split tags plus the entry source (deduped, case-insensitive).
func tagsForRecord(rec model.ImageMetadataJSONRecord) []string {
	tags := splitTags(rec.Tags)
	source := ""
	if rec.Source != nil {
		source = strings.TrimSpace(*rec.Source)
	}
	if source == "" {
		return tags
	}
	for _, t := range tags {
		if strings.EqualFold(t, source) {
			return tags
		}
	}
	return append(tags, source)
}
