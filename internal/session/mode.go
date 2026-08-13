package session

// Reading mode strings (stored on books and in API).
const (
	ModeEnglish        = "english"
	ModeEnglishChinese = "english_chinese"
	ModeOriginal       = "original"
	ModeEasierChinese  = "easier_chinese"
)

const (
	TrackEasier   = "easier"
	TrackChinese  = "chinese"
	TrackOriginal = "original"
)

// ReadingOptions: user can enable Easier and/or 中文 independently.
type ReadingOptions struct {
	ShowEasier  bool
	ShowChinese bool
}

// NavTrack picks which family →/← steps through (easier levels, or 中文 toggle).
func (o ReadingOptions) NavTrack() string {
	if !o.ShowEasier && !o.ShowChinese {
		return TrackOriginal
	}
	if o.ShowEasier {
		return TrackEasier
	}
	return TrackChinese
}

// NormalizeReadingMode maps legacy mode strings.
func NormalizeReadingMode(m string) string {
	switch m {
	case ModeEnglishChinese, ModeEasierChinese, ModeOriginal:
		return m
	default:
		return ModeEnglish
	}
}

// OptionsFromMode converts stored readingMode to checkbox flags.
func OptionsFromMode(m string) ReadingOptions {
	switch NormalizeReadingMode(m) {
	case ModeEasierChinese:
		return ReadingOptions{ShowEasier: true, ShowChinese: true}
	case ModeEnglishChinese:
		return ReadingOptions{ShowEasier: false, ShowChinese: true}
	case ModeOriginal:
		return ReadingOptions{}
	default:
		return ReadingOptions{ShowEasier: true}
	}
}

// OptionsFromTrack maps a single-track API value to flags.
func OptionsFromTrack(t string) ReadingOptions {
	switch t {
	case TrackChinese:
		return ReadingOptions{ShowChinese: true}
	case TrackOriginal:
		return ReadingOptions{}
	default:
		return ReadingOptions{ShowEasier: true}
	}
}

// ModeFromOptions encodes flags for meta/export.
func ModeFromOptions(o ReadingOptions) string {
	if o.ShowChinese && o.ShowEasier {
		return ModeEasierChinese
	}
	if o.ShowChinese {
		return ModeEnglishChinese
	}
	if o.ShowEasier {
		return ModeEnglish
	}
	return ModeOriginal
}

// ChineseEnabled reports whether a stored mode includes 中文.
func ChineseEnabled(mode string) bool {
	switch NormalizeReadingMode(mode) {
	case ModeEnglishChinese, ModeEasierChinese:
		return true
	default:
		return false
	}
}

// Combined returns true when → steps original → easier 1–3 → 中文.
func (o ReadingOptions) Combined() bool {
	return o.ShowEasier && o.ShowChinese
}

func (o ReadingOptions) MaxLevel() int {
	seq := o.NavLevels()
	if len(seq) == 0 {
		return 0
	}
	return seq[len(seq)-1]
}

// ChineseLevel is the unified 中文 slot in the level database.
func (o ReadingOptions) ChineseLevel() int {
	return LevelChinese
}
