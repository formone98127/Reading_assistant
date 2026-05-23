package gui

import (
	"log"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"reading-assistant/internal/save"
	"reading-assistant/internal/session"
)

func uriDir(u fyne.URI) string {
	p := u.Path()
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.Dir(p)
}

func (u *readerUI) exportDir() string {
	return save.ResolveDir(u.sourceDir, u.cfg.SaveDir)
}

func (u *readerUI) exportBase() string {
	if u.sourceBase != "" {
		return u.sourceBase
	}
	return "paste"
}

func (u *readerUI) persistExports(notify bool) {
	if u.sess == nil {
		return
	}
	pair, err := save.WritePair(
		u.exportDir(),
		u.exportBase(),
		u.sess.Sentences,
		u.sess.ParagraphsAtLevel(session.MaxLevel),
	)
	if err != nil {
		if notify {
			u.showErr(err)
		}
		return
	}
	if notify {
		u.status.SetText("Saved:\n" + pair.OriginalPath + "\n" + pair.RewrittenPath)
	}
}

func (u *readerUI) saveOnLoad() {
	if u.sess == nil {
		return
	}
	path, err := save.WriteOriginal(u.exportDir(), u.exportBase(), u.sess.Sentences)
	if err != nil {
		log.Printf("save on load: %v", err)
		return
	}
	u.status.SetText("Saved original:\n" + path)
}

func (u *readerUI) saveRewriteDialog() {
	if u.sess == nil {
		return
	}
	dir := u.exportDir()
	base := u.exportBase()
	pair, err := save.WritePair(
		dir, base,
		u.sess.Sentences,
		u.sess.ParagraphsAtLevel(session.MaxLevel),
	)
	if err != nil {
		u.showErr(err)
		return
	}
	u.status.SetText("Saved:\n" + pair.OriginalPath + "\n" + pair.RewrittenPath)
	dialog.ShowInformation("Saved", pair.OriginalPath+"\n\n"+pair.RewrittenPath, u.window)
}
