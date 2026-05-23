package library

import (
	"context"
	"fmt"

	"reading-assistant/internal/session"
	"reading-assistant/internal/simplify"
)

// Rewriter runs full-book simplification into the library bundle.
type Rewriter struct {
	Store *Store
	LLM   *simplify.Client
}

// ProgressFunc reports rewrite progress (done, total).
type ProgressFunc func(done, total int)

// RewriteAll simplifies every sentence and updates the bundle on disk.
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
	_ = r.Store.SetRewriteProgress(bookID, meta.RewriteDone, total, StatusRewriting, "")

	start := meta.RewriteDone
	if start > total {
		start = 0
	}

	for i := start; i < total; i++ {
		if ctx.Err() != nil {
			prepared, _ := r.Store.LoadPrepared(bookID)
			done := contiguousRewriteDone(prepared, total)
			_ = r.Store.SetRewriteProgress(bookID, done, total, StatusRewriting, "")
			return ctx.Err()
		}
		if levels, ok := prepared[i]; ok && len(levels) >= 1 {
			if onProgress != nil {
				onProgress(i+1, total)
			}
			_ = r.Store.SetRewriteProgress(bookID, i+1, total, StatusRewriting, "")
			continue
		}
		levels, _, err := r.LLM.SimplifyAllLevels(ctx, sentences[i])
		if err != nil {
			_ = r.Store.SetRewriteProgress(bookID, i, total, StatusError, err.Error())
			return fmt.Errorf("sentence %d: %w", i+1, err)
		}
		m := make(map[int]string)
		for j, t := range levels {
			if j >= session.MaxLevel {
				break
			}
			m[j+1] = t
		}
		if len(m) > 0 {
			prepared[i] = m
			if err := r.Store.SaveSentenceLevels(bookID, i, m); err != nil {
				return err
			}
			if err := r.Store.SaveRewrittenFile(bookID, sentences, prepared); err != nil {
				return err
			}
		}
		if onProgress != nil {
			onProgress(i+1, total)
		}
		_ = r.Store.SetRewriteProgress(bookID, i+1, total, StatusRewriting, "")
	}
	_ = r.Store.SetRewriteProgress(bookID, total, total, StatusDone, "")
	return nil
}
