package gui

import (
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"reading-assistant/internal/session"
)

func (u *readerUI) readingOptions() session.ReadingOptions {
	if u.chkEasier == nil {
		return session.ReadingOptions{ShowEasier: true}
	}
	return session.ReadingOptions{
		ShowEasier:  u.chkEasier.Checked,
		ShowChinese: u.chkChinese.Checked,
	}
}

func (u *readerUI) applyReadingOptions(o session.ReadingOptions) {
	if u.chkEasier == nil {
		return
	}
	u.chkEasier.SetChecked(o.ShowEasier)
	u.chkChinese.SetChecked(o.ShowChinese)
}

func (u *readerUI) loadReadingPrefs() {
	o := session.ReadingOptions{ShowEasier: true}
	if raw := u.app.Preferences().StringWithFallback("readingOptions", ""); raw != "" {
		var saved session.ReadingOptions
		if json.Unmarshal([]byte(raw), &saved) == nil {
			o = saved
		}
	} else if saved := u.app.Preferences().StringWithFallback("readingMode", ""); saved != "" {
		o = session.OptionsFromMode(saved)
	}
	u.applyReadingOptions(o)
}

func (u *readerUI) saveReadingPrefs() {
	if u.chkEasier == nil {
		return
	}
	o := u.readingOptions()
	raw, _ := json.Marshal(o)
	u.app.Preferences().SetString("readingOptions", string(raw))
	u.app.Preferences().SetString("readingMode", session.ModeFromOptions(o))
	if u.sess != nil {
		u.sess.SetReadingOptions(o)
		if u.bookID != "" {
			_ = u.lib.SetReadingOptions(u.bookID, o)
		}
		u.render(u.sess.View())
		u.prepareAsync()
	}
}

func trackOptionsBox(easier, chinese *widget.Check) fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel("Rewrite version (both: → easier 1→2→3→中文)"),
		easier,
		chinese,
	)
}
