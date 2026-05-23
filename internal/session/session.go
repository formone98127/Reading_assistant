package session

import (
	"context"
	"log"
	"strings"
	"sync"

	"reading-assistant/internal/simplify"
)

const MaxLevel = 3

type Session struct {
	mu        sync.Mutex
	Sentences []string
	Index     int
	Level     int
	Original  string
	Current   string
	Cache     map[int]string // levels 1-3 for current sentence

	SourceDir  string
	SourceName string
	BookID     string

	// prepared[idx] = simplified levels for that sentence (background LLM)
	prepared map[int]map[int]string
	inFlight map[int]chan struct{}
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	llm      *simplify.Client
}

func NewManager(llm *simplify.Client) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		llm:      llm,
	}
}

func (m *Manager) Create(id string, sentences []string) *Session {
	s := &Session{
		Sentences: sentences,
		Cache:     make(map[int]string),
		prepared:  make(map[int]map[int]string),
		inFlight:  make(map[int]chan struct{}),
	}
	if len(sentences) > 0 {
		s.applySentenceLocked(0)
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	return s, ok
}

func (m *Manager) LLM() *simplify.Client { return m.llm }

// SetSource records where exports should be written (e.g. folder of uploaded file).
func (s *Session) SetSource(dir, filename string) {
	s.mu.Lock()
	s.SourceDir = dir
	s.SourceName = filename
	s.mu.Unlock()
}

func (s *Session) SetBookID(id string) {
	s.mu.Lock()
	s.BookID = id
	s.mu.Unlock()
}

func (s *Session) BookIDValue() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.BookID
}

// RefreshPrepared merges levels loaded from disk (e.g. while background book rewrite runs).
func (s *Session) RefreshPrepared(all map[int]map[int]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx, levels := range all {
		cp := make(map[int]string, len(levels))
		for k, v := range levels {
			cp[k] = v
		}
		s.prepared[idx] = cp
	}
	idx := s.Index
	s.Cache = make(map[int]string)
	if levels, ok := s.prepared[idx]; ok {
		for k, v := range levels {
			s.Cache[k] = v
		}
	}
	if s.Level > 0 {
		if t, ok := s.Cache[s.Level]; ok {
			s.Current = t
		} else {
			s.Level = 0
			s.Current = s.Original
		}
	}
}

// SeedPrepared loads cached simplify levels (e.g. from library bundle).
func (s *Session) SeedPrepared(all map[int]map[int]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx, levels := range all {
		cp := make(map[int]string, len(levels))
		for k, v := range levels {
			cp[k] = v
		}
		s.prepared[idx] = cp
	}
	if levels, ok := s.prepared[s.Index]; ok {
		for k, v := range levels {
			s.Cache[k] = v
		}
	}
}

// ApplyReadPosition restores index and simplification level from library metadata.
func (s *Session) ApplyReadPosition(index, level int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.Sentences) {
		return
	}
	s.Index = index
	s.Original = s.Sentences[index]
	s.Level = 0
	s.Current = s.Original
	s.Cache = make(map[int]string)
	if levels, ok := s.prepared[index]; ok {
		for k, v := range levels {
			s.Cache[k] = v
		}
	}
	if level > 0 && level <= MaxLevel {
		if t, ok := s.Cache[level]; ok {
			s.Level = level
			s.Current = t
		}
	}
}

// Source returns the export directory and original filename.
func (s *Session) Source() (dir, filename string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.SourceDir, s.SourceName
}

// ParagraphsAtLevel returns each sentence at the given simplify level (falls back to easier levels or original).
func (s *Session) ParagraphsAtLevel(level int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.Sentences))
	for i, orig := range s.Sentences {
		text := orig
		if levels, ok := s.prepared[i]; ok {
			for lv := level; lv >= 1; lv-- {
				if t, ok := levels[lv]; ok && strings.TrimSpace(t) != "" {
					text = t
					break
				}
			}
		}
		out[i] = text
	}
	return out
}

// PreparedSnapshot returns a copy of all cached per-sentence simplify levels.
func (s *Session) PreparedSnapshot() map[int]map[int]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int]map[int]string, len(s.prepared))
	for idx, levels := range s.prepared {
		cp := make(map[int]string, len(levels))
		for k, v := range levels {
			cp[k] = v
		}
		out[idx] = cp
	}
	return out
}

