package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reading-assistant/internal/save"
)

const publicBooksDir = "public"

type publicBook struct {
	FileName  string `json:"fileName"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	UpdatedAt string `json:"updatedAt"`
}

func (s *Server) publicDir() string {
	dir := filepath.Join(s.cfg.SaveDir, publicBooksDir)
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func publicBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func publicFileName(title string) string {
	base := save.SafeFilename(strings.TrimSuffix(title, filepath.Ext(title)))
	if base == "" {
		base = "book"
	}
	return fmt.Sprintf("%s-%d.html", base, time.Now().Unix())
}

func (s *Server) listPublicBooks(r *http.Request) ([]publicBook, error) {
	dir := s.publicDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]publicBook, 0, len(entries))
	base := publicBaseURL(r)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".html") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		title := strings.TrimSuffix(e.Name(), ".html")
		out = append(out, publicBook{
			FileName:  e.Name(),
			Title:     title,
			URL:       base + "/shared/" + e.Name(),
			Size:      info.Size(),
			UpdatedAt: info.ModTime().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

func (s *Server) handlePublicAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/public"), "/")
	switch {
	case path == "" && r.Method == http.MethodGet:
		books, err := s.listPublicBooks(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"books": books})
	case path == "upload" && r.Method == http.MethodPost:
		s.handlePublicUpload(w, r)
	case path == "publish" && r.Method == http.MethodPost:
		s.handlePublicPublishLibrary(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handlePublicUpload(w http.ResponseWriter, r *http.Request) {
	const maxBody = 64 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := r.ParseMultipartForm(maxBody); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sampleLen := len(data)
	if sampleLen > 4096 {
		sampleLen = 4096
	}
	if !strings.Contains(strings.ToLower(string(data[:sampleLen])), "<html") {
		http.Error(w, "upload an exported reader .html file", http.StatusBadRequest)
		return
	}
	name := publicFileName(hdr.Filename)
	if title := strings.TrimSpace(r.FormValue("title")); title != "" {
		name = publicFileName(title)
	}
	path := filepath.Join(s.publicDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok":       true,
		"fileName": name,
		"url":      publicBaseURL(r) + "/shared/" + name,
	})
}

func (s *Server) handlePublicPublishLibrary(w http.ResponseWriter, r *http.Request) {
	bookID := r.URL.Query().Get("id")
	if bookID == "" {
		http.Error(w, "id query required", http.StatusBadRequest)
		return
	}
	q := r.URL.Query()
	filename, data, err := s.library.HTMLExportBook(bookID, q.Get("mode"), q.Get("showEasier"), q.Get("showChinese"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	name := publicFileName(filename)
	path := filepath.Join(s.publicDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok":       true,
		"fileName": name,
		"url":      publicBaseURL(r) + "/shared/" + name,
	})
}

func (s *Server) handleSharedBook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/shared/"), "/")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || !strings.HasSuffix(strings.ToLower(name), ".html") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.publicDir(), name)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, path)
}
