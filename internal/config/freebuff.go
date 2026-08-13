package config

import (
	"fmt"
	"strings"
)

// DefaultFreebuffModel matches Codebuff base2-free default.
const DefaultFreebuffModel = "minimax/minimax-m2.7"

// FreebuffRewriteModels are root-orchestrator models (base2-free family).
// Gemini IDs from the proxy are tool subagents and fail plain chat completions.
var FreebuffRewriteModels = []string{
	"minimax/minimax-m2.7",
	"moonshotai/kimi-k2.6",
	"deepseek/deepseek-v4-pro",
	"deepseek/deepseek-v4-flash",
}

// IsRootFreebuffModel reports whether id is a supported root rewrite model
// (exact or OpenRouter dated suffix, e.g. minimax/minimax-m2.7-20260211).
func IsRootFreebuffModel(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, allowed := range FreebuffRewriteModels {
		if id == allowed || strings.HasPrefix(id, allowed+"-") {
			return true
		}
	}
	return false
}

// FilterFreebuffModels returns root-safe models that the proxy actually lists.
func FilterFreebuffModels(proxyModels []string) []string {
	var out []string
	for _, m := range proxyModels {
		if IsRootFreebuffModel(m) {
			out = append(out, m)
		}
	}
	return out
}

// MatchRootModelOnProxy picks the proxy model id for a requested root model.
func MatchRootModelOnProxy(requested string, proxyModels []string) (string, bool) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", false
	}
	for _, p := range proxyModels {
		if p == requested {
			return p, true
		}
	}
	if !IsRootFreebuffModel(requested) {
		return "", false
	}
	for _, p := range proxyModels {
		if IsRootFreebuffModel(p) {
			for _, allowed := range FreebuffRewriteModels {
				if requested == allowed || strings.HasPrefix(requested, allowed+"-") {
					if p == allowed || strings.HasPrefix(p, allowed+"-") {
						return p, true
					}
				}
			}
		}
	}
	return "", false
}

// PickFreebuffModel chooses a model that exists on the proxy, preferring requested.
func PickFreebuffModel(requested, baseURL, apiKey string) (string, error) {
	requested = strings.TrimSpace(requested)
	proxy, err := ListOpenAIModels(baseURL, apiKey)
	if err != nil {
		if IsRootFreebuffModel(requested) {
			return requested, err
		}
		return "", err
	}
	if m, ok := MatchRootModelOnProxy(requested, proxy); ok {
		return m, nil
	}
	safe := FilterFreebuffModels(proxy)
	if len(safe) > 0 {
		return safe[0], nil
	}
	return "", fmt.Errorf(
		"Freebuff2API lists %v but no root rewrite models (%v). Reinstall: go install github.com/Quorinex/Freebuff2API@latest — or use Ollama",
		proxy,
		FreebuffRewriteModels,
	)
}

// NormalizeFreebuffModel maps legacy/unknown ids to a root default (offline).
func NormalizeFreebuffModel(model string) string {
	model = strings.TrimSpace(model)
	if IsRootFreebuffModel(model) {
		return model
	}
	return DefaultFreebuffModel
}
