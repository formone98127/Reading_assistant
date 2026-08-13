package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reading-assistant/internal/config"
	"reading-assistant/internal/library"
	"reading-assistant/internal/webfs"
	"reading-assistant/internal/parser"
	"reading-assistant/internal/save"
	"reading-assistant/internal/sentences"
	"reading-assistant/internal/session"
	"reading-assistant/internal/simplify"
)

type Server struct {
	cfg            config.Config
	cfgMu          sync.RWMutex
	llm            *simplify.Client
	manager        *session.Manager
	library        *library.Store
	rewriter       *library.Rewriter
	rewriteMu      sync.Mutex
	rewriteCancel  map[string]context.CancelFunc
	static         http.Handler
}

func New(cfg config.Config, static http.Handler) *Server {
	llm := &simplify.Client{}
	applyLLMConfig(cfg, llm)
	lib := library.NewStore(cfg.SaveDir)
	rw := &library.Rewriter{
		Store: lib,
		LLM:   llm,
	}
	applyRewriterTTS(cfg, rw)
	return &Server{
		cfg:      cfg,
		llm:      llm,
		manager:  session.NewManager(llm),
		library:  lib,
		rewriter: rw,
		rewriteCancel: make(map[string]context.CancelFunc),
		static:        static,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/llm", s.handleLLM)
	mux.HandleFunc("/api/ollama", s.handleOllama)
	mux.HandleFunc("/api/load", s.handleLoad)
	mux.HandleFunc("/api/easier", s.handleEasier)
	mux.HandleFunc("/api/harder", s.handleHarder)
	mux.HandleFunc("/api/next", s.handleNext)
	mux.HandleFunc("/api/prev", s.handlePrev)
	mux.HandleFunc("/api/goto", s.handleGoto)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/rsvp-text", s.handleRsvpText)
	mux.HandleFunc("/api/save", s.handleSave)
	mux.HandleFunc("/api/export/", s.handleExport)
	mux.HandleFunc("/api/library", s.handleLibrary)
	mux.HandleFunc("/api/library/", s.handleLibrary)
	mux.HandleFunc("/api/public", s.handlePublicAPI)
	mux.HandleFunc("/api/public/", s.handlePublicAPI)
	mux.HandleFunc("/api/progress", s.handleSaveProgress)
	mux.HandleFunc("/api/reading-mode", s.handleReadingMode)
	mux.HandleFunc("/api/tts", s.handleTTS)
	mux.HandleFunc("/book/", s.handleBookReader)
	mux.HandleFunc("/shared/", s.handleSharedBook)
	mux.Handle("/export", webfs.ExportHubHandler())
	mux.Handle("/export.html", webfs.ExportHubHandler())
	mux.Handle("/export.js", webfs.ExportHubHandler())
	mux.Handle("/export.css", webfs.ExportHubHandler())
	mux.Handle("/public", webfs.PublicHubHandler())
	mux.Handle("/public.html", webfs.PublicHubHandler())
	mux.Handle("/public.js", webfs.PublicHubHandler())
	if s.static != nil {
		mux.Handle("/", s.static)
	}
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	cfg := s.cfg
	s.cfgMu.RUnlock()
	writeJSON(w, map[string]any{
		"ok":       true,
		"provider": config.NormalizeLLMProvider(cfg.LLMProvider),
		"model":    config.LLMModel(cfg),
		"url":      config.LLMBaseURL(cfg),
		"ollama":   cfg.OllamaURL,
		"voxcpmUrl": cfg.VoxCPMURL,
	})
}

func (s *Server) handleLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	const maxBody = 32 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	var text string
	var filename string

	var readOpts session.ReadingOptions
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req struct {
			Text        string `json:"text"`
			ReadingMode string `json:"readingMode"`
			ShowEasier  *bool  `json:"showEasier"`
			ShowChinese *bool  `json:"showChinese"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		text = parser.FromPlain(req.Text)
		filename = "paste.txt"
		readOpts = readingOptionsFromJSON(req.ShowEasier, req.ShowChinese, req.ReadingMode)
	} else {
		if err := r.ParseMultipartForm(maxBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		readOpts = readingOptionsFromForm(r)
		if pasted := r.FormValue("text"); pasted != "" {
			text = parser.FromPlain(pasted)
			filename = "paste.txt"
		} else {
			file, hdr, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "provide a file or text field", http.StatusBadRequest)
				return
			}
			defer file.Close()
			filename = hdr.Filename
			text, err = parser.ExtractText(filename, file)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}

	if text == "" {
		http.Error(w, "no text found", http.StatusBadRequest)
		return
	}
	parts := sentences.Split(text)
	if len(parts) == 0 {
		http.Error(w, "no sentences detected", http.StatusBadRequest)
		return
	}
	id := newSessionID()
	sess := s.manager.Create(id, parts)
	sess.SetReadingOptions(readOpts)
	base := save.BaseName(filename)
	dir := filepath.Join(s.cfg.SaveDir, base)
	sess.SetSource(dir, filename)
	origPath, err := save.WriteOriginal(dir, base, parts)
	saved := save.Pair{Dir: dir, Base: base, OriginalPath: origPath}
	if err != nil {
		log.Printf("save on load: %v", err)
	}
	s.prepareWhileReading(sess)
	writeJSON(w, map[string]any{
		"sessionId":      id,
		"total":          len(parts),
		"state":          sess.View(),
		"saved":          saved,
		"sourceFilename": filename,
	})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	kind := strings.TrimPrefix(r.URL.Path, "/api/export/")
	switch kind {
	case "original", "rewritten", "html":
	default:
		http.Error(w, "use /api/export/original, /api/export/rewritten, or /api/export/html", http.StatusBadRequest)
		return
	}
	if kind == "html" {
		if bookID := sess.BookIDValue(); bookID != "" {
			name, data, err := s.library.HTMLExport(bookID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
			_, _ = w.Write(data)
			return
		}
		_, filename := sess.Source()
		base := save.BaseName(filename)
		if base == "" {
			base = "paste"
		}
		v := sess.View()
		sentencesOut := save.BuildReaderSentences(sess.Sentences, sess.PreparedSnapshot(), sess.ChineseSnapshot(), session.MaxLevel)
		o := sess.ReadingOptionsValue()
		o = save.EffectiveExportOptions(o, sentencesOut)
		data, err := save.BuildBookHTML(save.ReaderExport{
			Title:       base,
			StartIndex:  v.Index,
			StartLevel:  v.Level,
			MaxLevel:    session.MaxLevel,
			ReadingMode: session.ModeFromOptions(o),
			Sentences:   sentencesOut,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		name := save.SafeFilename(base) + ".html"
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		_, _ = w.Write(data)
		return
	}
	_, filename := sess.Source()
	base := save.BaseName(filename)
	if base == "" {
		base = "paste"
	}
	var paragraphs []string
	if kind == "original" {
		paragraphs = sess.Sentences
	} else {
		paragraphs = sess.ParagraphsAtLevel(session.MaxLevel)
	}
	name := base + "." + kind + ".txt"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write([]byte(save.JoinParagraphs(paragraphs)))
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	dir, filename := sess.Source()
	if dir == "" {
		base := save.BaseName(filename)
		if base == "" {
			base = "paste"
		}
		dir = filepath.Join(s.cfg.SaveDir, base)
	}
	base := save.BaseName(filename)
	if base == "" {
		base = "paste"
	}
	pair, err := save.WritePair(dir, base, sess.Sentences, sess.ParagraphsAtLevel(session.MaxLevel))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"saved": pair})
}

func (s *Server) sessionFromRequest(w http.ResponseWriter, r *http.Request) (*session.Session, bool) {
	id := r.Header.Get("X-Session-Id")
	if id == "" {
		id = r.URL.Query().Get("session")
	}
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return nil, false
	}
	sess, ok := s.manager.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return nil, false
	}
	return sess, true
}

func (s *Server) handleEasier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	// Don't tie Ollama to the HTTP request context — first load can take 1–2 min.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	log.Printf("simplify: session=%s model=%s", r.Header.Get("X-Session-Id"), s.currentModel())
	s.refreshSessionFromLibrary(sess)
	view, err := sess.Easier(ctx, s.llm)
	if err != nil {
		log.Printf("simplify error: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"state": s.enrichView(sess, view)})
	s.persistReadPosition(sess)
}

func (s *Server) handleHarder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	v := sess.Harder()
	writeJSON(w, map[string]any{"state": s.enrichView(sess, v)})
	s.persistReadPosition(sess)
}

func (s *Server) handleNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	state := sess.Next()
	s.prepareWhileReading(sess)
	writeJSON(w, map[string]any{"state": s.enrichView(sess, state)})
	s.persistReadPosition(sess)
}

func (s *Server) handlePrev(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	state := sess.Prev()
	s.prepareWhileReading(sess)
	writeJSON(w, map[string]any{"state": s.enrichView(sess, state)})
	s.persistReadPosition(sess)
}

func (s *Server) handleGoto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		Page int `json:"page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Page < 1 {
		http.Error(w, "invalid page", http.StatusBadRequest)
		return
	}
	state := sess.GoTo(req.Page - 1)
	s.prepareWhileReading(sess)
	writeJSON(w, map[string]any{"state": s.enrichView(sess, state)})
	s.persistReadPosition(sess)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, map[string]any{"state": s.enrichView(sess, sess.View())})
}

func (s *Server) handleRsvpText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	s.refreshSessionFromLibrary(sess)
	v := sess.View()
	writeJSON(w, map[string]any{
		"level":     v.Level,
		"index":     v.Index,
		"total":     v.Total,
		"sentences": sess.TextsAtUnifiedLevel(v.Level),
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (s *Server) prepareWhileReading(sess *session.Session) {
	s.refreshSessionFromLibrary(sess)
	// Library books: easier + 中文 come from rewrite cache only (no live LLM while reading).
	if sess.BookIDValue() != "" {
		return
	}
	o := sess.ReadingOptionsValue()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if o.ShowEasier {
			sess.PrepareWhileReading(ctx, s.llm)
		}
		if o.ShowChinese {
			sess.PrepareChinese(ctx, s.llm, sess.View().Index)
		}
	}()
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
