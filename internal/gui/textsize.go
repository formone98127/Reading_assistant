package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	fontSizeMin     = 16
	fontSizeMax     = 180
	fontSizeDefault = 22
)

// scaledTheme maps reader text size names to an explicit pixel size (up to 180pt).
type scaledTheme struct {
	fyne.Theme
	size float32
}

func newScaledTheme(px float32) *scaledTheme {
	base := theme.DefaultTheme()
	return &scaledTheme{Theme: base, size: px}
}

func (s *scaledTheme) setSize(px float32) {
	s.size = px
}

func (s *scaledTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText,
		theme.SizeNameHeadingText,
		theme.SizeNameSubHeadingText,
		theme.SizeNameCaptionText:
		return s.size
	case theme.SizeNameLineSpacing:
		spacing := s.size * 0.15
		if spacing < 4 {
			return 4
		}
		return spacing
	}
	if s.Theme != nil {
		return s.Theme.Size(name)
	}
	return theme.DefaultTheme().Size(name)
}

func setRichText(rt *widget.RichText, text string, bold, italic bool) {
	style := widget.RichTextStyle{
		TextStyle: fyne.TextStyle{Bold: bold, Italic: italic},
		SizeName:  theme.SizeNameText,
	}
	rt.Segments = []widget.RichTextSegment{&widget.TextSegment{Text: text, Style: style}}
	rt.Refresh()
}
