package library

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reading-assistant/internal/save"
	"reading-assistant/internal/session"
)

const (
	StatusPending   = "pending"
	StatusRewriting = "rewriting"
	StatusDone      = "done"
	StatusError     = "error"
)

// Meta describes a saved book bundle on disk.
type Meta struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	SourceFile         string    `json:"sourceFile"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	TotalSentences     int       `json:"totalSentences"`
	RewriteDone        int       `json:"rewriteDone"`
	RewriteStatus      string    `json:"rewriteStatus"`
	RewriteError       string    `json:"rewriteError,omitempty"`
	ReadIndex          int       `json:"readIndex"`
	ReadLevel          int       `json:"readLevel"`
	ReadingMode        string    `json:"readingMode,omitempty"`
	ShowEasier         bool      `json:"showEasier"`
	ShowChinese        bool      `json:"showChinese"`
	ChineseRewriteDone int       `json:"chineseRewriteDone,omitempty"`
	AudioRewriteDone   int       `json:"audioRewriteDone,omitempty"`
	AudioGenerating    bool      `json:"audioGenerating,omitempty"`
	TTSEnabled         bool      `json:"ttsEnabled,omitempty"`
	VoiceOnly          bool      `json:"voiceOnly,omitempty"`
}

// ResolvedOptions returns checkbox flags, migrating legacy readingMode when needed.
func (m *Meta) ResolvedOptions() session.ReadingOptions {
	if m == nil {
		return session.ReadingOptions{ShowEasier: true}
	}
	if m.ShowEasier || m.ShowChinese {
		return session.ReadingOptions{ShowEasier: m.ShowEasier, ShowChinese: m.ShowChinese}
	}
	return session.OptionsFromMode(m.ReadingMode)
}

// Store manages the on-disk book library.
// Rewrite data: book.json (original + easier levels + 中文 per sentence).
// Reading position: progress.json. Catalog: meta.json.
type Store struct {
	Root  string
	locks sync.Map // book id -> *sync.Mutex
}

func (s *Store) bookLock(id string) *sync.Mutex {
	v, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func NewStore(root string) *Store {
	_ = os.MkdirAll(filepath.Join(root, "library"), 0o755)
	return &Store{Root: filepath.Join(root, "library")}
}

func (s *Store) bookDir(id string) string {
	return filepath.Join(s.Root, id)
}

// Dir is the bundle folder for a book.
func (s *Store) Dir(id string) string {
	return s.bookDir(id)
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Import saves a new book bundle (legacy readingMode string).
func (s *Store) Import(title, sourceFilename string, sourceData []byte, sentences []string, readingMode string) (*Meta, error) {
	return s.ImportWithOptions(title, sourceFilename, sourceData, sentences, session.OptionsFromMode(readingMode), false, false)
}

// ImportWithOptions saves a new book bundle with Easier / 中文 display flags.
func (s *Store) ImportWithOptions(title, sourceFilename string, sourceData []byte, sentences []string, opts session.ReadingOptions, ttsEnabled, voiceOnly bool) (*Meta, error) {
	if len(sentences) == 0 {
		return nil, fmt.Errorf("no sentences")
	}
	if voiceOnly {
		ttsEnabled = true
		opts = session.ReadingOptions{}
	}
	id := newID()
	dir := s.bookDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	ext := filepath.Ext(sourceFilename)
	if ext == "" {
		ext = ".txt"
	}
	srcName := "source" + ext
	if err := os.WriteFile(filepath.Join(dir, srcName), sourceData, 0o644); err != nil {
		return nil, err
	}
	if err := s.saveBook(id, newBookFile(title, sentences)); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	meta := &Meta{
		ID:             id,
		Title:          title,
		SourceFile:     srcName,
		CreatedAt:      now,
		UpdatedAt:      now,
		TotalSentences: len(sentences),
		RewriteStatus:  StatusPending,
		ReadingMode:    session.ModeFromOptions(opts),
		ShowEasier:     opts.ShowEasier,
		ShowChinese:    opts.ShowChinese,
		TTSEnabled:     ttsEnabled,
		VoiceOnly:      voiceOnly,
	}
	if err := s.saveMeta(meta); err != nil {
		return nil, err
	}
	if err := s.SaveReadProgress(id, ReadProgress{
		ShowEasier:  opts.ShowEasier,
		ShowChinese: opts.ShowChinese,
	}); err != nil {
		return nil, err
	}
	if _, err := save.WriteOriginal(dir, "book", sentences); err != nil {
		return nil, err
	}
	if _, err := save.WriteRewritten(dir, "book", sentences); err != nil {
		return nil, err
	}
	return meta, nil
}

// List returns all books, newest first.
func (s *Store) List() ([]Meta, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Meta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := s.LoadMeta(e.Name())
		if err != nil {
			continue
		}
		out = append(out, *m)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].UpdatedAt.After(out[i].UpdatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (s *Store) LoadMeta(id string) (*Meta, error) {
	if err := validateBookID(id); err != nil {
		return nil, err
	}
	s.bookLock(id).Lock()
	defer s.bookLock(id).Unlock()
	m, err := s.readMeta(id)
	if err != nil {
		return nil, err
	}
	_ = s.reconcileMetaProgress(&m)
	return &m, nil
}

func (s *Store) readMeta(id string) (Meta, error) {
	data, err := os.ReadFile(filepath.Join(s.bookDir(id), "meta.json"))
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		// Repair: scan for trailing garbage after first complete JSON object.
		if fixed := salvageMetaJSON(data); fixed != nil {
			if err2 := json.Unmarshal(fixed, &m); err2 == nil {
				_ = os.WriteFile(filepath.Join(s.bookDir(id), "meta.json"), fixed, 0o644)
				return m, nil
			}
		}
		return Meta{}, err
	}
	return m, nil
}

// salvageMetaJSON strips garbage bytes trailing the first complete top-level JSON object.
func salvageMetaJSON(raw []byte) []byte {
	// Find the closing } of the first top-level object by tracking depth.
	depth := 0
	end := -1
	for i, b := range raw {
		switch b {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
				goto out
			}
		}
	}
out:
	if end <= 0 || end >= len(raw) {
		return nil
	}
	fixed := make([]byte, end)
	copy(fixed, raw[:end])
	// Re-indent so the file looks consistent
	var v any
	if err := json.Unmarshal(fixed, &v); err != nil {
		return nil
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil
	}
	out = append(out, '\n')
	return out
}

func (s *Store) saveMeta(m *Meta) error {
	if err := validateBookID(m.ID); err != nil {
		return err
	}
	s.bookLock(m.ID).Lock()
	defer s.bookLock(m.ID).Unlock()
	return s.writeMeta(m)
}

func (s *Store) writeMeta(m *Meta) error {
	m.UpdatedAt = time.Now().UTC()
	return writeJSON(filepath.Join(s.bookDir(m.ID), "meta.json"), m)
}

func (s *Store) updateMeta(id string, fn func(*Meta) error) error {
	if err := validateBookID(id); err != nil {
		return err
	}
	s.bookLock(id).Lock()
	defer s.bookLock(id).Unlock()
	m, err := s.readMeta(id)
	if err != nil {
		return err
	}
	if err := fn(&m); err != nil {
		return err
	}
	return s.writeMeta(&m)
}

// reconcileMetaProgress repairs stale progress counters from english.json/chinese.json.
// This prevents completed books from staying "busy" because meta.json missed the
// final chineseRewriteDone write.
func (s *Store) reconcileMetaProgress(m *Meta) error {
	if m == nil || m.ID == "" || m.TotalSentences <= 0 {
		return nil
	}
	_ = s.migrateLegacyLevels(m.ID)
	english, err := s.loadEnglishLevels(m.ID)
	if err != nil {
		return err
	}
	chinese, err := s.loadChinese(m.ID)
	if err != nil {
		return err
	}
	prepared := PreparedData{English: english, Chinese: chinese}
	engDone := contiguousEnglishDone(prepared, m.TotalSentences)
	zhDone := contiguousChineseDone(prepared, m.TotalSentences)

	changed := false
	if engDone > m.RewriteDone {
		m.RewriteDone = engDone
		changed = true
	}
	if zhDone > m.ChineseRewriteDone {
		m.ChineseRewriteDone = zhDone
		changed = true
	}
	if m.TTSEnabled {
		audDone := contiguousAudioDone(s, m.ID, m.TotalSentences)
		if audDone > m.AudioRewriteDone {
			m.AudioRewriteDone = audDone
			changed = true
		}
	}
	if RewriteComplete(s, m.ID, prepared, m.TotalSentences, m.TTSEnabled, m.VoiceOnly) && m.RewriteStatus != StatusDone {
		m.RewriteDone = m.TotalSentences
		m.ChineseRewriteDone = m.TotalSentences
		if m.TTSEnabled {
			m.AudioRewriteDone = contiguousAudioDone(s, m.ID, m.TotalSentences)
		}
		m.RewriteStatus = StatusDone
		m.RewriteError = ""
		changed = true
	}
	if m.RewriteStatus == StatusDone && m.ChineseRewriteDone < m.TotalSentences && zhDone >= m.TotalSentences {
		m.ChineseRewriteDone = m.TotalSentences
		changed = true
	}
	if !changed {
		return nil
	}
	return s.writeMeta(m)
}

func (s *Store) LoadSentences(id string) ([]string, error) {
	book, err := s.LoadBook(id)
	if err != nil {
		return nil, err
	}
	return bookOriginals(book), nil
}

// LoadPrepared returns easier English and 中文 from book.json.
func (s *Store) LoadPrepared(id string) (PreparedData, error) {
	book, err := s.LoadBook(id)
	if err != nil {
		return PreparedData{}, err
	}
	return bookToPrepared(book), nil
}

func (s *Store) SaveSentenceLevels(id string, idx int, levels map[int]string) error {
	return s.updateBookSentence(id, idx, func(sent *save.ReaderSentence) error {
		if len(sent.Levels) < session.MaxLevel {
			sent.Levels = make([]string, session.MaxLevel)
		}
		for lv, txt := range levels {
			if lv >= 1 && lv <= session.MaxLevel {
				sent.Levels[lv-1] = txt
			}
		}
		return nil
	})
}

func (s *Store) SaveChinese(id string, idx int, text string) error {
	return s.updateBookSentence(id, idx, func(sent *save.ReaderSentence) error {
		sent.Chinese = strings.TrimSpace(text)
		return nil
	})
}

func (s *Store) SaveRewrittenFile(id string, sentences []string, prepared PreparedData) error {
	paras := rewrittenParagraphs(sentences, prepared.English)
	_, err := save.WriteRewritten(s.bookDir(id), "book", paras)
	return err
}

func rewrittenParagraphs(sentences []string, prepared map[int]map[int]string) []string {
	out := make([]string, len(sentences))
	for i, orig := range sentences {
		text := orig
		if levels, ok := prepared[i]; ok {
			for lv := 3; lv >= 1; lv-- {
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

func (s *Store) SetReadingMode(id, mode string) error {
	return s.SetReadingOptions(id, session.OptionsFromMode(mode))
}

func (s *Store) SetReadingOptions(id string, o session.ReadingOptions) error {
	p, err := s.LoadReadProgress(id)
	if err != nil {
		return err
	}
	p.ShowEasier = o.ShowEasier
	p.ShowChinese = o.ShowChinese
	return s.SaveReadProgress(id, p)
}

func (s *Store) SetChineseProgress(id string, done int) error {
	return s.updateMeta(id, func(m *Meta) error {
		m.ChineseRewriteDone = done
		if m.TotalSentences > 0 && done < m.TotalSentences && m.RewriteDone >= m.TotalSentences {
			m.RewriteStatus = StatusRewriting
		}
		return nil
	})
}

func (s *Store) SetRewriteProgress(id string, done, total int, status, errMsg string) error {
	return s.updateMeta(id, func(m *Meta) error {
		m.RewriteDone = done
		m.TotalSentences = total
		m.RewriteStatus = status
		m.RewriteError = errMsg
		return nil
	})
}

// Delete removes a book bundle from the library.
func (s *Store) Delete(id string) error {
	if err := validateBookID(id); err != nil {
		return err
	}
	dir := s.bookDir(id)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("book not found")
		}
		return err
	}
	return os.RemoveAll(dir)
}

func validateBookID(id string) error {
	if id == "" || strings.Contains(id, "..") || strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("invalid book id")
	}
	return nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
