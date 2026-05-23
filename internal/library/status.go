package library

import "fmt"

// StatusLabel is a short human-readable rewrite status for library lists.
func StatusLabel(m *Meta) string {
	if m == nil {
		return ""
	}
	switch m.RewriteStatus {
	case StatusPending:
		return "queued"
	case StatusRewriting:
		if m.TotalSentences > 0 {
			return fmt.Sprintf("rewriting %d/%d", m.RewriteDone, m.TotalSentences)
		}
		return "rewriting"
	case StatusDone:
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
