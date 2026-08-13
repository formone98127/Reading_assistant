package server

import (
	"fmt"
	"strings"

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
	book, bookErr := s.library.LoadBook(bookID)
	if bookErr == nil && v.Index >= 0 && v.Index < len(book.Sentences) {
		sent := book.Sentences[v.Index]
		var lvTexts []string
		for _, l := range sent.Levels {
			if strings.TrimSpace(l) != "" {
				lvTexts = append(lvTexts, l)
			}
		}
		v.LevelTexts = lvTexts
	}
	meta, err := s.library.LoadMeta(bookID)
	if err != nil {
		return v
	}
	// Do not call SetReadingMode here — it resets chineseVisible and level.
	// Mode is set at session open and via /api/reading-mode.
	v.BookRewriteDone = meta.RewriteDone
	v.BookRewriteTotal = meta.TotalSentences
	v.BookChineseDone = meta.ChineseRewriteDone
	v.BookTTSEnabled = meta.TTSEnabled
	v.BookAudioDone = meta.AudioRewriteDone
	v.BookAudioGenerating = meta.AudioGenerating
	v.BookVoiceOnly = meta.VoiceOnly
	if meta.TTSEnabled && v.Index >= 0 {
		v.HasAudio = s.library.HasAudio(bookID, v.Index)
	}
	v.BookRewriteActive = library.RewriteProgressActive(meta)
	if v.BookRewriteActive && meta.TotalSentences > 0 {
		if meta.AudioGenerating {
			v.PrepStatus = "Generating voice…"
			v.PrepActive = true
		} else if meta.VoiceOnly {
			if meta.AudioRewriteDone < meta.TotalSentences {
				v.PrepStatus = fmt.Sprintf("voice %d/%d — you can read now", meta.AudioRewriteDone, meta.TotalSentences)
				v.PrepActive = true
			}
		} else if meta.TTSEnabled &&
			meta.RewriteDone >= meta.TotalSentences &&
			meta.ChineseRewriteDone >= meta.TotalSentences &&
			meta.AudioRewriteDone < meta.TotalSentences {
			v.PrepStatus = fmt.Sprintf("voice %d/%d — you can read now", meta.AudioRewriteDone, meta.TotalSentences)
			v.PrepActive = true
		} else if meta.RewriteDone >= meta.TotalSentences && meta.ChineseRewriteDone < meta.TotalSentences {
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
