package library

import "fmt"

// RewriteProgressActive is true while english.json or chinese.json work is still in progress
// (including meta marked done before 中文 finished).
func RewriteProgressActive(m *Meta) bool {
	if m == nil || m.TotalSentences <= 0 {
		return false
	}
	switch m.RewriteStatus {
	case StatusPending, StatusRewriting:
		return true
	case StatusDone:
		if m.VoiceOnly {
			if m.TTSEnabled && m.AudioRewriteDone < m.TotalSentences {
				return true
			}
			return false
		}
		if m.RewriteDone < m.TotalSentences || m.ChineseRewriteDone < m.TotalSentences {
			return true
		}
		if m.TTSEnabled && m.AudioRewriteDone < m.TotalSentences {
			return true
		}
		return false
	default:
		return false
	}
}

// StatusLabel is a short human-readable rewrite status for library lists.
func StatusLabel(m *Meta) string {
	if m == nil {
		return ""
	}
	switch m.RewriteStatus {
	case StatusPending:
		return "queued"
	case StatusRewriting:
		if m.VoiceOnly && m.TTSEnabled && m.TotalSentences > 0 {
			return fmt.Sprintf("voice %d/%d", m.AudioRewriteDone, m.TotalSentences)
		}
		if m.TotalSentences > 0 &&
			m.RewriteDone >= m.TotalSentences &&
			m.ChineseRewriteDone >= m.TotalSentences &&
			m.TTSEnabled &&
			m.AudioRewriteDone < m.TotalSentences {
			return fmt.Sprintf("voice %d/%d", m.AudioRewriteDone, m.TotalSentences)
		}
		if m.TotalSentences > 0 &&
			m.RewriteDone >= m.TotalSentences &&
			m.ChineseRewriteDone < m.TotalSentences {
			return fmt.Sprintf("chinese %d/%d", m.ChineseRewriteDone, m.TotalSentences)
		}
		if m.TotalSentences > 0 {
			return fmt.Sprintf("english %d/%d", m.RewriteDone, m.TotalSentences)
		}
		return "rewriting"
	case StatusDone:
		if m.VoiceOnly {
			if m.TTSEnabled && m.TotalSentences > 0 && m.AudioRewriteDone < m.TotalSentences {
				return fmt.Sprintf("voice %d/%d", m.AudioRewriteDone, m.TotalSentences)
			}
			return "ready"
		}
		if m.TotalSentences > 0 && m.ChineseRewriteDone < m.TotalSentences {
			return fmt.Sprintf("chinese %d/%d", m.ChineseRewriteDone, m.TotalSentences)
		}
		if m.TotalSentences > 0 && m.RewriteDone < m.TotalSentences {
			return fmt.Sprintf("english %d/%d", m.RewriteDone, m.TotalSentences)
		}
		if m.TTSEnabled && m.TotalSentences > 0 && m.AudioRewriteDone < m.TotalSentences {
			return fmt.Sprintf("voice %d/%d", m.AudioRewriteDone, m.TotalSentences)
		}
		return "ready"
	case StatusError:
		return "rewrite paused"
	default:
		if m.RewriteStatus == "" {
			return "unknown"
		}
		return m.RewriteStatus
	}
}

// ListEntry formats a book as "title — status" for UI lists.
func ListEntry(m Meta) string {
	return fmt.Sprintf("%s — %s", m.Title, StatusLabel(&m))
}
