package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"reading-assistant/internal/config"
)

// Legacy /api/ollama — same as /api/llm; POST {model} keeps Ollama provider.
func (s *Server) handleOllama(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && strings.TrimSpace(req.Model) != "" {
			_ = s.setLLM(config.LLMProviderOllama, req.Model, "", "")
		}
	}
	s.handleLLM(w, r)
}
