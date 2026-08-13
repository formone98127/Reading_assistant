package session

import (
	"context"
	"fmt"
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
	Cache       map[int]string // current sentence: levels 1–3 easier, 4 = 中文
	ReadingMode string
	ShowEasier  bool
	ShowChinese bool
	Chinese     string // 中文 for current sentence

	SourceDir  string
	SourceName string
	BookID     string

	// levels[idx][level] — 1–3 easier English, 4 繁體中文
	levels map[int]map[int]string
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
		ShowEasier:  true,
		levels:      make(map[int]map[int]string),
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
	s.SetReadingOptions(OptionsFromMode(mode))
}

func (s *Session) SetReadingOptions(o ReadingOptions) {
	s.mu.Lock()
	s.ShowEasier = o.ShowEasier
	s.ShowChinese = o.ShowChinese
	s.ReadingMode = ModeFromOptions(o)
	s.Level = 0
	s.Current = s.Original
	s.mu.Unlock()
}

func (s *Session) ReadingOptionsValue() ReadingOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.optionsLocked()
}

func (s *Session) optionsLocked() ReadingOptions {
	return ReadingOptions{ShowEasier: s.ShowEasier, ShowChinese: s.ShowChinese}
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

// RefreshPrepared merges english.json + chinese.json into the unified level store.
func (s *Session) RefreshPrepared(english map[int]map[int]string, chinese map[int]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx, lv := range mergePreparedLevels(english, chinese) {
		s.levels[idx] = lv
	}
	s.syncCacheLocked(s.Index)
	if s.Level > 0 {
		s.applyCurrentFromLevelLocked()
	}
}

func (s *Session) syncCacheLocked(idx int) {
	s.Cache = make(map[int]string)
	if lv, ok := s.levels[idx]; ok {
		for k, v := range lv {
			s.Cache[k] = v
		}
	}
	if t, ok := s.levelTextLocked(idx, LevelChinese); ok {
		s.Chinese = t
	} else {
		s.Chinese = ""
	}
}

func (s *Session) levelTextLocked(idx, level int) (string, bool) {
	if t, ok := s.Cache[level]; ok && strings.TrimSpace(t) != "" {
		return t, true
	}
	if lv, ok := s.levels[idx]; ok {
		if t, ok := lv[level]; ok && strings.TrimSpace(t) != "" {
			return t, true
		}
	}
	return "", false
}

func (s *Session) setLevelTextLocked(idx, level int, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if s.levels[idx] == nil {
		s.levels[idx] = make(map[int]string)
	}
	s.levels[idx][level] = text
	if idx == s.Index {
		s.Cache[level] = text
		if level == LevelChinese {
			s.Chinese = text
		}
	}
}

// applyCurrentFromLevelLocked sets Current from the unified level index.
func (s *Session) applyCurrentFromLevelLocked() {
	if s.Level == 0 {
		s.Current = s.Original
		return
	}
	if t, ok := s.levelTextLocked(s.Index, s.Level); ok {
		s.Current = t
		if s.Level == LevelChinese {
			s.Chinese = t
		}
		return
	}
	s.Level = 0
	s.Current = s.Original
}

// SeedPrepared loads library cache into the unified level store.
func (s *Session) SeedPrepared(english map[int]map[int]string, chinese map[int]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx, lv := range mergePreparedLevels(english, chinese) {
		s.levels[idx] = lv
	}
	s.syncCacheLocked(s.Index)
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
	s.syncCacheLocked(index)
	if level > 0 {
		o := s.optionsLocked()
		level = o.NormalizeStoredLevel(level)
		if !o.levelAllowed(level) {
			level = 0
		}
		s.Level = level
		s.applyCurrentFromLevelLocked()
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
		if levels, ok := s.levels[i]; ok {
			for lv := level; lv >= 1; lv-- {
				if lv > MaxLevel {
					continue
				}
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

// TextsAtUnifiedLevel returns one string per sentence for RSVP (0=original, 1–3=easier, 4=中文 with original fallback).
func (s *Session) TextsAtUnifiedLevel(level int) []string {
	if level <= 0 {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := make([]string, len(s.Sentences))
		copy(out, s.Sentences)
		return out
	}
	if level == LevelChinese {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := make([]string, len(s.Sentences))
		for i, orig := range s.Sentences {
			text := orig
			if levels, ok := s.levels[i]; ok {
				if t, ok := levels[LevelChinese]; ok && strings.TrimSpace(t) != "" {
					text = t
				}
			}
			out[i] = text
		}
		return out
	}
	if level > MaxLevel {
		level = MaxLevel
	}
	return s.ParagraphsAtLevel(level)
}

// ChineseSnapshot returns level-4 text for library chinese.json.
func (s *Session) ChineseSnapshot() map[int]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, chinese := splitLevels(s.levels)
	return chinese
}

// PreparedSnapshot returns levels 1–3 for library english.json.
func (s *Session) PreparedSnapshot() map[int]map[int]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	english, _ := splitLevels(s.levels)
	return english
}

// PreparedCount is how many sentences have at least one easier level cached.
func (s *Session) PreparedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, levels := range s.levels {
		if englishLevelsOnly(levels) != nil {
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

	ReadingMode string `json:"readingMode"`
	Track       string `json:"track"` // NavTrack for clients
	ShowEasier  bool   `json:"showEasier"`
	ShowChinese bool   `json:"showChinese"`
	Chinese     string `json:"chinese,omitempty"`
	ChineseReady     bool   `json:"chineseReady"`
	ChinesePreparing bool   `json:"chinesePreparing"`

	BookRewriteActive bool `json:"bookRewriteActive"`
	BookRewriteDone   int  `json:"bookRewriteDone"`
	BookRewriteTotal  int  `json:"bookRewriteTotal"`
	BookChineseDone   int  `json:"bookChineseDone"`
	BookTTSEnabled    bool `json:"bookTTSEnabled"`
	BookAudioDone        int  `json:"bookAudioDone"`
	BookAudioGenerating bool `json:"bookAudioGenerating"`
	BookVoiceOnly        bool `json:"bookVoiceOnly"`
	HasAudio          bool `json:"hasAudio"`

	LevelTexts []string `json:"levelTexts,omitempty"` // index 0 = easier level 1
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
	if eng := englishLevelsOnly(s.levels[idx]); eng != nil {
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
	bookID := s.BookID
	showEasier := s.ShowEasier
	s.mu.Unlock()
	if !showEasier || bookID != "" {
		return
	}
	s.PrepareSentence(ctx, llm, cur)
}

// PrepareChinese starts background translation when idx is the active sentence.
func (s *Session) PrepareChinese(ctx context.Context, llm *simplify.Client, idx int) {
	s.mu.Lock()
	if !s.ShowChinese || idx < 0 || idx >= len(s.Sentences) || idx != s.Index {
		s.mu.Unlock()
		return
	}
	if _, ok := s.levelTextLocked(idx, LevelChinese); ok {
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
	if eng := englishLevelsOnly(s.levels[idx]); eng != nil {
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
	for lv, text := range m {
		s.setLevelTextLocked(idx, lv, text)
	}
	s.mu.Unlock()
	return nil
}

func (s *Session) fetchChinese(ctx context.Context, llm *simplify.Client, idx int, original string) error {
	s.mu.Lock()
	if !s.ShowChinese {
		s.mu.Unlock()
		return nil
	}
	if _, ok := s.levelTextLocked(idx, LevelChinese); ok {
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
	s.setLevelTextLocked(idx, LevelChinese, text)
	s.mu.Unlock()
	return nil
}

func (s *Session) ensureLevels(ctx context.Context, llm *simplify.Client, original string) error {
	s.mu.Lock()
	idx := s.Index
	if eng := englishLevelsOnly(s.levels[idx]); eng != nil {
		for k, v := range eng {
			s.Cache[k] = v
		}
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	return s.fetchLevels(ctx, llm, idx, original)
}

func (s *Session) hasEasierCachedLocked(idx int) bool {
	if eng := englishLevelsOnly(s.levels[idx]); eng != nil {
		return true
	}
	for lv := 1; lv <= MaxLevel; lv++ {
		if _, ok := s.Cache[lv]; ok {
			return true
		}
	}
	return false
}

func (s *Session) Easier(ctx context.Context, llm *simplify.Client) (View, error) {
	s.mu.Lock()
	o := s.optionsLocked()
	seq := o.NavLevels()
	next, ok := nextNavLevel(seq, s.Level)
	if !ok {
		v := s.viewLocked()
		s.mu.Unlock()
		return v, nil
	}
	if t, ok := s.levelTextLocked(s.Index, next); ok {
		s.Level = next
		s.Current = t
		if next == LevelChinese {
			s.Chinese = t
		}
		v := s.viewLocked()
		s.mu.Unlock()
		return v, nil
	}
	idx := s.Index
	original := s.Original
	bookID := s.BookID
	needZh := next == LevelChinese
	needEng := next >= 1 && next <= MaxLevel
	s.mu.Unlock()

	if needEng {
		if err := s.ensureLevels(ctx, llm, original); err != nil {
			return View{}, err
		}
	}
	if needZh && bookID == "" {
		_ = s.fetchChinese(ctx, llm, idx, original)
	}

	s.mu.Lock()
	if t, ok := s.levelTextLocked(s.Index, next); ok {
		s.Level = next
		s.Current = t
		if next == LevelChinese {
			s.Chinese = t
		}
		v := s.viewLocked()
		s.mu.Unlock()
		return v, nil
	}
	v := s.viewLocked()
	s.mu.Unlock()
	return v, nil
}

func (s *Session) Harder() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.optionsLocked()
	if s.Level <= 0 {
		return s.viewLocked()
	}
	seq := o.NavLevels()
	prev, ok := prevNavLevel(seq, s.Level)
	if !ok {
		return s.viewLocked()
	}
	s.Level = prev
	if prev == 0 {
		s.Current = s.Original
	} else if t, ok := s.levelTextLocked(s.Index, prev); ok {
		s.Current = t
		if prev == LevelChinese {
			s.Chinese = t
		}
	} else {
		s.Current = s.Original
		s.Level = 0
	}
	return s.viewLocked()
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

// GoTo jumps to a sentence index (0-based). Resets to original level like Next/Prev.
func (s *Session) GoTo(index int) View {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Sentences) == 0 {
		return s.viewLocked()
	}
	if index < 0 {
		index = 0
	}
	if index >= len(s.Sentences) {
		index = len(s.Sentences) - 1
	}
	if index != s.Index {
		s.Index = index
		s.applySentenceLocked(s.Index)
	}
	return s.viewLocked()
}

func (s *Session) applySentenceLocked(idx int) {
	if s.BookID == "" {
		s.evictLevelsExcept(idx)
	}
	s.Level = 0
	s.Original = s.Sentences[idx]
	s.Current = s.Original
	s.syncCacheLocked(idx)
}

func (s *Session) evictLevelsExcept(keep int) {
	for i := range s.levels {
		if i != keep {
			delete(s.levels, i)
		}
	}
	for i := range s.inFlight {
		if i != keep {
			delete(s.inFlight, i)
		}
	}
	for i := range s.zhFlight {
		if i != keep {
			delete(s.zhFlight, i)
		}
	}
}

func (s *Session) viewLocked() View {
	o := s.optionsLocked()
	nav := o.NavTrack()
	mode := ModeFromOptions(o)
	maxLvl := o.MaxLevel()
	seq := o.NavLevels()

	chinese := s.Chinese
	if t, ok := s.levelTextLocked(s.Index, LevelChinese); ok {
		chinese = t
	}
	chReady := chinese != ""
	_, zhBusy := s.zhFlight[s.Index]
	zhPreparing := o.ShowChinese && zhBusy && !chReady
	engReady := s.hasEasierCachedLocked(s.Index)
	_, engBusy := s.inFlight[s.Index]

	var ready, preparing bool
	switch nav {
	case TrackChinese:
		ready = chReady
		preparing = zhPreparing
	case TrackEasier:
		ready = engReady
		preparing = engBusy && !engReady
	default:
		ready = true
	}

	prepStatus, prepActive := s.prepStatusLocked(ready, chReady)
	prev, prevLvl := s.compareLocked()

	level := s.Level
	current := s.Current
	next, hasNext := nextNavLevel(seq, level)
	canSimplify := false
	if hasNext {
		if _, ok := s.levelTextLocked(s.Index, next); ok {
			canSimplify = true
		} else if next == LevelChinese {
			canSimplify = chReady
		} else if o.ShowEasier {
			canSimplify = engReady || engBusy
		}
	}
	canGoHarder := level > 0

	var levelTexts []string
	for lv := 1; lv <= MaxLevel; lv++ {
		if t, ok := s.levelTextLocked(s.Index, lv); ok && strings.TrimSpace(t) != "" {
			levelTexts = append(levelTexts, t)
		}
	}

	return View{
		Index:            s.Index,
		Total:            len(s.Sentences),
		Level:            level,
		MaxLevel:         maxLvl,
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
		Track:            nav,
		ShowEasier:       o.ShowEasier,
		ShowChinese:      o.ShowChinese,
		Chinese:          chinese,
		ChineseReady:     chReady,
		ChinesePreparing: zhPreparing,
		LevelTexts:       levelTexts,
	}
}

func (s *Session) prepStatusLocked(engReady, chReady bool) (string, bool) {
	_, curBusy := s.inFlight[s.Index]
	o := s.optionsLocked()

	if s.Level == LevelChinese {
		return "中文 — ← previous · ↓↑ change sentence", false
	}
	if s.Level > 0 && s.Level <= MaxLevel {
		seq := o.NavLevels()
		next, hasNext := nextNavLevel(seq, s.Level)
		if hasNext && next == LevelChinese {
			if chReady {
				return fmt.Sprintf("Easier %d — press → for 中文 · ↓↑ change sentence", s.Level), false
			}
			return fmt.Sprintf("Easier %d — 中文 not ready yet · ↓↑ change sentence", s.Level), false
		}
		return fmt.Sprintf("Easier %d — press → for next · ↓↑ change sentence", s.Level), false
	}

	switch o.NavTrack() {
	case TrackChinese:
		if !chReady {
			return "Press → for 中文 when rewrite has this sentence", false
		}
		return "Original — press → for 中文 · ↓↑ change sentence", false
	case TrackEasier:
		if curBusy || !engReady {
			return "", false
		}
		return "", false
	default:
		return "Original only — ↓↑ change sentence", false
	}
}

func (s *Session) compareLocked() (string, int) {
	if s.Level <= 0 {
		return "", 0
	}
	return s.Original, 0
}
