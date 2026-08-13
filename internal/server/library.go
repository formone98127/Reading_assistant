package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
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
	case path == "publish":
		if r.Method == http.MethodPost {
			s.handleLibraryPublish(w, r)
			return
		}
	case path == "delete":
		if r.Method == http.MethodPost || r.Method == http.MethodDelete {
			s.handleLibraryDelete(w, r)
			return
		}
	case path == "audio":
		if r.Method == http.MethodGet {
			s.handleLibraryAudio(w, r)
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
	q := r.URL.Query()
	name, data, err := s.library.HTMLExportBook(id, q.Get("mode"), q.Get("showEasier"), q.Get("showChinese"))
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
		BookID      string `json:"bookId"`
		ShowEasier  *bool  `json:"showEasier"`
		ShowChinese *bool  `json:"showChinese"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookID == "" {
		http.Error(w, "bookId required", http.StatusBadRequest)
		return
	}
	opts := readingOptionsFromJSON(req.ShowEasier, req.ShowChinese, "")
	restoreSaved := req.ShowEasier == nil && req.ShowChinese == nil
	sess, meta, sid, err := s.openLibrarySession(req.BookID, opts, restoreSaved)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.prepareWhileReading(sess)
	s.ensureLibraryRewrite(meta)
	o := sess.ReadingOptionsValue()
	writeJSON(w, map[string]any{
		"sessionId":      sid,
		"total":          len(sess.Sentences),
		"state":          s.enrichView(sess, sess.View()),
		"bookId":         meta.ID,
		"sourceFilename": meta.Title,
		"readingMode":    session.ModeFromOptions(o),
		"track":          o.NavTrack(),
		"showEasier":     o.ShowEasier,
		"showChinese":    o.ShowChinese,
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
	opts := readingOptionsFromForm(r)
	voiceOnly := r.FormValue("voiceOnly") == "true"
	ttsEnabled := r.FormValue("generateVoice") != "false"
	if voiceOnly {
		ttsEnabled = true
		opts = session.ReadingOptions{}
	}
	meta, err := s.library.ImportWithOptions(title, filename, data, parts, opts, ttsEnabled, voiceOnly)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.startLibraryRewrite(meta.ID)
	writeJSON(w, map[string]any{
		"book":        meta,
		"openNow":     true,
		"readingMode": meta.ReadingMode,
	})
}

func (s *Server) openLibrarySession(bookID string, opts session.ReadingOptions, restoreSaved bool) (*session.Session, *library.Meta, string, error) {
	meta, err := s.library.LoadMeta(bookID)
	if err != nil {
		return nil, nil, "", err
	}
	prog, err := s.library.LoadReadProgress(bookID)
	if err != nil {
		return nil, nil, "", err
	}
	if restoreSaved {
		opts = session.ReadingOptions{ShowEasier: prog.ShowEasier, ShowChinese: prog.ShowChinese}
	}
	parts, err := s.library.LoadSentences(bookID)
	if err != nil {
		return nil, nil, "", err
	}
	prepared, err := s.library.LoadPrepared(bookID)
	if err != nil {
		return nil, nil, "", err
	}
	_ = s.library.SetReadingOptions(bookID, opts)
	meta, _ = s.library.LoadMeta(bookID)
	sid := newSessionID()
	sess := s.manager.Create(sid, parts)
	sess.SetBookID(bookID)
	sess.SetSource(s.library.Dir(bookID), meta.Title)
	sess.SetReadingOptions(opts)
	sess.SeedPrepared(prepared.English, prepared.Chinese)
	sess.ApplyReadPosition(prog.Index, prog.Level)
	return sess, meta, sid, nil
}

func (s *Server) cancelLibraryRewrite(bookID string) {
	s.rewriteMu.Lock()
	if cancel, ok := s.rewriteCancel[bookID]; ok {
		cancel()
		delete(s.rewriteCancel, bookID)
	}
	s.rewriteMu.Unlock()
}

func (s *Server) handleLibraryAudio(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query required", http.StatusBadRequest)
		return
	}
	idxStr := r.URL.Query().Get("index")
	if idxStr == "" {
		http.Error(w, "index query required", http.StatusBadRequest)
		return
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 {
		http.Error(w, "invalid index", http.StatusBadRequest)
		return
	}
	if !s.library.HasAudio(id, idx) {
		http.Error(w, "audio not found", http.StatusNotFound)
		return
	}
	data, err := s.library.LoadAudio(id, idx)
	if err != nil {
		http.Error(w, "audio not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(data)
}

func (s *Server) handleLibraryDelete(w http.ResponseWriter, r *http.Request) {
	bookID := r.URL.Query().Get("id")
	if bookID == "" {
		var req struct {
			BookID string `json:"bookId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			bookID = req.BookID
		}
	}
	if bookID == "" {
		http.Error(w, "bookId required", http.StatusBadRequest)
		return
	}
	s.cancelLibraryRewrite(bookID)
	if err := s.library.Delete(bookID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "bookId": bookID})
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
		prepared, err := s.library.LoadPrepared(meta.ID)
		if err == nil && !library.RewriteComplete(s.library, meta.ID, prepared, meta.TotalSentences, meta.TTSEnabled, meta.VoiceOnly) {
			s.startLibraryRewrite(meta.ID)
			return
		}
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
		s.cancelLibraryRewrite(bookID)
	}
	_ = s.library.PersistPrepared(bookID, sess.PreparedSnapshot(), sess.ChineseSnapshot())
}

func (s *Server) persistReadPosition(sess *session.Session) {
	bookID := sess.BookIDValue()
	if bookID == "" {
		return
	}
	v := sess.View()
	opts := sess.ReadingOptionsValue()
	_ = s.library.SaveReadProgress(bookID, library.ReadProgress{
		Index:       v.Index,
		Level:       v.Level,
		ShowEasier:  opts.ShowEasier,
		ShowChinese: opts.ShowChinese,
	})
}
