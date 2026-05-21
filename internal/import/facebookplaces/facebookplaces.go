package facebookplaces

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/daveontour/aimuseum/internal/importstorage"
)

const facebookSource = "facebook"

// ImportStats holds statistics about the import process.
type ImportStats struct {
	PlacesImported int
	PlacesCreated  int
	PlacesUpdated  int
	Errors         []string
}

// ProgressCallback is called after each file is processed.
type ProgressCallback func(ImportStats)

// CancelledCheck returns true if the import should be cancelled.
type CancelledCheck func() bool

// ExtractPlacesFromData recursively extracts all 'place' elements from nested JSON structures.
func ExtractPlacesFromData(data interface{}, placesList *[]map[string]interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		if place, ok := v["place"]; ok {
			if placeMap, ok := place.(map[string]interface{}); ok {
				*placesList = append(*placesList, placeMap)
			}
		}
		for _, val := range v {
			ExtractPlacesFromData(val, placesList)
		}
	case []interface{}:
		for _, item := range v {
			ExtractPlacesFromData(item, placesList)
		}
	}
}

// ImportFacebookPlacesFromFile imports places from a single Facebook posts JSON file.
func ImportFacebookPlacesFromFile(ctx context.Context, pool *sql.DB, filePath string) (*ImportStats, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	var placesList []map[string]interface{}
	ExtractPlacesFromData(jsonData, &placesList)

	stats := &ImportStats{Errors: []string{}}
	storage := importstorage.NewFacebookPlacesStorage(pool)

	for _, placeData := range placesList {
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		default:
		}

		nameVal, _ := placeData["name"].(string)
		name := strings.TrimSpace(nameVal)
		if name == "" {
			stats.Errors = append(stats.Errors, fmt.Sprintf("Skipped place with empty name: %v", placeData))
			continue
		}

		var latitude, longitude *float64
		if coord, ok := placeData["coordinate"].(map[string]interface{}); ok {
			if lat, ok := toFloat64(coord["latitude"]); ok {
				latitude = &lat
			}
			if lng, ok := toFloat64(coord["longitude"]); ok {
				longitude = &lng
			}
		}

		address := ""
		if a, ok := placeData["address"].(string); ok {
			address = strings.TrimSpace(a)
		}

		url := ""
		if u, ok := placeData["url"].(string); ok {
			url = strings.TrimSpace(u)
		}

		created, err := storage.SaveOrUpdateLocation(ctx, name, address, latitude, longitude, facebookSource, url)
		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Sprintf("Error processing place %s: %v", name, err))
			continue
		}

		stats.PlacesImported++
		if created {
			stats.PlacesCreated++
		} else {
			stats.PlacesUpdated++
		}
	}

	if err := storage.UpdateLocationRegions(ctx); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("update_location_regions: %v (non-fatal)", err))
	}

	return stats, nil
}

// ImportFacebookPlacesFromDirectory imports places from all JSON files in a directory.
func ImportFacebookPlacesFromDirectory(ctx context.Context, pool *sql.DB, directoryPath string, progressCallback ProgressCallback, cancelledCheck CancelledCheck) (*ImportStats, error) {
	var jsonFiles []string
	err := filepath.WalkDir(directoryPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			jsonFiles = append(jsonFiles, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}
	sort.Strings(jsonFiles)

	aggStats := &ImportStats{Errors: []string{}}
	storage := importstorage.NewFacebookPlacesStorage(pool)

	for _, jsonFile := range jsonFiles {
		if cancelledCheck != nil && cancelledCheck() {
			break
		}
		select {
		case <-ctx.Done():
			return aggStats, ctx.Err()
		default:
		}

		stats, err := ImportFacebookPlacesFromFile(ctx, pool, jsonFile)
		if err != nil {
			aggStats.Errors = append(aggStats.Errors, fmt.Sprintf("Error processing %s: %v", jsonFile, err))
			continue
		}

		aggStats.PlacesImported += stats.PlacesImported
		aggStats.PlacesCreated += stats.PlacesCreated
		aggStats.PlacesUpdated += stats.PlacesUpdated
		aggStats.Errors = append(aggStats.Errors, stats.Errors...)

		if progressCallback != nil {
			progressCallback(*aggStats)
		}
	}

	if err := storage.UpdateLocationRegions(ctx); err != nil {
		aggStats.Errors = append(aggStats.Errors, fmt.Sprintf("update_location_regions: %v (non-fatal)", err))
	}

	return aggStats, nil
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
