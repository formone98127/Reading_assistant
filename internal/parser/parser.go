package parser

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// ExtractText returns plain text from a file or raw bytes based on extension.
func ExtractText(filename string, r io.Reader) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", "":
		return extractTXT(r)
	case ".epub":
		return extractEPUB(r)
	case ".pdf":
		return extractPDF(r)
	case ".mobi":
		return extractMOBI(r)
	default:
		return "", fmt.Errorf("unsupported format %q (use .txt, .epub, .pdf, or .mobi)", ext)
	}
}

// FromPlain accepts pasted or uploaded plain text without a filename extension.
func FromPlain(text string) string {
	return strings.TrimSpace(text)
}
