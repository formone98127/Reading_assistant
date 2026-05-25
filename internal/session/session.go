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
	mu          sync.Mutex
	Sentences   []string
	Index       int
	Level       int
	Original    string
	Current     string
	Cache       map[int]string // levels 1-3 for current sentence
	ReadingMode    string
	Chinese        string // translation for current sentence
	chineseVisible bool   // EN+中文: false = original only, true = show 中文

	SourceDir  string
	SourceName string
	BookID     string

	// prepared[idx] = simplified English levels (1–3), separate from Chinese
	prepared map[int]map[int]string
	chinese  map[int]string
	inFlight map[int]chan struct{}
	zhFlight map[int]chan struct{}
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
		Sentences:   sentences,
		Cache:       make(map[int]string),
		ReadingMode: ModeEnglish,
		prepared:    make(map[int]map[int]string),
		chinese:     make(map[int]string),
		inFlight:    make(map[int]chan struct{}),
		zhFlight:    make(map[int]chan struct{}),
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

func (s *Session) SetReadingMode(mode string) {
	s.mu.Lock()
	s.ReadingMode = NormalizeReadingMode(mode)
	s.Level = 0
	s.Current = s.Original
	s.chineseVisible = false
	s.mu.Unlock()
}

func (s *Session) ReadingModeValue() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return NormalizeReadingMode(s.ReadingMode)
}

func (s *Session) BookIDValue() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.BookID
}

// RefreshPrepared merges English levels and Chinese from disk.
func (s *Session) RefreshPrepared(english map[int]map[int]string, chinese map[int]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx, levels := range english {
		cp := make(map[int]string, len(levels))
		for k, v := range levels {
			cp[k] = v
		}
		s.prepared[idx] = cp
	}
	for idx, text := range chinese {
		if strings.TrimSpace(text) != "" {
			s.chinese[idx] = text
		}
	}
	idx := s.Index
	s.Cache = make(map[int]string)
	if levels, ok := s.prepared[idx]; ok {
		for k, v := range levels {
			s.Cache[k] = v
		}
	}
	s.Chinese = s.chinese[idx]
	if s.Level > 0 {
		if t, ok := s.Cache[s.Level]; ok {
			s.Current = t
		} else {
			s.Level = 0
			s.Current = s.Original
		}
	}
}

