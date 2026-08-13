package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	LLMProviderOllama   = "ollama"
	LLMProviderFreebuff = "freebuff"

	DefaultFreebuffURL = "http://127.0.0.1:8081/v1"
)

func NormalizeLLMProvider(p string) string {
	if p == LLMProviderFreebuff {
		return LLMProviderFreebuff
	}
	return LLMProviderOllama
}

type openAIModelsResp struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ListOpenAIModels lists models from an OpenAI-compatible API (e.g. Freebuff2API).
func ListOpenAIModels(baseURL, apiKey string) ([]string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	url := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	url += "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models: %s", res.Status)
	}
	var out openAIModelsResp
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	var names []string
	for _, m := range out.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			names = append(names, id)
		}
	}
	return names, nil
}

// ResolveFreebuffModel picks the first root-safe model from the proxy list.
func ResolveFreebuffModel(baseURL, apiKey string) (string, error) {
	return PickFreebuffModel("", baseURL, apiKey)
}

// LLMBaseURL returns the rewrite API base URL for the active provider.
func LLMBaseURL(c Config) string {
	if NormalizeLLMProvider(c.LLMProvider) == LLMProviderFreebuff {
		return c.FreebuffURL
	}
	return c.OllamaURL
}

// LLMModel returns the active model name.
func LLMModel(c Config) string {
	if NormalizeLLMProvider(c.LLMProvider) == LLMProviderFreebuff {
		return NormalizeFreebuffModel(c.FreebuffModel)
	}
	return c.OllamaModel
}
