package library

import (
	"os"
	"path/filepath"
	"time"

	"reading-assistant/internal/session"
)

const progressFile = "progress.json"

// ReadProgress is the saved reading position for one library book.
type ReadProgress struct {
	Index       int       `json:"index"`
	Level       int       `json:"level"`
	ShowEasier  bool      `json:"showEasier"`
	ShowChinese bool      `json:"showChinese"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (s *Store) progressPath(id string) string {
	return filepath.Join(s.bookDir(id), progressFile)
}

// LoadReadProgress reads progress.json, migrating from meta.json on first access.
func (s *Store) LoadReadProgress(id string) (ReadProgress, error) {
	if err := validateBookID(id); err != nil {
		return ReadProgress{}, err
	}
	var p ReadProgress
	err := readJSON(s.progressPath(id), &p)
	if err == nil {
		return p, nil
	}
	if !os.IsNotExist(err) {
		return ReadProgress{}, err
	}
	m, err := s.LoadMeta(id)
	if err != nil {
		return ReadProgress{}, err
	}
	opts := m.ResolvedOptions()
	p = ReadProgress{
		Index:       m.ReadIndex,
		Level:       m.ReadLevel,
		ShowEasier:  opts.ShowEasier,
		ShowChinese: opts.ShowChinese,
	}
	if err := s.writeProgress(id, p); err != nil {
		return ReadProgress{}, err
	}
	return p, nil
}

// SaveReadProgress writes progress.json and mirrors index/level into meta.json for list views.
func (s *Store) SaveReadProgress(id string, p ReadProgress) error {
	if err := validateBookID(id); err != nil {
		return err
	}
	if err := s.writeProgress(id, p); err != nil {
		return err
	}
	return s.updateMeta(id, func(m *Meta) error {
		m.ReadIndex = p.Index
		m.ReadLevel = p.Level
		m.ShowEasier = p.ShowEasier
		m.ShowChinese = p.ShowChinese
		m.ReadingMode = session.ModeFromOptions(session.ReadingOptions{
			ShowEasier:  p.ShowEasier,
			ShowChinese: p.ShowChinese,
		})
		return nil
	})
}

func (s *Store) writeProgress(id string, p ReadProgress) error {
	p.UpdatedAt = time.Now().UTC()
	return writeJSON(s.progressPath(id), p)
}

// SaveReadPosition saves index and level, preserving reading options from progress.json.
func (s *Store) SaveReadPosition(id string, index, level int) error {
	p, err := s.LoadReadProgress(id)
	if err != nil {
		p = ReadProgress{ShowEasier: true}
	}
	p.Index = index
	p.Level = level
	return s.SaveReadProgress(id, p)
}
