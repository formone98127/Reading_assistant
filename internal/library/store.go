package library

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	SourceFile     string    `json:"sourceFile"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	TotalSentences int       `json:"totalSentences"`
	RewriteDone    int       `json:"rewriteDone"`
	RewriteStatus  string    `json:"rewriteStatus"`
	RewriteError   string    `json:"rewriteError,omitempty"`
	ReadIndex      int       `json:"readIndex"`
	ReadLevel           int       `json:"readLevel"`
	ReadingMode         string    `json:"readingMode,omitempty"`
	ChineseRewriteDone  int       `json:"chineseRewriteDone,omitempty"`
}

// Store manages the on-disk book library.
// English easier levels: english.json. 繁體 中文: chinese.json (separate files).
type Store struct {
	Root string
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

// Import saves a new book bundle (source file + sentences).
func (s *Store) Import(title, sourceFilename string, sourceData []byte, sentences []string, readingMode string) (*Meta, error) {
	if len(sentences) == 0 {
		return nil, fmt.Errorf("no sentences")
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
	if err := writeJSON(filepath.Join(dir, "sentences.json"), sentences); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(dir, englishLevelsFile), map[string]map[string]string{}); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(dir, chineseFile), map[string]string{}); err != nil {
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
		ReadingMode: session.NormalizeReadingMode(readingMode),
	}
	if err := s.saveMeta(meta); err != nil {
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
	data, err := os.ReadFile(filepath.Join(s.bookDir(id), "meta.json"))
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) saveMeta(m *Meta) error {
	m.UpdatedAt = time.Now().UTC()
	return writeJSON(filepath.Join(s.bookDir(m.ID), "meta.json"), m)
}

func (s *Store) LoadSentences(id string) ([]string, error) {
	var sentences []string
	if err := readJSON(filepath.Join(s.bookDir(id), "sentences.json"), &sentences); err != nil {
		return nil, err
	}
	return sentences, nil
}

// LoadPrepared returns English (english.json) and 中文 (chinese.json).
func (s *Store) LoadPrepared(id string) (PreparedData, error) {
	_ = s.migrateLegacyLevels(id)
	english, err := s.loadEnglishLevels(id)
	if err != nil {
		return PreparedData{}, err
	}
	chinese, err := s.loadChinese(id)
	if err != nil {
		return PreparedData{}, err
	}
	if english == nil {
		english = map[int]map[int]string{}
	}
	if chinese == nil {
		chinese = map[int]string{}
	}
	return PreparedData{English: english, Chinese: chinese}, nil
}

func (s *Store) SaveSentenceLevels(id string, idx int, levels map[int]string) error {
	raw := map[string]map[string]string{}
	path := s.englishLevelsPath(id)
	_ = readJSON(path, &raw)
	key := fmt.Sprintf("%d", idx)
	out := make(map[string]string)
	for lv, txt := range levels {
		out[fmt.Sprintf("%d", lv)] = txt
	}
	raw[key] = out
	return writeJSON(path, raw)
}

func (s *Store) SaveChinese(id string, idx int, text string) error {
	raw := map[string]string{}
	path := s.chinesePath(id)
	_ = readJSON(path, &raw)
	key := fmt.Sprintf("%d", idx)
	if strings.TrimSpace(text) == "" {
		delete(raw, key)
	} else {
		raw[key] = text
	}
	return writeJSON(path, raw)
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
	m, err := s.LoadMeta(id)
	if err != nil {
		return err
	}
	m.ReadingMode = session.NormalizeReadingMode(mode)
	return s.saveMeta(m)
}

func (s *Store) SetChineseProgress(id string, done int) error {
	m, err := s.LoadMeta(id)
	if err != nil {
		return err
	}
	m.ChineseRewriteDone = done
	if m.TotalSentences > 0 && done < m.TotalSentences && m.RewriteDone >= m.TotalSentences {
		m.RewriteStatus = StatusRewriting
	}
	return s.saveMeta(m)
}

func (s *Store) SetRewriteProgress(id string, done, total int, status, errMsg string) error {
	m, err := s.LoadMeta(id)
	if err != nil {
		return err
	}
	m.RewriteDone = done
	m.TotalSentences = total
	m.RewriteStatus = status
	m.RewriteError = errMsg
	return s.saveMeta(m)
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

func (s *Store) SaveReadPosition(id string, index, level int) error {
	m, err := s.LoadMeta(id)
	if err != nil {
		return err
	}
	m.ReadIndex = index
	m.ReadLevel = level
	return s.saveMeta(m)
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
