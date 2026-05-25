package server

import (
	"fmt"

	"reading-assistant/internal/library"
	"reading-assistant/internal/session"
)

func (s *Server) enrichView(sess *session.Session, v session.View) session.View {
	bookID := sess.BookIDValue()
	if bookID == "" {
		return v
	}
	s.refreshSessionFromLibrary(sess)
	v = sess.View()
	meta, err := s.library.LoadMeta(bookID)
	if err != nil {
		return v
	}
	// Do not call SetReadingMode here — it resets chineseVisible and level.
	// Mode is set at session open and via /api/reading-mode.
	v.BookRewriteDone = meta.RewriteDone
	v.BookRewriteTotal = meta.TotalSentences
	v.BookChineseDone = meta.ChineseRewriteDone
	v.BookRewriteActive = library.RewriteProgressActive(meta)
	if v.BookRewriteActive && meta.TotalSentences > 0 {
		if meta.RewriteDone >= meta.TotalSentences && meta.ChineseRewriteDone < meta.TotalSentences {
			v.PrepStatus = fmt.Sprintf("chinese.json %d/%d — you can read now", meta.ChineseRewriteDone, meta.TotalSentences)
		} else {
			v.PrepStatus = fmt.Sprintf("english.json %d/%d — you can read now", meta.RewriteDone, meta.TotalSentences)
		}
		v.PrepActive = true
	} else if v.ShowChinese && v.PrepStatus != "" {
		// Keep session hints visible in EN+中文 when not in background rewrite.
		v.PrepActive = true
	}
	return v
}
