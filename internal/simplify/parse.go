package simplify

import (
	"regexp"
	"strings"
)

var (
	boldLine = regexp.MustCompile(`(?is)###\s*1\.\s*Simplified Version\s*\n>\s*\*{0,2}(.+?)\*{0,2}\s*(?:\n|$)`)
	fallback = regexp.MustCompile(`(?is)>\s*\*{0,2}(.+?)\*{0,2}\s*$`)
)

// ParseSimplified extracts the simplified sentence from the model response.
func ParseSimplified(response string) string {
	response = strings.TrimSpace(response)
	if m := boldLine.FindStringSubmatch(response); len(m) > 1 {
		return clean(m[1])
	}
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, ">") {
			return clean(strings.TrimPrefix(line, ">"))
		}
	}
	if m := fallback.FindStringSubmatch(response); len(m) > 1 {
		return clean(m[1])
	}
	return clean(response)
}

func clean(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*")
	s = strings.TrimSpace(s)
	return s
}

// fallbackSimplified uses the first plausible line when the model skips markdown format.
func fallbackSimplified(response string) string {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "###") || strings.HasPrefix(low, "##") {
			continue
		}
		if strings.HasPrefix(low, "- **") || strings.HasPrefix(low, "**word") {
			continue
		}
		if strings.HasPrefix(line, ">") {
			return clean(strings.TrimPrefix(line, ">"))
		}
		if len(line) > 300 {
			continue
		}
		return clean(line)
	}
	return ""
}
