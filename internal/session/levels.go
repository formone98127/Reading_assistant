package session

import "strings"

// LevelChinese is the unified level index for 繁體中文 (same ladder as easier 1–3).
const LevelChinese = 4

// NavLevels returns the →/← ladder for current reading options (0 = original).
func (o ReadingOptions) NavLevels() []int {
	if !o.ShowEasier && !o.ShowChinese {
		return []int{0}
	}
	out := []int{0}
	if o.ShowEasier {
		out = append(out, 1, 2, 3)
	}
	if o.ShowChinese {
		out = append(out, LevelChinese)
	}
	return out
}

// NormalizeStoredLevel maps legacy readLevel (中文-only used 1) to LevelChinese.
func (o ReadingOptions) NormalizeStoredLevel(level int) int {
	if level == 1 && o.ShowChinese && !o.ShowEasier {
		return LevelChinese
	}
	return level
}

func (o ReadingOptions) levelAllowed(level int) bool {
	if level == 0 {
		return true
	}
	for _, l := range o.NavLevels() {
		if l == level {
			return true
		}
	}
	return false
}

func nextNavLevel(seq []int, level int) (int, bool) {
	for i, l := range seq {
		if l == level && i+1 < len(seq) {
			return seq[i+1], true
		}
	}
	return level, false
}

func prevNavLevel(seq []int, level int) (int, bool) {
	for i, l := range seq {
		if l == level && i > 0 {
			return seq[i-1], true
		}
	}
	return 0, level > 0
}

func mergePreparedLevels(english map[int]map[int]string, chinese map[int]string) map[int]map[int]string {
	out := make(map[int]map[int]string)
	for idx, levels := range english {
		cp := make(map[int]string, len(levels)+1)
		for k, v := range levels {
			if k >= 1 && k <= MaxLevel && strings.TrimSpace(v) != "" {
				cp[k] = v
			}
		}
		if len(cp) > 0 {
			out[idx] = cp
		}
	}
	for idx, text := range chinese {
		if strings.TrimSpace(text) == "" {
			continue
		}
		if out[idx] == nil {
			out[idx] = make(map[int]string)
		}
		out[idx][LevelChinese] = text
	}
	return out
}

func splitLevels(levels map[int]map[int]string) (english map[int]map[int]string, chinese map[int]string) {
	english = make(map[int]map[int]string)
	chinese = make(map[int]string)
	for idx, m := range levels {
		for lv, text := range m {
			if strings.TrimSpace(text) == "" {
				continue
			}
			if lv == LevelChinese {
				chinese[idx] = text
				continue
			}
			if lv >= 1 && lv <= MaxLevel {
				if english[idx] == nil {
					english[idx] = make(map[int]string)
				}
				english[idx][lv] = text
			}
		}
	}
	return english, chinese
}

func englishLevelsOnly(m map[int]string) map[int]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[int]string)
	for lv, text := range m {
		if lv >= 1 && lv <= MaxLevel && strings.TrimSpace(text) != "" {
			out[lv] = text
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
