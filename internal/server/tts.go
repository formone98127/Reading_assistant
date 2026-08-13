package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"reading-assistant/internal/config"
	"reading-assistant/internal/tts"
)

func (s *Server) handleTTS(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleTTSGet(w, r)
	case http.MethodPost:
		s.handleTTSSet(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTTSGet(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	cfg := s.cfg
	s.cfgMu.RUnlock()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	st := tts.NewClient(cfg.VoxCPMURL).HealthStatus(ctx)

	writeJSON(w, map[string]any{
		"url":                cfg.VoxCPMURL,
		"control":            cfg.VoxCPMControl,
		"cfgValue":           cfg.VoxCPMCfg,
		"inferenceTimesteps": cfg.VoxCPMTimesteps,
		"ok":                 st.Ready,
		"reachable":          st.Reachable,
		"loading":            st.Loading,
	})
}

func (s *Server) handleTTSSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL                string   `json:"url"`
		Control            *string  `json:"control"`
		CFGValue           *float64 `json:"cfgValue"`
		InferenceTimesteps *int     `json:"inferenceTimesteps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.cfgMu.Lock()
	if u := strings.TrimSpace(req.URL); u != "" {
		s.cfg.VoxCPMURL = u
	}
	if req.Control != nil {
		s.cfg.VoxCPMControl = strings.TrimSpace(*req.Control)
	}
	if req.CFGValue != nil {
		s.cfg.VoxCPMCfg = *req.CFGValue
	}
	if req.InferenceTimesteps != nil {
		s.cfg.VoxCPMTimesteps = *req.InferenceTimesteps
	}
	cfg := s.cfg
	applyRewriterTTS(cfg, s.rewriter)
	s.cfgMu.Unlock()

	user := config.LoadUserSettings(cfg.SaveDir)
	user.VoxCPMURL = cfg.VoxCPMURL
	user.VoxCPMControl = cfg.VoxCPMControl
	user.VoxCPMCfg = cfg.VoxCPMCfg
	user.VoxCPMTimesteps = cfg.VoxCPMTimesteps
	if err := config.SaveUserSettings(cfg.SaveDir, user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}
