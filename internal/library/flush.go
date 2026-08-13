package library

import "strings"

// PersistPrepared merges session rewrites into book.json.
func (s *Store) PersistPrepared(id string, english map[int]map[int]string, chinese map[int]string) error {
	prepared, err := s.LoadPrepared(id)
	if err != nil {
		return err
	}
	for idx, levels := range english {
		if len(levels) == 0 {
			continue
		}
		prepared.English[idx] = levels
		if err := s.SaveSentenceLevels(id, idx, levels); err != nil {
			return err
		}
	}
	for idx, text := range chinese {
		if strings.TrimSpace(text) == "" {
			continue
		}
		prepared.Chinese[idx] = text
		if err := s.SaveChinese(id, idx, text); err != nil {
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
	total := len(sentences)
	done := contiguousEnglishDone(prepared, total)
	status := StatusRewriting
	meta, _ := s.LoadMeta(id)
	ttsEnabled := meta != nil && meta.TTSEnabled
	voiceOnly := meta != nil && meta.VoiceOnly
	if RewriteComplete(s, id, prepared, total, ttsEnabled, voiceOnly) {
		status = StatusDone
		done = total
	}
	_ = s.SetChineseProgress(id, contiguousChineseDone(prepared, total))
	return s.SetRewriteProgress(id, done, total, status, "")
}
