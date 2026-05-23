package library

import (
	"reading-assistant/internal/save"
	"reading-assistant/internal/session"
)

// HTMLExport builds a shareable interactive HTML reader for a library book.
func (s *Store) HTMLExport(bookID string) (filename string, content []byte, err error) {
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
	content, err = save.BuildBookHTML(save.ReaderExport{
		Title:      meta.Title,
		StartIndex: meta.ReadIndex,
		StartLevel: meta.ReadLevel,
		MaxLevel:   session.MaxLevel,
		Sentences:  save.BuildReaderSentences(sentences, prepared, session.MaxLevel),
	})
	if err != nil {
		return "", nil, err
	}
	return save.SafeFilename(meta.Title) + ".html", content, nil
}
