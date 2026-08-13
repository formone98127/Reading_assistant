package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (u *readerUI) bindKeys() {
	c := u.window.Canvas()
	mod := fyne.KeyModifierShortcutDefault

	add := func(key fyne.KeyName, m fyne.KeyModifier, when func() bool, fn func()) {
		sc := &desktop.CustomShortcut{KeyName: key, Modifier: m}
		c.AddShortcut(sc, func(fyne.Shortcut) {
			if when() {
				fn()
			}
		})
	}

	whenReader := func() bool {
		return u.readerCard.Visible() && !u.busy
	}
	whenLoad := func() bool {
		return !u.readerCard.Visible()
	}

	// Match exported HTML reader: ↓ prev · ↑ next · ← harder · → simpler
	for _, b := range []struct {
		key fyne.KeyName
		fn  func()
	}{
		{fyne.KeyDown, u.prev},
		{fyne.KeyUp, u.next},
		{fyne.KeyLeft, u.harder},
		{fyne.KeyRight, u.easier},
	} {
		add(b.key, mod, whenReader, b.fn)
	}

	add(fyne.KeyEscape, 0, whenReader, u.exitReader)
	add(fyne.KeyM, mod, whenReader, u.showReadingModeDialog)

	add(fyne.KeyMinus, mod, func() bool { return u.readerCard.Visible() }, func() { u.adjustTextSize(-2) })
	larger := func() { u.adjustTextSize(2) }
	add(fyne.KeyEqual, mod, func() bool { return u.readerCard.Visible() }, larger)
	add(fyne.KeyPlus, mod, func() bool { return u.readerCard.Visible() }, larger)

	add(fyne.KeyReturn, mod, whenLoad, u.startPaste)
	add(fyne.KeyO, mod, whenLoad, u.openFile)

	c.SetOnTypedKey(func(ev *fyne.KeyEvent) {
		u.onArrowKey(ev)
	})
}