// PreparedCount is how many sentences have at least one simplified level cached.
func (s *Session) PreparedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, levels := range s.prepared {
		if len(levels) > 0 {
			n++
		}
	}
	return n
}

type View struct {
	Index       int    `json:"index"`
	Total       int    `json:"total"`
	Level       int    `json:"level"`
	MaxLevel    int    `json:"maxLevel"`
	Sentence       string `json:"sentence"`
	Original       string `json:"original"`
	Previous       string `json:"previous"`       // version one level above current (for compare after ↓)
	PreviousLevel  int    `json:"previousLevel"`  // 0 = original
	CanSimplify    bool   `json:"canSimplify"`
	CanGoHarder bool   `json:"canGoHarder"`
	Preparing   bool   `json:"preparing"`
	Ready       bool   `json:"ready"`
	PrepStatus  string `json:"prepStatus"`
	PrepActive  bool   `json:"prepActive"`
	AtEnd       bool   `json:"atEnd"`

	BookRewriteActive bool `json:"bookRewriteActive"`
	BookRewriteDone   int  `json:"bookRewriteDone"`
	BookRewriteTotal  int  `json:"bookRewriteTotal"`
}

func (s *Session) View() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.viewLocked()
}

// PrepareSentence starts background LLM only when idx is the active sentence.
func (s *Session) PrepareSentence(ctx context.Context, llm *simplify.Client, idx int) {
	s.mu.Lock()
	if idx < 0 || idx >= len(s.Sentences) || idx != s.Index {
		s.mu.Unlock()
		return
	}
	if levels, ok := s.prepared[idx]; ok && len(levels) > 0 {
		s.mu.Unlock()
		return
	}
	if _, busy := s.inFlight[idx]; busy {
		s.mu.Unlock()
		return
	}
	original := s.Sentences[idx]
	s.mu.Unlock()

	_ = s.fetchLevels(ctx, llm, idx, original)
}

// PrepareWhileReading preloads simplifications for the current sentence only.
func (s *Session) PrepareWhileReading(ctx context.Context, llm *simplify.Client) {
	s.mu.Lock()
	cur := s.Index
	s.mu.Unlock()
	s.PrepareSentence(ctx, llm, cur)
}

func (s *Session) fetchLevels(ctx context.Context, llm *simplify.Client, idx int, original string) error {
	s.mu.Lock()
	if levels, ok := s.prepared[idx]; ok && len(levels) > 0 {
		s.mu.Unlock()
		return nil
	}
	if ch, ok := s.inFlight[idx]; ok {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
			return nil
		}
	}
	done := make(chan struct{})
	s.inFlight[idx] = done
	cur := s.Index
	s.mu.Unlock()

	levels, _, err := llm.SimplifyAllLevels(ctx, original)

	s.mu.Lock()
	delete(s.inFlight, idx)
	close(done)
	stale := idx != s.Index || idx != cur || idx < 0 || idx >= len(s.Sentences) || s.Sentences[idx] != original
	if err != nil {
		s.mu.Unlock()
		if !stale {
			log.Printf("prepare sentence %d: %v", idx, err)
		}
		return err
	}
	if stale {
		s.mu.Unlock()
		return nil
	}
	m := make(map[int]string)
	for i, t := range levels {
		if i >= MaxLevel {
			break
		}
		m[i+1] = t
	}
	if len(m) > 0 {
		s.prepared[idx] = m
		for k, v := range m {
			s.Cache[k] = v
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Session) ensureLevels(ctx context.Context, llm *simplify.Client, original string) error {
	s.mu.Lock()
	idx := s.Index
	if len(s.Cache) >= 1 {
		s.mu.Unlock()
		return nil
	}
	if levels, ok := s.prepared[idx]; ok && len(levels) > 0 {
		for k, v := range levels {
			s.Cache[k] = v
		}
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	return s.fetchLevels(ctx, llm, idx, original)
}

func (s *Session) Easier(ctx context.Context, llm *simplify.Client) (View, error) {
	s.mu.Lock()
	if s.Level >= MaxLevel {
		v := s.viewLocked()
		s.mu.Unlock()
		return v, nil
	}
	next := s.Level + 1
	if cached, ok := s.Cache[next]; ok {
		prev := s.Current
		s.Level = next
		s.Current = cached
		v := s.viewLocked()
		v.Previous = prev
		if next == 1 {
			v.PreviousLevel = 0
		} else {
			v.PreviousLevel = next - 1
		}
		s.mu.Unlock()
		return v, nil
	}
	original := s.Original
	s.mu.Unlock()

	if err := s.ensureLevels(ctx, llm, original); err != nil {
		return View{}, err
	}

	s.mu.Lock()
	prev := s.Current
	s.Level = next
	if t, ok := s.Cache[next]; ok {
		s.Current = t
	}
	v := s.viewLocked()
	v.Previous = prev
	if next == 1 {
		v.PreviousLevel = 0
	} else {
		v.PreviousLevel = next - 1
	}
	s.mu.Unlock()
	return v, nil
}

func (s *Session) Harder() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Level <= 0 {
		return s.viewLocked()
	}
	left := s.Current
	s.Level--
	if s.Level == 0 {
		s.Current = s.Original
	} else if t, ok := s.Cache[s.Level]; ok {
		s.Current = t
	}
	v := s.viewLocked()
	v.Previous = left
	if s.Level == 0 {
		v.PreviousLevel = 1
	} else {
		v.PreviousLevel = s.Level + 1
	}
	return v
}

func (s *Session) Next() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Index+1 >= len(s.Sentences) {
		return s.viewLocked()
	}
	s.Index++
	s.applySentenceLocked(s.Index)
	return s.viewLocked()
}

