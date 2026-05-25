package library

import "strings"

func englishComplete(p PreparedData, idx int) bool {
	levels, ok := p.English[idx]
	return ok && len(levels) >= 1
}

func chineseComplete(p PreparedData, idx int) bool {
	return strings.TrimSpace(p.Chinese[idx]) != ""
}

func sentenceComplete(p PreparedData, idx int) bool {
	return englishComplete(p, idx) && chineseComplete(p, idx)
}

func firstIncompleteEnglish(p PreparedData, total int) int {
	for i := 0; i < total; i++ {
		if !englishComplete(p, i) {
			return i
		}
	}
	return total
}

func firstIncompleteChinese(p PreparedData, total int) int {
	for i := 0; i < total; i++ {
		if !chineseComplete(p, i) {
			return i
		}
	}
	return total
}

func contiguousEnglishDone(p PreparedData, total int) int {
	done := 0
	for i := 0; i < total; i++ {
		if !englishComplete(p, i) {
			break
		}
		done = i + 1
	}
	return done
}

func contiguousChineseDone(p PreparedData, total int) int {
	done := 0
	for i := 0; i < total; i++ {
		if !chineseComplete(p, i) {
			break
		}
		done = i + 1
	}
	return done
}

func englishRewriteComplete(p PreparedData, total int) bool {
	if total == 0 {
		return true
	}
	return contiguousEnglishDone(p, total) >= total
}

func chineseRewriteComplete(p PreparedData, total int) bool {
	if total == 0 {
		return true
	}
	return contiguousChineseDone(p, total) >= total
}

// RewriteComplete is true when both english.json and chinese.json are complete.
func RewriteComplete(p PreparedData, total int) bool {
	return englishRewriteComplete(p, total) && chineseRewriteComplete(p, total)
}
