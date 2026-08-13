package server

import (
	"net/http"
	"strings"

)

// handleBookReader serves an exported book as a full-page website at /book/{id}.
func (s *Server) handleBookReader(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/book/")
	id = strings.Trim(id, "/")
	if id == "" {
		http.Redirect(w, r, "/export.html", http.StatusFound)
		return
	}
	q := r.URL.Query()
	_, data, err := s.library.HTMLExportBook(id, q.Get("mode"), q.Get("showEasier"), q.Get("showChinese"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (s *Server) handleLibraryPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bookID := r.URL.Query().Get("id")
	if bookID == "" {
		http.Error(w, "id query required", http.StatusBadRequest)
		return
	}
	q := r.URL.Query()
	modeParam := q.Get("mode")
	opts := readingOptionsFromQuery(modeParam, q.Get("showEasier"), q.Get("showChinese"))
	path, err := s.library.PublishHTML(bookID, modeParam, q.Get("showEasier"), q.Get("showChinese"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	siteURL := scheme + "://" + r.Host + "/book/" + bookID + exportBookQuery(opts, q.Get("showEasier"), q.Get("showChinese"), modeParam)
	writeJSON(w, map[string]any{
		"ok":       true,
		"path":     path,
		"siteUrl":  siteURL,
		"bookId":   bookID,
		"fileName": "reader.html",
	})
}
