package save

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BaseName is the filename without extension.
func BaseName(filename string) string {
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

// ResolveDir uses sourceDir when set, otherwise defaultSaveDir.
func ResolveDir(sourceDir, defaultSaveDir string) string {
	if strings.TrimSpace(sourceDir) != "" {
		return sourceDir
	}
	return defaultSaveDir
}

// DefaultDir returns the app save folder (created if missing).
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "reading-assistant", "saves")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// JoinParagraphs formats sentences for export files.
func JoinParagraphs(paragraphs []string) string {
	return joinParagraphs(paragraphs)
}

func joinParagraphs(paragraphs []string) string {
	var b strings.Builder
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(p)
	}
	return b.String()
}

// WriteOriginal saves sentence-split source text.
func WriteOriginal(dir, base string, sentences []string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, base+".original.txt")
	return path, os.WriteFile(path, []byte(joinParagraphs(sentences)), 0o644)
}

// WriteRewritten saves the simplified version (one paragraph per sentence).
func WriteRewritten(dir, base string, paragraphs []string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, base+".rewritten.txt")
	return path, os.WriteFile(path, []byte(joinParagraphs(paragraphs)), 0o644)
}

// Pair holds paths written for a session export.
type Pair struct {
	OriginalPath  string `json:"originalPath"`
	RewrittenPath string `json:"rewrittenPath"`
	Dir           string `json:"dir"`
	Base          string `json:"base"`
}

// WritePair writes original and rewritten files; rewritten may be partial.
func WritePair(dir, base string, original, rewritten []string) (Pair, error) {
	if base == "" {
		base = "paste"
	}
	origPath, err := WriteOriginal(dir, base, original)
	if err != nil {
		return Pair{}, fmt.Errorf("original: %w", err)
	}
	rewPath, err := WriteRewritten(dir, base, rewritten)
	if err != nil {
		return Pair{}, fmt.Errorf("rewritten: %w", err)
	}
	return Pair{
		OriginalPath:  origPath,
		RewrittenPath: rewPath,
		Dir:           dir,
		Base:          base,
	}, nil
}
