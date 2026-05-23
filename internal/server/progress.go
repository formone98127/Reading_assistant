package server

import (
	"encoding/json"
	"net/http"

	"reading-assistant/internal/session"
)

func (s *Server) handleSaveProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, closeRewrite, ok := s.sessionFromRequestFlexible(w, r)
	if !ok {
		return
	}
	s.flushLibraryFromSession(sess, closeRewrite)
	s.persistReadPosition(sess)
	writeJSON(w, map[string]any{"ok": true})
}

// sessionFromRequestFlexible resolves session id from header, query, or JSON body (for sendBeacon on tab close).
func (s *Server) sessionFromRequestFlexible(w http.ResponseWriter, r *http.Request) (*session.Session, bool, bool) {
	id := r.Header.Get("X-Session-Id")
	closeRewrite := r.URL.Query().Get("close") == "1"
	if id == "" {
		id = r.URL.Query().Get("session")
	}
	if id == "" && r.Body != nil && r.ContentLength != 0 {
		var req struct {
			SessionID    string `json:"sessionId"`
			CloseRewrite bool   `json:"close"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if id == "" {
			id = req.SessionID
		}
		closeRewrite = closeRewrite || req.CloseRewrite
	}
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return nil, false, false
	}
	sess, ok := s.manager.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return nil, false, false
	}
	return sess, closeRewrite, true
}
