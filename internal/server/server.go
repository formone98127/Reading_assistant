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
	"reading-assistant/internal/parser"
	"reading-assistant/internal/save"
	"reading-assistant/internal/sentences"
	"reading-assistant/internal/session"
	"reading-assistant/internal/simplify"
)

type Server struct {
	cfg            config.Config
	manager        *session.Manager
	library        *library.Store
	rewriter       *library.Rewriter
	rewriteMu      sync.Mutex
	rewriteCancel  map[string]context.CancelFunc
	static         http.Handler
}

func New(cfg config.Config, static http.Handler) *Server {
	llm := &simplify.Client{
		BaseURL: cfg.OllamaURL,
		Model:   cfg.OllamaModel,
	}
	lib := library.NewStore(cfg.SaveDir)
	return &Server{
		cfg:     cfg,
		manager: session.NewManager(llm),
		library: lib,
		rewriter: &library.Rewriter{
			Store: lib,
			LLM:   llm,
		},
		static: static,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/load", s.handleLoad)
	mux.HandleFunc("/api/easier", s.handleEasier)
	mux.HandleFunc("/api/harder", s.handleHarder)
	mux.HandleFunc("/api/next", s.handleNext)
	mux.HandleFunc("/api/prev", s.handlePrev)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/save", s.handleSave)
	mux.HandleFunc("/api/export/", s.handleExport)
	mux.HandleFunc("/api/library", s.handleLibrary)
	mux.HandleFunc("/api/library/", s.handleLibrary)
	mux.HandleFunc("/api/progress", s.handleSaveProgress)
	if s.static != nil {
		mux.Handle("/", s.static)
	}
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"ok":    true,
		"model": s.cfg.OllamaModel,
		"ollama": s.cfg.OllamaURL,
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

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		text = parser.FromPlain(req.Text)
		filename = "paste.txt"
	} else {
		if err := r.ParseMultipartForm(maxBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
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
	case "original", "rewritten":
	default:
		http.Error(w, "use /api/export/original or /api/export/rewritten", http.StatusBadRequest)
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
	llm := &simplify.Client{BaseURL: s.cfg.OllamaURL, Model: s.cfg.OllamaModel}
	log.Printf("simplify: session=%s model=%s", r.Header.Get("X-Session-Id"), s.cfg.OllamaModel)
	view, err := sess.Easier(ctx, llm)
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

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionFromRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, map[string]any{"state": s.enrichView(sess, sess.View())})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (s *Server) prepareWhileReading(sess *session.Session) {
	llm := s.manager.LLM()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		sess.PrepareWhileReading(ctx, llm)
	}()
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
