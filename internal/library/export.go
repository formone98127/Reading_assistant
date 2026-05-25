package library

import (
	"os"
	"path/filepath"

	"reading-assistant/internal/save"
	"reading-assistant/internal/session"
)

// HTMLExport builds a shareable interactive HTML reader for a library book.
func (s *Store) HTMLExport(bookID string) (filename string, content []byte, err error) {
	return s.HTMLExportBook(bookID, "")
}

// HTMLExportBook builds export HTML. modeOverride is optional (english / english_chinese).
func (s *Store) HTMLExportBook(bookID string, modeOverride string) (filename string, content []byte, err error) {
	meta, err := s.LoadMeta(bookID)
	if err != nil {
		return "", nil, err
	}
	sentences, err := s.LoadSentences(bookID)
	if err != nil {
		return "", nil, err
	}
	prepared, err := s.LoadPrepared(bookID)
	if err != nil {
		return "", nil, err
	}
	mode := meta.ReadingMode
	if modeOverride != "" {
		mode = session.NormalizeReadingMode(modeOverride)
	}
	sentencesOut := save.BuildReaderSentences(sentences, prepared.English, prepared.Chinese, session.MaxLevel)
	content, err = save.BuildBookHTML(save.ReaderExport{
		Title:       meta.Title,
		StartIndex:  meta.ReadIndex,
		StartLevel:  meta.ReadLevel,
		MaxLevel:    session.MaxLevel,
		ReadingMode: save.EffectiveExportReadingMode(mode, sentencesOut),
		Sentences:   sentencesOut,
	})
	if err != nil {
		return "", nil, err
	}
	return save.SafeFilename(meta.Title) + ".html", content, nil
}

// PublishHTML writes a standalone reader site into the book bundle (reader.html).
func (s *Store) PublishHTML(bookID string, modeOverride string) (string, error) {
	_, data, err := s.HTMLExportBook(bookID, modeOverride)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.bookDir(bookID), "reader.html")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
