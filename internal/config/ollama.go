package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ollamaTags struct {
	Models []struct {
		Name       string    `json:"name"`
		ModifiedAt time.Time `json:"modified_at"`
	} `json:"models"`
}

// ResolveOllamaModel returns the best local model: gemma4 if installed, else most recent.
func ResolveOllamaModel(baseURL string) (string, error) {
	names, err := ListOllamaModels(baseURL)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no models in ollama; run: ollama pull gemma4")
	}
	for _, n := range names {
		if strings.HasPrefix(strings.ToLower(n), "gemma4") {
			return n, nil
		}
	}
	return names[0], nil
}

// ListOllamaModels returns installed Ollama model names (most recently modified first).
func ListOllamaModels(baseURL string) ([]string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := strings.TrimSuffix(baseURL, "/") + "/api/tags"
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama tags: %s", res.Status)
	}
	var tags ollamaTags
	if err := json.NewDecoder(res.Body).Decode(&tags); err != nil {
		return nil, err
	}
	if len(tags.Models) == 0 {
		return nil, nil
	}
	best := tags.Models[0].Name
	bestAt := tags.Models[0].ModifiedAt
	for _, m := range tags.Models[1:] {
		if m.ModifiedAt.After(bestAt) {
			best = m.Name
			bestAt = m.ModifiedAt
		}
	}
	// Return all names but ensure most-recent is first for non-gemma fallback.
	out := []string{best}
	for _, m := range tags.Models {
		if m.Name != best {
			out = append(out, m.Name)
		}
	}
	return out, nil
}
