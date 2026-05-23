package gui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"reading-assistant/internal/library"
	"reading-assistant/internal/parser"
	"reading-assistant/internal/save"
	"reading-assistant/internal/sentences"
	"reading-assistant/internal/session"
)

func (u *readerUI) initLibrary() {
	u.lib = library.NewStore(u.cfg.SaveDir)
	u.rewriter = &library.Rewriter{Store: u.lib, LLM: u.llm}
}

func (u *readerUI) refreshLibraryList() {
	if u.libraryList == nil {
		return
	}
	books, err := u.lib.List()
	u.libraryItems = nil
	u.libraryIDs = nil
	u.selectedLibrary = -1
	if u.libraryStatus != nil {
		if err != nil {
			u.libraryStatus.SetText("Could not load library: " + err.Error())
		} else if len(books) == 0 {
			u.libraryStatus.SetText("No saved books — upload a file below.")
		} else {
			u.libraryStatus.SetText("")
		}
	}
	if err == nil && len(books) > 0 {
		u.libraryItems = make([]string, len(books))
		u.libraryIDs = make([]string, len(books))
		for i, b := range books {
			u.libraryItems[i] = library.ListEntry(b)
			u.libraryIDs[i] = b.ID
		}
	}
	u.libraryList.Refresh()
}

func (u *readerUI) openSelectedLibraryBook() {
	if u.libraryList == nil {
		return
	}
	idx := u.selectedLibrary
	if idx < 0 || idx >= len(u.libraryIDs) {
		return
	}
	id := u.libraryIDs[idx]
	meta, err := u.lib.LoadMeta(id)
	if err != nil {
		u.showErr(err)
		return
	}
	u.openLibraryBook(id)
	u.ensureBookRewrite(id, meta.Title, meta.RewriteStatus)
}

func (u *readerUI) importAndRewrite(name string, data []byte, text string) {
	parts := sentences.Split(text)
	if len(parts) == 0 {
		dialog.ShowInformation("No sentences", "Could not split text into sentences.", u.window)
		return
	}
	title := save.BaseName(name)
	meta, err := u.lib.Import(title, name, data, parts)
	if err != nil {
		u.showErr(err)
		return
	}
	u.refreshLibraryList()
	u.openLibraryBook(meta.ID)
	u.startBackgroundRewrite(meta.ID, meta.Title)
}

func (u *readerUI) ensureBookRewrite(bookID, title, status string) {
	if status == library.StatusDone || status == library.StatusError {
		return
	}
	u.startBackgroundRewrite(bookID, title)
}

func (u *readerUI) startBackgroundRewrite(bookID, title string) {
	if u.activeRewriteBook == bookID {
		return
	}
	if u.rewriteCancel != nil {
		u.rewriteCancel()
	}
	u.activeRewriteBook = bookID
	ctx, cancel := context.WithCancel(context.Background())
	u.rewriteCancel = cancel

	go func() {
		ctx, cancelTimeout := context.WithTimeout(ctx, 2*time.Hour)
		defer cancelTimeout()
		err := u.rewriter.RewriteAll(ctx, bookID, func(done, total int) {
			fyne.Do(func() {
				if u.bookID != bookID {
					return
				}
				u.syncLibraryPrepared(bookID)
				v := u.enrichLibraryView(u.sess.View(), bookID)
				u.render(v)
			})
		})
		fyne.Do(func() {
			u.refreshLibraryList()
			if u.bookID == bookID {
				u.syncLibraryPrepared(bookID)
				if u.sess != nil {
					u.render(u.enrichLibraryView(u.sess.View(), bookID))
				}
			}
			if u.activeRewriteBook == bookID {
				u.activeRewriteBook = ""
			}
			if err != nil && ctx.Err() == nil {
				u.showErr(err)
			}
		})
	}()
}

