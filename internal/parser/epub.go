package parser

import (
	"bytes"
	"io"
	"strings"

	"github.com/simp-lee/epub"
)

func extractEPUB(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	book, err := epub.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	defer book.Close()

	var b strings.Builder
	for _, ch := range book.ContentChapters() {
		t, err := ch.TextContent()
		if err != nil {
			continue
		}
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(t)
	}
	return b.String(), nil
}
