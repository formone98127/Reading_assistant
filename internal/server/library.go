package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"reading-assistant/internal/library"
	"reading-assistant/internal/parser"
	"reading-assistant/internal/save"
	"reading-assistant/internal/sentences"
	"reading-assistant/internal/session"
)

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/library")
	path = strings.TrimPrefix(path, "/")
	switch {
	case path == "":
		if r.Method == http.MethodGet {
			s.handleLibraryList(w, r)
			return
		}
	case path == "import":
		if r.Method == http.MethodPost {
			s.handleLibraryImport(w, r)
			return
		}
	case path == "open":
		if r.Method == http.MethodPost {
			s.handleLibraryOpen(w, r)
			return
		}
	case path == "book" || strings.HasPrefix(path, "book/"):
		if r.Method == http.MethodGet {
			s.handleLibraryBook(w, r)
			return
		}
	case path == "export":
		if r.Method == http.MethodGet {
			s.handleLibraryExport(w, r)
			return
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) handleLibraryExport(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query required", http.StatusBadRequest)
		return
	}
	name, data, err := s.library.HTMLExport(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(data)
}

func (s *Server) handleLibraryList(w http.ResponseWriter, r *http.Request) {
	list, err := s.library.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"books": list})
}

func (s *Server) handleLibraryBook(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query required", http.StatusBadRequest)
		return
	}
	meta, err := s.library.LoadMeta(id)
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"book": meta})
}

func (s *Server) handleLibraryOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BookID string `json:"bookId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookID == "" {
		http.Error(w, "bookId required", http.StatusBadRequest)
		return
	}
	sess, meta, sid, err := s.openLibrarySession(req.BookID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.prepareWhileReading(sess)
	s.ensureLibraryRewrite(meta)
	writeJSON(w, map[string]any{
		"sessionId":      sid,
		"total":          len(sess.Sentences),
		"state":          s.enrichView(sess, sess.View()),
		"bookId":         meta.ID,
		"sourceFilename": meta.Title,
	})
}

func (s *Server) handleLibraryImport(w http.ResponseWriter, r *http.Request) {
	const maxBody = 32 << 20
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
	filename := hdr.Filename
	text, err := parser.ExtractText(filename, bytes.NewReader(data))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	parts := sentences.Split(text)
	if len(parts) == 0 {
		http.Error(w, "no sentences detected", http.StatusBadRequest)
		return
	}
	title := save.BaseName(filename)
	meta, err := s.library.Import(title, filename, data, parts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.startLibraryRewrite(meta.ID)
	writeJSON(w, map[string]any{
		"book":    meta,
		"openNow": true,
	})
}

func (s *Server) openLibrarySession(bookID string) (*session.Session, *library.Meta, string, error) {
	meta, err := s.library.LoadMeta(bookID)
	if err != nil {
		return nil, nil, "", err
	}
	parts, err := s.library.LoadSentences(bookID)
	if err != nil {
		return nil, nil, "", err
	}
	prepared, err := s.library.LoadPrepared(bookID)
	if err != nil {
		return nil, nil, "", err
	}
	sid := newSessionID()
	sess := s.manager.Create(sid, parts)
	sess.SetBookID(bookID)
	sess.SetSource(s.library.Dir(bookID), meta.Title)
	sess.SeedPrepared(prepared)
	sess.ApplyReadPosition(meta.ReadIndex, meta.ReadLevel)
	return sess, meta, sid, nil
}

func (s *Server) startLibraryRewrite(bookID string) {
	s.rewriteMu.Lock()
	if s.rewriteCancel == nil {
		s.rewriteCancel = make(map[string]context.CancelFunc)
	}
	if cancel, ok := s.rewriteCancel[bookID]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.rewriteCancel[bookID] = cancel
	s.rewriteMu.Unlock()

	go func() {
		defer func() {
			s.rewriteMu.Lock()
			delete(s.rewriteCancel, bookID)
			s.rewriteMu.Unlock()
		}()
		ctx, cancelTimeout := context.WithTimeout(ctx, 2*time.Hour)
		defer cancelTimeout()
		if err := s.rewriter.RewriteAll(ctx, bookID, nil); err != nil && ctx.Err() == nil {
			log.Printf("library rewrite %s: %v", bookID, err)
		}
	}()
}

func (s *Server) ensureLibraryRewrite(meta *library.Meta) {
	if meta.RewriteStatus == library.StatusDone {
		return
	}
	s.startLibraryRewrite(meta.ID)
}

func (s *Server) flushLibraryFromSession(sess *session.Session, stopRewrite bool) {
	bookID := sess.BookIDValue()
	if bookID == "" {
		return
	}
	if stopRewrite {
		s.rewriteMu.Lock()
		if cancel, ok := s.rewriteCancel[bookID]; ok {
			cancel()
			delete(s.rewriteCancel, bookID)
		}
		s.rewriteMu.Unlock()
	}
	_ = s.library.PersistPrepared(bookID, sess.PreparedSnapshot())
}

func (s *Server) persistReadPosition(sess *session.Session) {
	bookID := sess.BookIDValue()
	if bookID == "" {
		return
	}
	v := sess.View()
	_ = s.library.SaveReadPosition(bookID, v.Index, v.Level)
}
