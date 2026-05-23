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
	if prepared, err := s.library.LoadPrepared(bookID); err == nil {
		sess.RefreshPrepared(prepared)
		v = sess.View()
	}
	meta, err := s.library.LoadMeta(bookID)
	if err != nil {
		return v
	}
	v.BookRewriteDone = meta.RewriteDone
	v.BookRewriteTotal = meta.TotalSentences
	v.BookRewriteActive = meta.RewriteStatus == library.StatusRewriting || meta.RewriteStatus == library.StatusPending
	if v.BookRewriteActive && meta.TotalSentences > 0 {
		v.PrepStatus = fmt.Sprintf("Background rewrite %d/%d — you can read now", meta.RewriteDone, meta.TotalSentences)
		v.PrepActive = true
	}
	return v
}
