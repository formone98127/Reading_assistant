package library

import (
	"encoding/base64"
	"os"
	"path/filepath"

	"reading-assistant/internal/save"
	"reading-assistant/internal/session"
)

// HTMLExport builds a shareable interactive HTML reader for a library book.
func (s *Store) HTMLExport(bookID string) (filename string, content []byte, err error) {
	return s.HTMLExportBook(bookID, "", "", "")
}

// HTMLExportBook builds export HTML. Query overrides: mode (legacy) or showEasier/showChinese.
func (s *Store) HTMLExportBook(bookID, modeOverride, showEasierQ, showChineseQ string) (filename string, content []byte, err error) {
	meta, err := s.LoadMeta(bookID)
	if err != nil {
		return "", nil, err
	}
	book, err := s.LoadBook(bookID)
	if err != nil {
		return "", nil, err
	}
	opts := meta.ResolvedOptions()
	if showEasierQ != "" || showChineseQ != "" {
		e := showEasierQ == "true" || showEasierQ == "1"
		c := showChineseQ == "true" || showChineseQ == "1"
		opts = session.ReadingOptions{ShowEasier: e, ShowChinese: c}
	} else if modeOverride == session.TrackEasier || modeOverride == session.TrackChinese || modeOverride == session.TrackOriginal {
		opts = session.OptionsFromTrack(modeOverride)
	} else if modeOverride != "" {
		opts = session.OptionsFromMode(modeOverride)
	}
	sentencesOut := book.Sentences
	if meta.TTSEnabled {
		for i := range sentencesOut {
			if !s.HasAudio(bookID, i) {
				continue
			}
			wav, err := s.LoadAudio(bookID, i)
			if err != nil || len(wav) == 0 {
				continue
			}
			sentencesOut[i].Audio = base64.StdEncoding.EncodeToString(wav)
		}
	}
	opts = save.EffectiveExportOptions(opts, sentencesOut)
	prog, err := s.LoadReadProgress(bookID)
	if err != nil {
		return "", nil, err
	}
	content, err = save.BuildBookHTML(save.ReaderExport{
		Title:       meta.Title,
		StartIndex:  prog.Index,
		StartLevel:  prog.Level,
		MaxLevel:    session.MaxLevel,
		ReadingMode: session.ModeFromOptions(opts),
		TTSEnabled:  meta.TTSEnabled,
		Sentences:   sentencesOut,
	})
	if err != nil {
		return "", nil, err
	}
	return save.SafeFilename(meta.Title) + ".html", content, nil
}

// PublishHTML writes a standalone reader site into the book bundle (reader.html).
func (s *Store) PublishHTML(bookID, modeOverride, showEasierQ, showChineseQ string) (string, error) {
	_, data, err := s.HTMLExportBook(bookID, modeOverride, showEasierQ, showChineseQ)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.bookDir(bookID), "reader.html")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
