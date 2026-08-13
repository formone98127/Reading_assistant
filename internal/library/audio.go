package library

import (
	"fmt"
	"os"
	"path/filepath"

	"reading-assistant/internal/audio"
)

func (s *Store) audioDir(id string) string {
	return filepath.Join(s.bookDir(id), "audio")
}

func (s *Store) audioPath(id string, idx int) string {
	return filepath.Join(s.audioDir(id), fmt.Sprintf("%06d.wav", idx))
}

func (s *Store) referenceAudioPath(id string) string {
	return filepath.Join(s.audioDir(id), "reference.wav")
}

// HasReferenceAudio reports whether the book has a voice anchor clip.
func (s *Store) HasReferenceAudio(id string) bool {
	if err := validateBookID(id); err != nil {
		return false
	}
	_, err := os.Stat(s.referenceAudioPath(id))
	return err == nil
}

// ReferenceAudioAbsPath returns the absolute path to reference.wav, or "" if missing.
func (s *Store) ReferenceAudioAbsPath(id string) (string, error) {
	if err := validateBookID(id); err != nil {
		return "", err
	}
	path := s.referenceAudioPath(id)
	if _, err := os.Stat(path); err != nil {
		return "", nil
	}
	return filepath.Abs(path)
}

// SaveReferenceAudio stores the voice timbre anchor (sentence 0 wav).
func (s *Store) SaveReferenceAudio(id string, wav []byte) error {
	wav, _ = audio.NormalizeWAV(wav)
	if err := validateBookID(id); err != nil {
		return err
	}
	dir := s.audioDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.referenceAudioPath(id), wav, 0o644)
}

// EnsureReferenceAudio seeds reference.wav from sentence 0 if it already exists.
func (s *Store) EnsureReferenceAudio(id string) error {
	if s.HasReferenceAudio(id) {
		return nil
	}
	if !s.HasAudio(id, 0) {
		return nil
	}
	wav, err := s.readRawAudio(id, 0)
	if err != nil {
		return err
	}
	wav, _ = audio.NormalizeWAV(wav)
	return s.SaveReferenceAudio(id, wav)
}

// ClearSentenceAudio removes per-sentence wav files and resets progress.
func (s *Store) ClearSentenceAudio(id string, total int) error {
	if err := validateBookID(id); err != nil {
		return err
	}
	for i := 0; i < total; i++ {
		_ = os.Remove(s.audioPath(id, i))
	}
	_ = os.Remove(s.referenceAudioPath(id))
	return s.updateMeta(id, func(m *Meta) error {
		m.AudioRewriteDone = 0
		return nil
	})
}

// HasAudio reports whether sentence idx has a saved wav file.
func (s *Store) HasAudio(id string, idx int) bool {
	if err := validateBookID(id); err != nil {
		return false
	}
	_, err := os.Stat(s.audioPath(id, idx))
	return err == nil
}

// SaveAudio writes wav bytes for one sentence index.
func (s *Store) SaveAudio(id string, idx int, wav []byte) error {
	wav, _ = audio.NormalizeWAV(wav)
	if err := validateBookID(id); err != nil {
		return err
	}
	dir := s.audioDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.audioPath(id, idx), wav, 0o644)
}

// LoadAudio returns volume-normalized wav bytes for one sentence index.
func (s *Store) LoadAudio(id string, idx int) ([]byte, error) {
	data, err := s.readRawAudio(id, idx)
	if err != nil {
		return nil, err
	}
	return audio.NormalizeWAV(data)
}

func (s *Store) readRawAudio(id string, idx int) ([]byte, error) {
	if err := validateBookID(id); err != nil {
		return nil, err
	}
	return os.ReadFile(s.audioPath(id, idx))
}

// SetAudioGenerating marks whether the book is in one-shot voice synthesis.
func (s *Store) SetAudioGenerating(id string, generating bool) error {
	return s.updateMeta(id, func(meta *Meta) error {
		meta.AudioGenerating = generating
		return nil
	})
}

// SetAudioProgress updates audioRewriteDone in meta.json.
func (s *Store) SetAudioProgress(id string, done int) error {
	return s.updateMeta(id, func(meta *Meta) error {
		meta.AudioRewriteDone = done
		return nil
	})
}
