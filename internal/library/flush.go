package library

// PersistPrepared merges in-memory levels into the bundle, updates book.rewritten.txt,
// and records rewrite progress so RewriteAll can resume after reopen.
func (s *Store) PersistPrepared(id string, fromSession map[int]map[int]string) error {
	prepared, err := s.LoadPrepared(id)
	if err != nil {
		return err
	}
	for idx, levels := range fromSession {
		if len(levels) == 0 {
			continue
		}
		prepared[idx] = levels
		if err := s.SaveSentenceLevels(id, idx, levels); err != nil {
			return err
		}
	}
	sentences, err := s.LoadSentences(id)
	if err != nil {
		return err
	}
	if err := s.SaveRewrittenFile(id, sentences, prepared); err != nil {
		return err
	}
	done := contiguousRewriteDone(prepared, len(sentences))
	status := StatusRewriting
	if done >= len(sentences) {
		status = StatusDone
	}
	return s.SetRewriteProgress(id, done, len(sentences), status, "")
}

func contiguousRewriteDone(prepared map[int]map[int]string, total int) int {
	done := 0
	for i := 0; i < total; i++ {
		levels, ok := prepared[i]
		if !ok || len(levels) < 1 {
			break
		}
		done = i + 1
	}
	return done
}
