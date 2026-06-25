package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const ollamaListModelsTimeout = 5 * time.Second

// ListOllamaModels GETs {baseURL}/api/tags and returns sorted unique model names.
func ListOllamaModels(ctx context.Context, baseURL string) ([]string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("lOCALAI_BASE_URL is not configured")
	}

	reqCtx, cancel := context.WithTimeout(ctx, ollamaListModelsTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("hTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("decode tags: %w", err)
	}

	seen := make(map[string]struct{})
	var names []string
	for _, m := range tags.Models {
		n := strings.TrimSpace(m.Name)
		if n == "" {
			n = strings.TrimSpace(m.Model)
		}
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}
