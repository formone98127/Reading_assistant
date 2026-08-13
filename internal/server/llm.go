package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"reading-assistant/internal/config"
	"reading-assistant/internal/simplify"
)

func applyLLMConfig(cfg config.Config, llm *simplify.Client) {
	llm.Backend = simplify.Backend(config.NormalizeLLMProvider(cfg.LLMProvider))
	if llm.Backend == simplify.BackendFreebuff {
		llm.BaseURL = cfg.FreebuffURL
		llm.Model = cfg.FreebuffModel
		llm.APIKey = cfg.FreebuffAPIKey
		return
	}
	llm.Backend = simplify.BackendOllama
	llm.BaseURL = cfg.OllamaURL
	llm.Model = cfg.OllamaModel
	llm.APIKey = ""
}

func (s *Server) handleLLM(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleLLMGet(w, r)
	case http.MethodPost:
		s.handleLLMSet(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLLMGet(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	cfg := s.cfg
	s.cfgMu.RUnlock()

	provider := config.NormalizeLLMProvider(cfg.LLMProvider)
	var models []string
	var ok bool
	var freebuffWarning string
	if provider == config.LLMProviderFreebuff {
		var err error
		raw, err := config.ListOpenAIModels(cfg.FreebuffURL, cfg.FreebuffAPIKey)
		ok = err == nil
		models = config.FilterFreebuffModels(raw)
		if ok && len(models) == 0 && len(raw) > 0 {
			freebuffWarning = "proxy has only Gemini subagent models; reinstall Freebuff2API or use Ollama"
		}
	} else {
		var err error
		models, err = config.ListOllamaModels(cfg.OllamaURL)
		ok = err == nil
	}
	activeModel := config.LLMModel(cfg)
	if provider == config.LLMProviderFreebuff && ok && len(models) > 0 {
		if _, onProxy := config.MatchRootModelOnProxy(activeModel, models); !onProxy {
			if resolved, err := config.PickFreebuffModel(activeModel, cfg.FreebuffURL, cfg.FreebuffAPIKey); err == nil {
				s.cfgMu.Lock()
				s.cfg.FreebuffModel = resolved
				s.cfgMu.Unlock()
				applyLLMConfig(s.cfg, s.llm)
				activeModel = resolved
				user := config.LoadUserSettings(s.cfg.SaveDir)
				user.FreebuffModel = resolved
				_ = config.SaveUserSettings(s.cfg.SaveDir, user)
			}
		}
	}
	writeJSON(w, map[string]any{
		"provider":         provider,
		"model":              activeModel,
		"url":                config.LLMBaseURL(cfg),
		"models":             models,
		"freebuffWarning":    freebuffWarning,
		"llmOk":              ok,
		"ollamaOk":      provider == config.LLMProviderOllama && ok,
		"freebuffUrl":   cfg.FreebuffURL,
		"freebuffModel": cfg.FreebuffModel,
		"ollamaUrl":     cfg.OllamaURL,
		"ollamaModel":   cfg.OllamaModel,
	})
}

func (s *Server) handleLLMSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider       string `json:"provider"`
		Model          string `json:"model"`
		FreebuffURL    string `json:"freebuffUrl"`
		FreebuffAPIKey string `json:"freebuffApiKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.setLLM(req.Provider, req.Model, req.FreebuffURL, req.FreebuffAPIKey); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.cfgMu.RLock()
	cfg := s.cfg
	s.cfgMu.RUnlock()
	log.Printf("llm: provider=%s model=%q", cfg.LLMProvider, config.LLMModel(cfg))
	writeJSON(w, map[string]any{
		"provider": config.NormalizeLLMProvider(cfg.LLMProvider),
		"model":    config.LLMModel(cfg),
	})
}

func (s *Server) setLLM(provider, model, freebuffURL, freebuffAPIKey string) error {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()

	if p := strings.TrimSpace(provider); p != "" {
		s.cfg.LLMProvider = config.NormalizeLLMProvider(p)
	}
	if u := strings.TrimSpace(freebuffURL); u != "" {
		s.cfg.FreebuffURL = u
	}
	if freebuffAPIKey != "" {
		s.cfg.FreebuffAPIKey = strings.TrimSpace(freebuffAPIKey)
	}
	if m := strings.TrimSpace(model); m != "" {
		if s.cfg.LLMProvider == config.LLMProviderFreebuff {
			resolved, err := config.PickFreebuffModel(m, s.cfg.FreebuffURL, s.cfg.FreebuffAPIKey)
			if err != nil {
				return err
			}
			s.cfg.FreebuffModel = resolved
		} else {
			s.cfg.OllamaModel = m
		}
	}
	applyLLMConfig(s.cfg, s.llm)

	user := config.LoadUserSettings(s.cfg.SaveDir)
	user.LLMProvider = s.cfg.LLMProvider
	user.OllamaModel = s.cfg.OllamaModel
	user.FreebuffURL = s.cfg.FreebuffURL
	user.FreebuffModel = s.cfg.FreebuffModel
	user.FreebuffAPIKey = s.cfg.FreebuffAPIKey
	return config.SaveUserSettings(s.cfg.SaveDir, user)
}

func (s *Server) currentModel() string {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return config.LLMModel(s.cfg)
}