// SeedPrepared loads cached English levels and Chinese (e.g. from library bundle).
func (s *Session) SeedPrepared(english map[int]map[int]string, chinese map[int]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx, levels := range english {
		cp := make(map[int]string, len(levels))
		for k, v := range levels {
			cp[k] = v
		}
		s.prepared[idx] = cp
	}
	for idx, text := range chinese {
		if strings.TrimSpace(text) != "" {
			s.chinese[idx] = text
		}
	}
	if levels, ok := s.prepared[s.Index]; ok {
		for k, v := range levels {
			s.Cache[k] = v
		}
	}
	s.Chinese = s.chinese[s.Index]
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
	s.Chinese = s.chinese[index]
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

// ChineseSnapshot returns a copy of cached Chinese translations.
func (s *Session) ChineseSnapshot() map[int]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int]string, len(s.chinese))
	for idx, text := range s.chinese {
		if strings.TrimSpace(text) != "" {
			out[idx] = text
		}
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

	ReadingMode      string `json:"readingMode"`
	ShowChinese      bool   `json:"showChinese"`
	ChineseVisible   bool   `json:"chineseVisible"`
	Chinese          string `json:"chinese,omitempty"`
	ChineseReady     bool   `json:"chineseReady"`
	ChinesePreparing bool   `json:"chinesePreparing"`

	BookRewriteActive bool `json:"bookRewriteActive"`
	BookRewriteDone   int  `json:"bookRewriteDone"`
	BookRewriteTotal  int  `json:"bookRewriteTotal"`
	BookChineseDone   int  `json:"bookChineseDone"`
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

// PrepareWhileReading preloads English easier levels for the current sentence (paste only).
// 中文 while reading comes from library chinese.json only (not mixed with english.json).
func (s *Session) PrepareWhileReading(ctx context.Context, llm *simplify.Client) {
	s.mu.Lock()
	cur := s.Index
	mode := NormalizeReadingMode(s.ReadingMode)
	bookID := s.BookID
	s.mu.Unlock()
	if ChineseEnabled(mode) || bookID != "" {
		return
	}
	s.PrepareSentence(ctx, llm, cur)
}

// PrepareChinese starts background translation when idx is the active sentence.
func (s *Session) PrepareChinese(ctx context.Context, llm *simplify.Client, idx int) {
	s.mu.Lock()
	if !ChineseEnabled(s.ReadingMode) || idx < 0 || idx >= len(s.Sentences) || idx != s.Index {
		s.mu.Unlock()
		return
	}
	if t, ok := s.chinese[idx]; ok && strings.TrimSpace(t) != "" {
		s.mu.Unlock()
		return
	}
	if _, busy := s.zhFlight[idx]; busy {
		s.mu.Unlock()
		return
	}
	original := s.Sentences[idx]
	s.mu.Unlock()
	_ = s.fetchChinese(ctx, llm, idx, original)
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

func (s *Session) fetchChinese(ctx context.Context, llm *simplify.Client, idx int, original string) error {
	s.mu.Lock()
	if !ChineseEnabled(s.ReadingMode) {
		s.mu.Unlock()
		return nil
	}
	if t, ok := s.chinese[idx]; ok && strings.TrimSpace(t) != "" {
		s.mu.Unlock()
		return nil
	}
	if ch, ok := s.zhFlight[idx]; ok {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
			return nil
		}
	}
	done := make(chan struct{})
	s.zhFlight[idx] = done
	cur := s.Index
	s.mu.Unlock()

	text, err := llm.TranslateChinese(ctx, original)

	s.mu.Lock()
	delete(s.zhFlight, idx)
	close(done)
	stale := idx != s.Index || idx != cur || idx < 0 || idx >= len(s.Sentences) || s.Sentences[idx] != original
	if err != nil {
		s.mu.Unlock()
		if !stale {
			log.Printf("prepare chinese %d: %v", idx, err)
		}
		return err
	}
	if stale {
		s.mu.Unlock()
		return nil
	}
	s.chinese[idx] = text
	if idx == s.Index {
		s.Chinese = text
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
	if ChineseEnabled(s.ReadingMode) {
		s.chineseVisible = true
		v := s.viewLocked()
		s.mu.Unlock()
		return v, nil
	}
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
	if ChineseEnabled(s.ReadingMode) {
		s.chineseVisible = false
		return s.viewLocked()
	}
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
		s.evictChineseExcept(idx)
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
	s.Chinese = s.chinese[idx]
	s.chineseVisible = false
}

func (s *Session) evictChineseExcept(keep int) {
	for i := range s.chinese {
		if i != keep {
			delete(s.chinese, i)
		}
	}
	for i := range s.zhFlight {
		if i != keep {
			delete(s.zhFlight, i)
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
	mode := NormalizeReadingMode(s.ReadingMode)
	showZh := ChineseEnabled(mode)

	var ready bool
	var preparing bool
	if showZh {
		ch := strings.TrimSpace(s.chinese[s.Index])
		ready = ch != ""
		_, zhBusy := s.zhFlight[s.Index]
		preparing = zhBusy && !ready
	} else {
		ready = len(s.Cache) > 0
		if !ready {
			if levels, ok := s.prepared[s.Index]; ok {
				ready = len(levels) > 0
			}
		}
		_, curBusy := s.inFlight[s.Index]
		preparing = curBusy && !ready
	}

	prepStatus, prepActive := s.prepStatusLocked(ready)
	prev, prevLvl := s.compareLocked()

	level := s.Level
	current := s.Current
	if showZh {
		level = 0
		current = s.Original
	}

	chinese := s.Chinese
	chReady := strings.TrimSpace(chinese) != ""
	if !chReady {
		if t, ok := s.chinese[s.Index]; ok {
			chinese = t
			chReady = strings.TrimSpace(t) != ""
		}
	}
	_, zhBusy := s.zhFlight[s.Index]
	zhPreparing := showZh && zhBusy && !chReady

	var canSimplify, canGoHarder bool
	if showZh {
		// → reveals 中文 under original (even while translation is still loading).
		canSimplify = !s.chineseVisible
		canGoHarder = s.chineseVisible
	} else {
		canSimplify = level < MaxLevel
		canGoHarder = level > 0
	}

	return View{
		Index:            s.Index,
		Total:            len(s.Sentences),
		Level:            level,
		MaxLevel:         MaxLevel,
		Sentence:         current,
		Original:         s.Original,
		Previous:         prev,
		PreviousLevel:    prevLvl,
		CanSimplify:      canSimplify,
		CanGoHarder:      canGoHarder,
		Preparing:        preparing,
		Ready:            ready,
		PrepStatus:       prepStatus,
		PrepActive:       prepActive,
		AtEnd:            s.Index >= len(s.Sentences)-1,
		ReadingMode:      mode,
		ShowChinese:      showZh,
		ChineseVisible:   showZh && s.chineseVisible,
		Chinese:          chinese,
		ChineseReady:     chReady,
		ChinesePreparing: zhPreparing,
	}
}

func (s *Session) prepStatusLocked(curReady bool) (string, bool) {
	_, curBusy := s.inFlight[s.Index]
	showZh := ChineseEnabled(s.ReadingMode)
	chReady := strings.TrimSpace(s.chinese[s.Index]) != ""

	if showZh {
		if !chReady {
			return "Press → for 中文 when library rewrite has this sentence", false
		}
		if s.chineseVisible {
			return "English + 中文 — ← original only · ↓↑ change sentence", false
		}
		return "Original only — press → for English + 中文 · ↓↑ change sentence", false
	}
	if curBusy {
		return "Preparing 3 easier versions for this sentence (Gemma 4)…", true
	}
	if !curReady {
		return "Preparing easier versions for this sentence…", true
	}
	return "Ready — press → for easier versions", false
}

func (s *Session) compareLocked() (string, int) {
	if ChineseEnabled(s.ReadingMode) {
		return "", 0
	}
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
