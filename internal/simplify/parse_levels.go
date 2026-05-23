package simplify

import (
	"regexp"
	"strings"
)

var levelPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^LEVEL\s*([123])\s*[:\-\.]?\s*(.+)$`),
	regexp.MustCompile(`(?im)^\s*([123])[\.\)]\s+(.+)$`),
	regexp.MustCompile(`(?im)^\*\*LEVEL\s*([123])\*\*\s*[:\-]?\s*(.+)$`),
	regexp.MustCompile(`(?im)^(?:version|level)\s*([123])\s*[:\-]\s*(.+)$`),
}

// ParseLevels extracts up to 3 simplified sentences from a batch response.
func ParseLevels(response string) [3]string {
	var out [3]string
	response = strings.TrimSpace(response)
	if response == "" {
		return out
	}
	for _, re := range levelPatterns {
		for _, m := range re.FindAllStringSubmatch(response, -1) {
			if len(m) < 3 {
				continue
			}
			idx := m[1]
			text := clean(m[2])
			if text == "" || text == "..." {
				continue
			}
			switch idx {
			case "1":
				out[0] = text
			case "2":
				out[1] = text
			case "3":
				out[2] = text
			}
		}
		if out[0] != "" {
			return out
		}
	}
	return out
}

// LevelsFromResponse returns 1–3 levels from model text.
func LevelsFromResponse(response string, original string) []string {
	parsed := ParseLevels(response)
	var levels []string
	for i := 0; i < 3; i++ {
		t := parsed[i]
		if t == "" && i == 0 {
			t = firstUsefulLine(response, original)
		}
		if t == "" && len(levels) > 0 {
			t = levels[len(levels)-1]
		}
		if t == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(t), strings.TrimSpace(original)) && i > 0 {
			continue
		}
		levels = append(levels, t)
	}
	if len(levels) == 0 {
		levels = linesAsLevels(response, original)
	}
	return levels
}

func firstUsefulLine(response, original string) string {
	if t := ParseSimplified(response); t != "" {
		return t
	}
	return fallbackSimplified(response)
}

func linesAsLevels(response, original string) []string {
	var levels []string
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "level") || strings.HasPrefix(low, "original") {
			continue
		}
		if strings.HasPrefix(low, "###") || strings.HasPrefix(low, "- **") {
			continue
		}
		line = clean(line)
		if line == "" || line == "..." || len(line) < 8 {
			continue
		}
		if strings.EqualFold(line, original) {
			continue
		}
		levels = append(levels, line)
		if len(levels) >= 3 {
			break
		}
	}
	return levels
}