func (u *readerUI) syncLibraryPrepared(bookID string) {
	if u.sess == nil || bookID == "" {
		return
	}
	prepared, err := u.lib.LoadPrepared(bookID)
	if err != nil {
		return
	}
	u.sess.RefreshPrepared(prepared)
}

func (u *readerUI) enrichLibraryView(v session.View, bookID string) session.View {
	meta, err := u.lib.LoadMeta(bookID)
	if err != nil {
		return v
	}
	v.BookRewriteDone = meta.RewriteDone
	v.BookRewriteTotal = meta.TotalSentences
	v.BookRewriteActive = meta.RewriteStatus == library.StatusRewriting || meta.RewriteStatus == library.StatusPending
	if v.BookRewriteActive && meta.TotalSentences > 0 {
		v.PrepStatus = fmt.Sprintf("Background rewrite %d/%d — you can read now", meta.RewriteDone, meta.TotalSentences)
		v.PrepActive = true
	}
	return v
}

func (u *readerUI) openLibraryBook(bookID string) {
	meta, err := u.lib.LoadMeta(bookID)
	if err != nil {
		u.showErr(err)
		return
	}
	parts, err := u.lib.LoadSentences(bookID)
	if err != nil {
		u.showErr(err)
		return
	}
	prepared, err := u.lib.LoadPrepared(bookID)
	if err != nil {
		u.showErr(err)
		return
	}
	u.sourceDir = u.lib.Dir(bookID)
	u.sourceBase = "book"
	u.bookID = bookID
	u.sess = u.mgr.Create("gui-"+bookID, parts)
	u.sess.SetBookID(bookID)
	u.sess.SetSource(u.sourceDir, meta.Title)
	u.sess.SeedPrepared(prepared)
	u.sess.ApplyReadPosition(meta.ReadIndex, meta.ReadLevel)
	u.loadCard.Hide()
	u.readerCard.Show()
	v := u.enrichLibraryView(u.sess.View(), bookID)
	u.render(v)
	u.prepareAsync()
	u.startPoll()
	u.focusReader()
}

func (u *readerUI) persistLibraryPosition() {
	if u.sess == nil || u.bookID == "" {
		return
	}
	v := u.sess.View()
	_ = u.lib.SaveReadPosition(u.bookID, v.Index, v.Level)
}

func (u *readerUI) saveOnClose() {
	u.app.Preferences().SetFloat("textSize", float64(u.textSize))
	u.stopBackgroundRewrite()
	u.flushLibraryToDisk()
	u.persistLibraryPosition()
}

func (u *readerUI) stopBackgroundRewrite() {
	if u.rewriteCancel != nil {
		u.rewriteCancel()
		u.rewriteCancel = nil
	}
	u.activeRewriteBook = ""
}

func (u *readerUI) flushLibraryToDisk() {
	if u.bookID == "" || u.sess == nil {
		return
	}
	_ = u.lib.PersistPrepared(u.bookID, u.sess.PreparedSnapshot())
}

func (u *readerUI) selectedLibraryBookID() string {
	if u.selectedLibrary < 0 || u.selectedLibrary >= len(u.libraryIDs) {
		return ""
	}
	return u.libraryIDs[u.selectedLibrary]
}

func (u *readerUI) exportLibraryHTML(bookID string) {
	if bookID == "" {
		dialog.ShowInformation("Export HTML", "Select a book from the library first.", u.window)
		return
	}
	name, data, err := u.lib.HTMLExport(bookID)
	if err != nil {
		u.showErr(err)
		return
	}
	fd := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		defer w.Close()
		if _, err := w.Write(data); err != nil {
			u.showErr(err)
			return
		}
		u.status.SetText("Exported HTML:\n" + w.URI().Path())
	}, u.window)
	fd.SetFileName(name)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".html"}))
	fd.Show()
}

func (u *readerUI) importFileFromReader(name string, r io.Reader) ([]byte, string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	text, err := parser.ExtractText(name, bytes.NewReader(data))
	return data, text, err
}
