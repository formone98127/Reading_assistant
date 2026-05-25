package server

import (
	"encoding/json"
	"net/http"

	"reading-assistant/internal/session"
)

func (s *Server) handleReadingMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		ReadingMode string `json:"readingMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mode := session.NormalizeReadingMode(req.ReadingMode)
	sess.SetReadingMode(mode)
	if bookID := sess.BookIDValue(); bookID != "" {
		if err := s.library.SetReadingMode(bookID, mode); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if meta, err := s.library.LoadMeta(bookID); err == nil {
			s.ensureLibraryRewrite(meta)
		}
	}
	s.prepareWhileReading(sess)
	writeJSON(w, map[string]any{"state": s.enrichView(sess, sess.View())})
}
