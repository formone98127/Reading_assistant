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

func audioRewriteComplete(s *Store, bookID string, total int) bool {
	if total == 0 {
		return true
	}
	for i := 0; i < total; i++ {
		if !s.HasAudio(bookID, i) {
			return false
		}
	}
	return true
}

func firstIncompleteAudio(s *Store, bookID string, total int) int {
	for i := 0; i < total; i++ {
		if !s.HasAudio(bookID, i) {
			return i
		}
	}
	return total
}

func contiguousAudioDone(s *Store, bookID string, total int) int {
	done := 0
	for i := 0; i < total; i++ {
		if !s.HasAudio(bookID, i) {
			break
		}
		done = i + 1
	}
	return done
}

// RewriteComplete is true when english, chinese, and optional audio are complete.
func RewriteComplete(s *Store, bookID string, p PreparedData, total int, ttsEnabled, voiceOnly bool) bool {
	if voiceOnly {
		if !ttsEnabled {
			return true
		}
		return audioRewriteComplete(s, bookID, total)
	}
	if !englishRewriteComplete(p, total) || !chineseRewriteComplete(p, total) {
		return false
	}
	if ttsEnabled {
		return audioRewriteComplete(s, bookID, total)
	}
	return true
}
