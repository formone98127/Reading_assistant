package library

import (
	"os"
	"path/filepath"
	"testing"

	"reading-assistant/internal/session"
)

func TestBookJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	id := "book0001"
	if err := os.MkdirAll(filepath.Join(dir, id), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := &Meta{ID: id, Title: "Demo", TotalSentences: 2, RewriteStatus: StatusPending, ShowEasier: true}
	if err := s.saveMeta(meta); err != nil {
		t.Fatal(err)
	}
	book := newBookFile("Demo", []string{"Hello world.", "Goodbye."})
	if err := s.saveBook(id, book); err != nil {
		t.Fatal(err)
	}

	if err := s.SaveSentenceLevels(id, 0, map[int]string{1: "Hi world.", 2: "Hi.", 3: "Hi!"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveChinese(id, 0, "你好。"); err != nil {
		t.Fatal(err)
	}

	got, err := s.LoadBook(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sentences[0].Levels[0] != "Hi world." || got.Sentences[0].Chinese != "你好。" {
		t.Fatalf("got sentence 0: %+v", got.Sentences[0])
	}
	prepared, err := s.LoadPrepared(id)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.English[0][1] != "Hi world." || prepared.Chinese[0] != "你好。" {
		t.Fatalf("prepared: english=%v chinese=%v", prepared.English[0], prepared.Chinese[0])
	}
	sents, err := s.LoadSentences(id)
	if err != nil || sents[0] != "Hello world." {
		t.Fatalf("originals: %v err=%v", sents, err)
	}
}

func TestMigrateLegacyToBookJSON(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	id := "legacy01"
	bookDir := filepath.Join(dir, id)
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := &Meta{ID: id, Title: "Legacy", TotalSentences: 1, RewriteStatus: StatusDone, ShowEasier: true}
	if err := s.saveMeta(meta); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(bookDir, "sentences.json"), []string{"Original."}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(bookDir, englishLevelsFile), map[string]map[string]string{
		"0": {"1": "Easy.", "2": "Easier.", "3": "Easiest."},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(bookDir, chineseFile), map[string]string{"0": "原文。"}); err != nil {
		t.Fatal(err)
	}

	book, err := s.LoadBook(id)
	if err != nil {
		t.Fatal(err)
	}
	if book.Sentences[0].Original != "Original." {
		t.Fatalf("original: %q", book.Sentences[0].Original)
	}
	if book.Sentences[0].Levels[2] != "Easiest." || book.Sentences[0].Chinese != "原文。" {
		t.Fatalf("levels/zh: %+v", book.Sentences[0])
	}
	if _, err := os.Stat(filepath.Join(bookDir, bookFile)); err != nil {
		t.Fatal("book.json not created")
	}
	_ = session.MaxLevel
}
