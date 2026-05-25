package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	englishLevelsFile = "english.json"
	chineseFile       = "chinese.json"
	legacyLevelsFile  = "levels.json"
)

func (s *Store) englishLevelsPath(id string) string {
	return filepath.Join(s.bookDir(id), englishLevelsFile)
}

func (s *Store) chinesePath(id string) string {
	return filepath.Join(s.bookDir(id), chineseFile)
}

func (s *Store) loadEnglishLevels(id string) (map[int]map[int]string, error) {
	raw := map[string]map[string]string{}
	if err := readJSON(s.englishLevelsPath(id), &raw); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	out := make(map[int]map[int]string, len(raw))
	for k, v := range raw {
		var idx int
		if _, err := fmt.Sscanf(k, "%d", &idx); err != nil {
			continue
		}
		levels := make(map[int]string)
		for lk, txt := range v {
			var lv int
			if _, err := fmt.Sscanf(lk, "%d", &lv); err == nil && strings.TrimSpace(txt) != "" {
				levels[lv] = txt
			}
		}
		if len(levels) > 0 {
			out[idx] = levels
		}
	}
	return out, nil
}

func (s *Store) loadChinese(id string) (map[int]string, error) {
	raw := map[string]string{}
	if err := readJSON(s.chinesePath(id), &raw); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	out := make(map[int]string, len(raw))
	for k, txt := range raw {
		var idx int
		if _, err := fmt.Sscanf(k, "%d", &idx); err != nil {
			continue
		}
		if strings.TrimSpace(txt) != "" {
			out[idx] = txt
		}
	}
	return out, nil
}

func splitLegacyLevels(raw map[string]map[string]string) (map[string]map[string]string, map[string]string) {
	engOut := map[string]map[string]string{}
	chOut := map[string]string{}
	for k, v := range raw {
		levels := make(map[string]string)
		for lk, txt := range v {
			if lk == "zh" || lk == "zh-Hant" || lk == "zh-Hans" {
				if strings.TrimSpace(txt) != "" {
					chOut[k] = txt
				}
				continue
			}
			var lv int
			if _, err := fmt.Sscanf(lk, "%d", &lv); err == nil && strings.TrimSpace(txt) != "" {
				levels[lk] = txt
			}
		}
		if len(levels) > 0 {
			engOut[k] = levels
		}
	}
	return engOut, chOut
}

// migrateLegacyLevels splits old levels.json (English + zh mixed) into english.json and chinese.json.
func (s *Store) migrateLegacyLevels(id string) error {
	legacyPath := filepath.Join(s.bookDir(id), legacyLevelsFile)
	raw := map[string]map[string]string{}
	if err := readJSON(legacyPath, &raw); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	engOut, chOut := splitLegacyLevels(raw)
	engPath := s.englishLevelsPath(id)
	chPath := s.chinesePath(id)
	if _, err := os.Stat(engPath); os.IsNotExist(err) {
		if len(engOut) > 0 {
			if err := writeJSON(engPath, engOut); err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(chPath); os.IsNotExist(err) {
		if len(chOut) > 0 {
			if err := writeJSON(chPath, chOut); err != nil {
				return err
			}
		}
	}
	return nil
}
