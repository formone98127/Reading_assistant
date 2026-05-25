package server

import "reading-assistant/internal/session"

// refreshSessionFromLibrary reloads english.json and chinese.json into the session.
func (s *Server) refreshSessionFromLibrary(sess *session.Session) {
	bookID := sess.BookIDValue()
	if bookID == "" {
		return
	}
	prepared, err := s.library.LoadPrepared(bookID)
	if err != nil {
		return
	}
	sess.RefreshPrepared(prepared.English, prepared.Chinese)
}
