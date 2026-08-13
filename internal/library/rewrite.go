package library

import (
	"context"
	"fmt"

	"reading-assistant/internal/session"
	"reading-assistant/internal/simplify"
	"reading-assistant/internal/tts"
)

// Rewriter runs full-book simplification into the library bundle.
type Rewriter struct {
	Store          *Store
	LLM            *simplify.Client
	TTS            *tts.Client
	TTSEnabled     bool
	TTSControl     string
	TTSCfg         float64
	TTSTimesteps   int
}

// ProgressFunc reports rewrite progress (done, total).
type ProgressFunc func(done, total int)

// RewriteAll pass 1: easier English → book.json. Pass 2: 中文.
func (r *Rewriter) RewriteAll(ctx context.Context, bookID string, onProgress ProgressFunc) error {
	meta, err := r.Store.LoadMeta(bookID)
	if err != nil {
		return err
	}
	sentences, err := r.Store.LoadSentences(bookID)
	if err != nil {
		return err
	}
	prepared, err := r.Store.LoadPrepared(bookID)
	if err != nil {
		return err
	}
	total := len(sentences)

	if !meta.VoiceOnly {
		engStart := firstIncompleteEnglish(prepared, total)
		_ = r.Store.SetRewriteProgress(bookID, contiguousEnglishDone(prepared, total), total, StatusRewriting, "")

		for i := engStart; i < total; i++ {
			if ctx.Err() != nil {
				prepared, _ = r.Store.LoadPrepared(bookID)
				done := contiguousEnglishDone(prepared, total)
				_ = r.Store.SetRewriteProgress(bookID, done, total, StatusRewriting, "")
				return ctx.Err()
			}
			if englishComplete(prepared, i) {
				_ = r.Store.SetRewriteProgress(bookID, i+1, total, StatusRewriting, "")
				if onProgress != nil {
					onProgress(i+1, total)
				}
				continue
			}
			lv, _, err := r.LLM.SimplifyAllLevels(ctx, sentences[i])
			if err != nil {
				_ = r.Store.SetRewriteProgress(bookID, i, total, StatusError, err.Error())
				return fmt.Errorf("english sentence %d: %w", i+1, err)
			}
			m := make(map[int]string)
			for j, t := range lv {
				if j >= session.MaxLevel {
					break
				}
				m[j+1] = t
			}
			if len(m) > 0 {
				prepared.English[i] = m
				if err := r.Store.SaveSentenceLevels(bookID, i, m); err != nil {
					return err
				}
				if err := r.Store.SaveRewrittenFile(bookID, sentences, prepared); err != nil {
					return err
				}
			}
			_ = r.Store.SetRewriteProgress(bookID, i+1, total, StatusRewriting, "")
			if onProgress != nil {
				onProgress(i+1, total)
			}
		}

		prepared, err = r.Store.LoadPrepared(bookID)
		if err != nil {
			return err
		}
		meta, err = r.Store.LoadMeta(bookID)
		if err != nil {
			return err
		}

		chStart := firstIncompleteChinese(prepared, total)
		if chStart < total {
			_ = r.Store.SetRewriteProgress(bookID, total, total, StatusRewriting, "")
		}
		for i := chStart; i < total; i++ {
			if ctx.Err() != nil {
				prepared, _ = r.Store.LoadPrepared(bookID)
				_ = r.Store.SetChineseProgress(bookID, contiguousChineseDone(prepared, total))
				return ctx.Err()
			}
			if chineseComplete(prepared, i) {
				_ = r.Store.SetChineseProgress(bookID, i+1)
				if onProgress != nil {
					onProgress(i+1, total)
				}
				continue
			}
			text, err := r.LLM.TranslateChinese(ctx, sentences[i])
			if err != nil {
				_ = r.Store.SetRewriteProgress(bookID, meta.RewriteDone, total, StatusError, err.Error())
				return fmt.Errorf("chinese sentence %d: %w", i+1, err)
			}
			prepared.Chinese[i] = text
			if err := r.Store.SaveChinese(bookID, i, text); err != nil {
				return err
			}
			_ = r.Store.SetChineseProgress(bookID, i+1)
			if onProgress != nil {
				onProgress(i+1, total)
			}
		}

		_ = r.Store.SetChineseProgress(bookID, contiguousChineseDone(prepared, total))
	} else {
		_ = r.Store.SetRewriteProgress(bookID, total, total, StatusRewriting, "")
		_ = r.Store.SetChineseProgress(bookID, total)
	}

	meta, err = r.Store.LoadMeta(bookID)
	if err != nil {
		return err
	}
	if meta.TTSEnabled && r.TTS != nil {
		if !audioRewriteComplete(r.Store, bookID, total) {
			_ = r.Store.SetRewriteProgress(bookID, total, total, StatusRewriting, "")
			_ = r.Store.ClearSentenceAudio(bookID, total)
			_ = r.Store.SetAudioGenerating(bookID, true)
			defer func() { _ = r.Store.SetAudioGenerating(bookID, false) }()
			segments, err := r.TTS.SpeakBook(ctx, tts.BookRequest{
				Sentences:          sentences,
				Control:            r.TTSControl,
				CFGValue:           r.TTSCfg,
				InferenceTimesteps: r.TTSTimesteps,
			})
			if err != nil {
				_ = r.Store.SetRewriteProgress(bookID, meta.RewriteDone, total, StatusError, err.Error())
				return fmt.Errorf("voice book: %w", err)
			}
			if len(segments) != total {
				return fmt.Errorf("voice book: got %d segments, want %d", len(segments), total)
			}
			for i, wav := range segments {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err := r.Store.SaveAudio(bookID, i, wav); err != nil {
					return err
				}
				if i == 0 {
					_ = r.Store.SaveReferenceAudio(bookID, wav)
				}
				_ = r.Store.SetAudioProgress(bookID, i+1)
				if onProgress != nil {
					onProgress(i+1, total)
				}
			}
		}
	}

	_ = r.Store.SetRewriteProgress(bookID, total, total, StatusDone, "")
	return nil
}
