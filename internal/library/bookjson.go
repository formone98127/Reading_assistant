package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reading-assistant/internal/save"
	"reading-assistant/internal/session"
)

const bookFile = "book.json"

// BookFile is the canonical on-disk rewrite bundle for one library book.
type BookFile struct {
	Version   int                   `json:"version"`
	Title     string                `json:"title"`
	Sentences []save.ReaderSentence `json:"sentences"`
}

func (s *Store) bookPath(id string) string {
	return filepath.Join(s.bookDir(id), bookFile)
}

// LoadBook reads book.json, migrating legacy sentences.json + english.json + chinese.json when needed.
func (s *Store) LoadBook(id string) (BookFile, error) {
	if err := validateBookID(id); err != nil {
		return BookFile{}, err
	}
	var book BookFile
	err := readJSON(s.bookPath(id), &book)
	if err == nil && len(book.Sentences) > 0 {
		return book, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return BookFile{}, err
	}
	return s.migrateLegacyBook(id)
}

func (s *Store) saveBook(id string, book BookFile) error {
	if book.Version == 0 {
		book.Version = 1
	}
	return writeJSON(s.bookPath(id), book)
}

func (s *Store) migrateLegacyBook(id string) (BookFile, error) {
	var meta Meta
	if err := readJSON(filepath.Join(s.bookDir(id), "meta.json"), &meta); err != nil {
		return BookFile{}, err
	}
	sentences, err := s.loadLegacySentences(id)
	if err != nil {
		return BookFile{}, err
	}
	_ = s.migrateLegacyLevels(id)
	english, err := s.loadEnglishLevels(id)
	if err != nil {
		return BookFile{}, err
	}
	chinese, err := s.loadChinese(id)
	if err != nil {
		return BookFile{}, err
	}
	book := BookFile{
		Version:   1,
		Title:     meta.Title,
		Sentences: save.BuildReaderSentences(sentences, english, chinese, session.MaxLevel),
	}
	if err := s.saveBook(id, book); err != nil {
		return BookFile{}, err
	}
	return book, nil
}

func (s *Store) loadLegacySentences(id string) ([]string, error) {
	var sentences []string
	if err := readJSON(filepath.Join(s.bookDir(id), "sentences.json"), &sentences); err != nil {
		return nil, err
	}
	return sentences, nil
}

func newBookFile(title string, sentences []string) BookFile {
	out := make([]save.ReaderSentence, len(sentences))
	for i, orig := range sentences {
		out[i] = save.ReaderSentence{
			Original: orig,
			Levels:   make([]string, session.MaxLevel),
		}
	}
	return BookFile{
		Version:   1,
		Title:     title,
		Sentences: out,
	}
}

func bookToPrepared(book BookFile) PreparedData {
	english := map[int]map[int]string{}
	chinese := map[int]string{}
	for i, sent := range book.Sentences {
		lv := map[int]string{}
		for j, t := range sent.Levels {
			if strings.TrimSpace(t) != "" {
				lv[j+1] = t
			}
		}
		if len(lv) > 0 {
			english[i] = lv
		}
		if strings.TrimSpace(sent.Chinese) != "" {
			chinese[i] = sent.Chinese
		}
	}
	return PreparedData{English: english, Chinese: chinese}
}

func bookOriginals(book BookFile) []string {
	out := make([]string, len(book.Sentences))
	for i, sent := range book.Sentences {
		out[i] = sent.Original
	}
	return out
}

func (s *Store) updateBookSentence(id string, idx int, fn func(*save.ReaderSentence) error) error {
	book, err := s.LoadBook(id)
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(book.Sentences) {
		return fmt.Errorf("sentence index out of range")
	}
	if err := fn(&book.Sentences[idx]); err != nil {
		return err
	}
	return s.saveBook(id, book)
}
