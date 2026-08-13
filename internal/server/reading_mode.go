package server

import (
	"encoding/json"
	"net/http"
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
		Track       string `json:"track"`
		ShowEasier  *bool  `json:"showEasier"`
		ShowChinese *bool  `json:"showChinese"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	opts := readingOptionsFromJSON(req.ShowEasier, req.ShowChinese, req.ReadingMode)
	if req.Track != "" {
		opts = mergeReadingOptions(opts, req.Track)
	}
	sess.SetReadingOptions(opts)
	if bookID := sess.BookIDValue(); bookID != "" {
		if err := s.library.SetReadingOptions(bookID, opts); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.persistReadPosition(sess)
		if meta, err := s.library.LoadMeta(bookID); err == nil {
			s.ensureLibraryRewrite(meta)
		}
	}
	s.prepareWhileReading(sess)
	writeJSON(w, map[string]any{"state": s.enrichView(sess, sess.View())})
}
