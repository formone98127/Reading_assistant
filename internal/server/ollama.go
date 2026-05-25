package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"reading-assistant/internal/config"
)

func (s *Server) handleOllama(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleOllamaGet(w, r)
	case http.MethodPost:
		s.handleOllamaSetModel(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOllamaGet(w http.ResponseWriter, r *http.Request) {
	models, err := config.ListOllamaModels(s.cfg.OllamaURL)
	if err != nil {
		models = nil
	}
	writeJSON(w, map[string]any{
		"url":     s.cfg.OllamaURL,
		"model":   s.currentModel(),
		"models":  models,
		"ollamaOk": err == nil,
	})
}

func (s *Server) handleOllamaSetModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}
	if err := s.setOllamaModel(model); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("ollama model set to %q", model)
	writeJSON(w, map[string]any{
		"model": s.currentModel(),
	})
}

func (s *Server) currentModel() string {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.OllamaModel
}

func (s *Server) setOllamaModel(model string) error {
	s.cfgMu.Lock()
	s.cfg.OllamaModel = model
	s.llm.Model = model
	s.cfgMu.Unlock()
	return config.SaveUserSettings(s.cfg.SaveDir, config.UserSettings{OllamaModel: model})
}