func (s *Session) Prev() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Index == 0 {
		return s.viewLocked()
	}
	s.Index--
	s.applySentenceLocked(s.Index)
	return s.viewLocked()
}

func (s *Session) applySentenceLocked(idx int) {
	if s.BookID == "" {
		s.evictPreparedExcept(idx)
	}
	s.Level = 0
	s.Original = s.Sentences[idx]
	s.Current = s.Original
	s.Cache = make(map[int]string)
	if levels, ok := s.prepared[idx]; ok {
		for k, v := range levels {
			s.Cache[k] = v
		}
	}
}

func (s *Session) evictPreparedExcept(keep int) {
	for i := range s.prepared {
		if i != keep {
			delete(s.prepared, i)
		}
	}
	// Drop tracking for other sentences; in-flight LLM work is ignored when it finishes.
	for i := range s.inFlight {
		if i != keep {
			delete(s.inFlight, i)
		}
	}
}

func (s *Session) viewLocked() View {
	ready := len(s.Cache) > 0
	if !ready {
		if levels, ok := s.prepared[s.Index]; ok {
			ready = len(levels) > 0
		}
	}
	_, preparing := s.inFlight[s.Index]
	preparing = preparing && !ready
	prepStatus, prepActive := s.prepStatusLocked(ready)
	prev, prevLvl := s.compareLocked()
	return View{
		Index:          s.Index,
		Total:          len(s.Sentences),
		Level:          s.Level,
		MaxLevel:       MaxLevel,
		Sentence:       s.Current,
		Original:       s.Original,
		Previous:       prev,
		PreviousLevel:  prevLvl,
		CanSimplify:    s.Level < MaxLevel,
		CanGoHarder:    s.Level > 0,
		Preparing:      preparing,
		Ready:          ready,
		PrepStatus:     prepStatus,
		PrepActive:     prepActive,
		AtEnd:          s.Index >= len(s.Sentences)-1,
	}
}

func (s *Session) prepStatusLocked(curReady bool) (string, bool) {
	_, curBusy := s.inFlight[s.Index]

	if curBusy {
		return "Preparing 3 easier versions for this sentence (Gemma 4)…", true
	}
	if !curReady {
		return "Preparing easier versions for this sentence…", true
	}
	return "Ready — press ↓ for easier versions", false
}

func (s *Session) compareLocked() (string, int) {
	if s.Level <= 0 {
		return "", 0
	}
	if s.Level == 1 {
		return s.Original, 0
	}
	if t, ok := s.Cache[s.Level-1]; ok {
		return t, s.Level - 1
	}
	return s.Original, 0
}
