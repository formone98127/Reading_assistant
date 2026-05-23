package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// keyCapture is a transparent focus target over the sentence area for arrow keys.
// It does not cover nav buttons (unlike a full-card overlay).
type keyCapture struct {
	widget.BaseWidget
	onKey func(*fyne.KeyEvent)
}

func newKeyCapture(onKey func(*fyne.KeyEvent)) *keyCapture {
	k := &keyCapture{onKey: onKey}
	k.ExtendBaseWidget(k)
	return k
}

func (k *keyCapture) TypedKey(ev *fyne.KeyEvent) {
	if k.onKey != nil {
		k.onKey(ev)
	}
}

func (k *keyCapture) TypedRune(_ rune) {}

func (k *keyCapture) Tapped(_ *fyne.PointEvent) {}

func (k *keyCapture) FocusGained() {}
func (k *keyCapture) FocusLost()   {}

func (k *keyCapture) CreateRenderer() fyne.WidgetRenderer {
	r := canvas.NewRectangle(color.Transparent)
	return widget.NewSimpleRenderer(r)
}
