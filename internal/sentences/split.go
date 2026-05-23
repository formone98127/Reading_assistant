package sentences

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Placeholder for a period that belongs to an abbreviation (not a sentence end).
const abbrevDot = "\uE001"

// Longer dotted abbrevs first so e.g. matches before e.
var abbrevDotRE = regexp.MustCompile(
	`(?i)\b(?:e\.g\.|i\.e\.|u\.s\.|u\.k\.|mr|mrs|ms|dr|prof|rev|hon|sr|jr|st|vs|etc|no)\.`,
)

// Split divides plain text into reading sentences.
func Split(text string) []string {
	text = normalizeWhitespace(text)
	if text == "" {
		return nil
	}
	masked := maskAbbrevDots(text)
	parts := splitMasked(masked)
	for i := range parts {
		parts[i] = unmaskAbbrevDots(parts[i])
	}
	return filterEmpty(parts)
}

func maskAbbrevDots(s string) string {
	return abbrevDotRE.ReplaceAllStringFunc(s, func(m string) string {
		return m[:len(m)-len(".")] + abbrevDot
	})
}

func unmaskAbbrevDots(s string) string {
	return strings.ReplaceAll(s, abbrevDot, ".")
}

func splitMasked(text string) []string {
	var out []string
	var buf strings.Builder
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		ch := runes[i]
		buf.WriteRune(ch)
		if ch == ';' {
			chunk := strings.TrimSpace(buf.String())
			if chunk != "" {
				out = append(out, chunk)
			}
			buf.Reset()
			end := i + 1
			for end < len(runes) && unicode.IsSpace(runes[end]) {
				end++
			}
			i = end - 1
		} else if ch == '.' || ch == '!' || ch == '?' || ch == '…' {
			end := i + 1
			for end < len(runes) && (runes[end] == '.' || runes[end] == '!' || runes[end] == '?') {
				buf.WriteRune(runes[end])
				end++
				i = end - 1
			}
			chunk := strings.TrimSpace(buf.String())
			if shouldSplit(chunk, runes, end) {
				out = append(out, chunk)
				buf.Reset()
				for end < len(runes) && unicode.IsSpace(runes[end]) {
					end++
				}
				i = end - 1
			}
		}
		i++
	}
	if tail := strings.TrimSpace(buf.String()); tail != "" {
		out = append(out, tail)
	}
	return out
}

func normalizeWhitespace(s string) string {
	s = norm.NFKC.String(s)
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\u200B', '\u200C', '\u200D', '\uFEFF':
			return -1
		case '\uFF0E', '\u2024', '\u00B7':
			return '.'
		case '\u00A0':
			return ' '
		}
		return r
	}, s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(line)
	}
	return strings.TrimSpace(b.String())
}

func shouldSplit(chunk string, runes []rune, after int) bool {
	chunk = strings.TrimSpace(chunk)
	if chunk == "" {
		return false
	}
	// e.g. / U.S. — period immediately followed by letter/digit (unmasked edge cases)
	for after < len(runes) && runes[after] == '.' {
		after++
	}
	if after < len(runes) {
		r := runes[after]
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func filterEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
