package session

// Reading modes control what the reader shows, not what library upload generates.
// Library books always store original + easier 1–3 + Chinese; modes pick the view.
const (
	// ModeEnglish: original and easier levels 1–3 only (no 中文 panel).
	ModeEnglish = "english"
	// ModeEnglishChinese: English line (original or easier) plus 中文 below.
	ModeEnglishChinese = "english_chinese"
)

// NormalizeReadingMode returns a supported mode, defaulting to English only.
func NormalizeReadingMode(m string) string {
	if m == ModeEnglishChinese {
		return ModeEnglishChinese
	}
	return ModeEnglish
}

// ChineseEnabled is true when the reader should show the 中文 panel.
func ChineseEnabled(m string) bool {
	return NormalizeReadingMode(m) == ModeEnglishChinese
}
