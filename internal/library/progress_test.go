package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadProgressRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	id := "abc12345"
	if err := os.MkdirAll(filepath.Join(dir, id), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := &Meta{
		ID:             id,
		Title:          "Test",
		TotalSentences: 10,
		RewriteStatus:  StatusPending,
		ShowEasier:     true,
	}
	if err := s.saveMeta(meta); err != nil {
		t.Fatal(err)
	}
	if err := s.saveBook(id, newBookFile("Test", []string{"one"})); err != nil {
		t.Fatal(err)
	}

	in := ReadProgress{Index: 3, Level: 2, ShowEasier: true, ShowChinese: true}
	if err := s.SaveReadProgress(id, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadReadProgress(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Index != 3 || got.Level != 2 || !got.ShowEasier || !got.ShowChinese {
		t.Fatalf("got %+v want index=3 level=2 easier+chinese", got)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("expected updatedAt")
	}
	data, err := os.ReadFile(filepath.Join(dir, id, progressFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("progress.json empty")
	}
}

func TestLoadReadProgressMigratesFromMeta(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	id := "deadbeef"
	if err := os.MkdirAll(filepath.Join(dir, id), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := &Meta{
		ID:             id,
		Title:          "Legacy",
		TotalSentences: 5,
		RewriteStatus:  StatusDone,
		ReadIndex:      4,
		ReadLevel:      1,
		ShowEasier:     true,
		ShowChinese:    false,
	}
	if err := s.saveMeta(meta); err != nil {
		t.Fatal(err)
	}

	got, err := s.LoadReadProgress(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Index != 4 || got.Level != 1 || !got.ShowEasier || got.ShowChinese {
		t.Fatalf("migrate got %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, id, progressFile)); err != nil {
		t.Fatal("expected progress.json created")
	}
}
