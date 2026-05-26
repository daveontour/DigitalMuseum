package georegion

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

// RegionDefinition is one region entry from regions.json.
type RegionDefinition struct {
	Code  string    `json:"code"`
	Label string    `json:"label"`
	BBox  []float64 `json:"bbox"` // [min_lon, min_lat, max_lon, max_lat]
}

// Config is the parsed regions.json document.
type Config struct {
	DefaultRegion string             `json:"default_region"`
	DefaultLabel  string             `json:"default_label"`
	Regions       []RegionDefinition `json:"regions"`
}

type regionDef struct {
	code  string
	label string
	bbox  [4]float64
}

// Registry holds loaded region definitions for lookup and label resolution.
type Registry struct {
	config Config
	boxes  []regionDef
	labels map[string]string
}

var defaultRegistry *Registry

// Load reads and validates regions.json from path and sets the package default registry.
func Load(path string) error {
	reg, err := loadRegistry(path)
	if err != nil {
		return err
	}
	defaultRegistry = reg
	return nil
}

// Default returns the loaded registry or panics if Load was not called.
func Default() *Registry {
	if defaultRegistry == nil {
		panic("georegion: Load must be called before use")
	}
	return defaultRegistry
}

func loadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read regions config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse regions config %s: %w", path, err)
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("validate regions config %s: %w", path, err)
	}

	reg := &Registry{
		config: cfg,
		labels: make(map[string]string, len(cfg.Regions)),
	}
	for _, r := range cfg.Regions {
		reg.boxes = append(reg.boxes, regionDef{
			code:  r.Code,
			label: r.Label,
			bbox:  [4]float64{r.BBox[0], r.BBox[1], r.BBox[2], r.BBox[3]},
		})
		reg.labels[strings.ToLower(r.Code)] = r.Label
	}
	if cfg.DefaultRegion != "" {
		dk := strings.ToLower(cfg.DefaultRegion)
		if _, ok := reg.labels[dk]; !ok && cfg.DefaultLabel != "" {
			reg.labels[dk] = cfg.DefaultLabel
		}
	}
	return reg, nil
}

func validateConfig(cfg *Config) error {
	if strings.TrimSpace(cfg.DefaultRegion) == "" {
		return fmt.Errorf("default_region is required")
	}
	if len(cfg.Regions) == 0 {
		return fmt.Errorf("regions must not be empty")
	}
	seen := make(map[string]struct{}, len(cfg.Regions))
	for i, r := range cfg.Regions {
		code := strings.TrimSpace(r.Code)
		if code == "" {
			return fmt.Errorf("regions[%d]: code is required", i)
		}
		if strings.TrimSpace(r.Label) == "" {
			return fmt.Errorf("regions[%d] code=%q: label is required", i, code)
		}
		cl := strings.ToLower(code)
		if _, ok := seen[cl]; ok {
			return fmt.Errorf("duplicate region code %q", code)
		}
		seen[cl] = struct{}{}
		if len(r.BBox) != 4 {
			return fmt.Errorf("regions[%d] code=%q: bbox must have 4 numbers", i, code)
		}
		for j, v := range r.BBox {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("regions[%d] code=%q: bbox[%d] must be finite", i, code, j)
			}
		}
		minLon, minLat, maxLon, maxLat := r.BBox[0], r.BBox[1], r.BBox[2], r.BBox[3]
		if minLon > maxLon {
			return fmt.Errorf("regions[%d] code=%q: min_lon > max_lon", i, code)
		}
		if minLat > maxLat {
			return fmt.Errorf("regions[%d] code=%q: min_lat > max_lat", i, code)
		}
	}
	return nil
}

// ConfigJSON returns the loaded config for API/bootstrap responses.
func (r *Registry) ConfigJSON() Config {
	return r.config
}

// Label returns the display label for a region code.
func (r *Registry) Label(code string) string {
	if strings.TrimSpace(code) == "" {
		return "Unknown"
	}
	k := strings.ToLower(strings.TrimSpace(code))
	if lbl, ok := r.labels[k]; ok {
		return lbl
	}
	if r.config.DefaultLabel != "" && k == strings.ToLower(r.config.DefaultRegion) {
		return r.config.DefaultLabel
	}
	return code
}

// RegionFromLatLng maps coordinates to a region code using bounding-box rules.
func (r *Registry) RegionFromLatLng(lat, lng float64) string {
	for _, box := range r.boxes {
		if inRegionBBox(lat, lng, box.bbox) {
			return box.code
		}
	}
	return r.config.DefaultRegion
}

// RegionFromLatLng uses the default registry loaded at startup.
func RegionFromLatLng(lat, lng float64) string {
	return Default().RegionFromLatLng(lat, lng)
}

// Label uses the default registry loaded at startup.
func Label(code string) string {
	return Default().Label(code)
}
