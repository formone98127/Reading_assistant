package gui

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"reading-assistant/internal/config"
	"reading-assistant/internal/library"
	"reading-assistant/internal/parser"
	"reading-assistant/internal/save"
	"reading-assistant/internal/sentences"
	"reading-assistant/internal/session"
	"reading-assistant/internal/simplify"
)

// Run starts the native desktop UI.
func Run(cfg config.Config) {
	a := app.NewWithID("reading-assistant")
	w := a.NewWindow("Reading Assistant")
	w.Resize(fyne.NewSize(720, 520))
	w.SetMaster()

	llm := &simplify.Client{BaseURL: cfg.OllamaURL, Model: cfg.OllamaModel}
	mgr := session.NewManager(llm)

	ui := &readerUI{
		cfg:      cfg,
		llm:      llm,
		mgr:      mgr,
		window:   w,
		app:      a,
		textSize: fontSizeDefault,
	}
	if v := a.Preferences().FloatWithFallback("textSize", fontSizeDefault); v >= fontSizeMin && v <= fontSizeMax {
		ui.textSize = float32(v)
	}
	ui.build()
	ui.initLibrary()
	ui.refreshLibraryList()
	ui.window.SetCloseIntercept(func() {
		ui.saveOnClose()
		ui.window.Close()
	})
	ui.warmModel()
	w.ShowAndRun()
}

type readerUI struct {
	cfg    config.Config
	llm    *simplify.Client
	mgr    *session.Manager
	sess   *session.Session
	window fyne.Window
	app    fyne.App

	loadCard   *fyne.Container
	readerCard *fyne.Container

	pasteEntry *widget.Entry
	progress   *widget.Label
	levelBadge *widget.Label
	sentenceRT      *widget.RichText
	sentenceScaled  *scaledTheme
	sentenceWrap    *container.ThemeOverride
	previousLabel   *widget.Label
	previousRT      *widget.RichText
	previousScaled  *scaledTheme
	previousWrap    *container.ThemeOverride
	readingScroll   *container.Scroll
	textSize        float32
	fontSlider     *widget.Slider
	fontSizeValue  *widget.Label
	compareBox     *fyne.Container
	status         *widget.Label
	prepBar        *widget.ProgressBarInfinite
	prepStatus     *widget.Label
	prepStatusBar  *fyne.Container
	hint   *widget.Label
	keyCap *keyCapture

	sourceDir  string
	sourceBase string
	bookID     string

	lib               *library.Store
	rewriter          *library.Rewriter
	rewriteCancel     context.CancelFunc
	activeRewriteBook string
	libraryList       *widget.List
	libraryRewriteBar *widget.ProgressBar
	libraryItems     []string
	libraryIDs       []string
	selectedLibrary  int

	busy     bool
	pollStop chan struct{}
}

func (u *readerUI) build() {
	u.pasteEntry = widget.NewMultiLineEntry()
	u.pasteEntry.SetPlaceHolder("Paste English text here…")
	u.pasteEntry.SetMinRowsVisible(10)

	openBtn := widget.NewButton("Add book to library (.txt, .epub, .pdf, .mobi)", func() { u.openFile() })
	startPaste := widget.NewButton("Start reading", func() { u.startPaste() })
	u.selectedLibrary = -1
	u.libraryList = widget.NewList(
		func() int { return len(u.libraryItems) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if int(id) < len(u.libraryItems) {
				obj.(*widget.Label).SetText(u.libraryItems[id])
			}
		},
	)
	u.libraryList.OnSelected = func(id widget.ListItemID) {
		u.selectedLibrary = int(id)
	}
	openLibBtn := widget.NewButton("Open saved book", func() { u.openSelectedLibraryBook() })
	refreshLibBtn := widget.NewButton("Refresh", func() { u.refreshLibraryList() })

	u.loadCard = container.NewVBox(
		widget.NewLabel("Load text to read sentence by sentence."),
		widget.NewLabel(fmt.Sprintf("Model: %s", u.cfg.OllamaModel)),
		widget.NewLabel("Ctrl+Enter start · Ctrl+O open file"),
		widget.NewSeparator(),
		widget.NewLabel("Your library"),
		u.libraryList,
		container.NewHBox(openLibBtn, refreshLibBtn),
		widget.NewSeparator(),
		widget.NewLabel("Paste"),
		u.pasteEntry,
		startPaste,
		widget.NewSeparator(),
		openBtn,
	)

	u.progress = widget.NewLabel("")
	u.levelBadge = widget.NewLabel("Original")
	u.sentenceRT = widget.NewRichText()
	u.sentenceRT.Wrapping = fyne.TextWrapWord
	u.sentenceScaled = newScaledTheme(u.textSize)
	u.sentenceWrap = container.NewThemeOverride(u.sentenceRT, u.sentenceScaled)
	u.previousLabel = widget.NewLabel("")
	u.previousLabel.TextStyle = fyne.TextStyle{Italic: true}
	u.previousRT = widget.NewRichText()
	u.previousRT.Wrapping = fyne.TextWrapWord
	u.previousScaled = newScaledTheme(u.textSize)
	u.previousWrap = container.NewThemeOverride(u.previousRT, u.previousScaled)
	u.compareBox = container.NewVBox(u.previousLabel, u.previousWrap)
	u.status = widget.NewLabel("")
	u.libraryRewriteBar = widget.NewProgressBar()
	u.libraryRewriteBar.Hide()
	u.prepBar = widget.NewProgressBarInfinite()
	u.prepStatus = widget.NewLabel("")
	u.prepStatus.Wrapping = fyne.TextWrapWord
	u.prepStatusBar = container.NewBorder(nil, nil, nil, u.prepBar, u.prepStatus)
	prepArea := container.NewVBox(u.libraryRewriteBar, u.prepStatusBar)
	u.hint = widget.NewLabel(
		"↓ Simpler · ↑ Harder · → Next · ← Prev · Ctrl+arrows anywhere · Ctrl +/− text size",
	)

	prevBtn := widget.NewButton("← Prev sentence", func() { u.prev() })
	nextBtn := widget.NewButton("Next sentence →", func() { u.next() })
	harderBtn := widget.NewButton("↑ Harder level", func() { u.harder() })
	simplerBtn := widget.NewButton("↓ Simpler", func() { u.easier() })
	saveBtn := widget.NewButton("Save", func() { u.saveRewriteDialog() })

	nowLabel := widget.NewLabel("Now reading")
	nowLabel.TextStyle = fyne.TextStyle{Bold: true}

	u.fontSlider = widget.NewSlider(fontSizeMin, fontSizeMax)
	u.fontSlider.Step = 1
	u.fontSlider.SetValue(float64(u.textSize))
	u.fontSlider.OnChanged = func(v float64) {
		u.textSize = float32(v)
		u.app.Preferences().SetFloat("textSize", v)
		u.fontSizeValue.SetText(fmt.Sprintf("%.0f pt", u.textSize))
		u.refreshTextSizes()
	}
	fontDown := widget.NewButton("A−", func() { u.adjustTextSize(-2) })
	fontUp := widget.NewButton("A+", func() { u.adjustTextSize(2) })
	fontSizeLabel := widget.NewLabel("Text size")
	u.fontSizeValue = widget.NewLabel(fmt.Sprintf("%.0f pt", u.textSize))
	fontRow := container.NewHBox(
		fontSizeLabel,
		fontDown,
		fontUp,
		u.fontSlider,
		layout.NewSpacer(),
		u.fontSizeValue,
	)

	u.keyCap = newKeyCapture(u.onArrowKey)
	sentencePanel := container.NewStack(
		container.NewPadded(u.sentenceWrap),
		u.keyCap,
	)

	readingContent := container.NewVBox(
		u.compareBox,
		nowLabel,
		sentencePanel,
	)
	u.readingScroll = container.NewVScroll(readingContent)

	topBar := container.NewVBox(
		container.NewHBox(u.progress, layout.NewSpacer(), u.levelBadge),
		fontRow,
		container.NewPadded(prepArea),
	)
	bottomBar := container.NewVBox(
		u.status,
		container.NewHBox(prevBtn, harderBtn, simplerBtn, nextBtn, layout.NewSpacer(), saveBtn),
		u.hint,
	)
	u.readerCard = container.NewBorder(topBar, bottomBar, nil, nil, u.readingScroll)
	u.readerCard.Hide()

	content := container.NewStack(u.loadCard, u.readerCard)
	u.window.SetContent(container.NewPadded(content))
	u.bindKeys()
}

func (u *readerUI) onArrowKey(ev *fyne.KeyEvent) {
	if !u.readerCard.Visible() || u.busy {
		return
	}
	switch ev.Name {
	case fyne.KeyDown:
		u.easier()
	case fyne.KeyUp:
		u.harder()
	case fyne.KeyRight:
		u.next()
	case fyne.KeyLeft:
		u.prev()
	}
}

func (u *readerUI) focusReader() {
	u.window.Canvas().Unfocus()
	if u.keyCap != nil {
		u.window.Canvas().Focus(u.keyCap)
	}
}

func (u *readerUI) warmModel() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := u.llm.Warm(ctx); err != nil {
			log.Printf("model warm-up: %v", err)
		}
	}()
}

func (u *readerUI) startPaste() {
	text := parser.FromPlain(u.pasteEntry.Text)
	if text == "" {
		dialog.ShowInformation("Paste text", "Enter some text first.", u.window)
		return
	}
	u.beginReading(text, "paste.txt", "")
}

func (u *readerUI) openFile() {
	dialog.ShowFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil || rc == nil {
			return
		}
		defer rc.Close()
		name := filepath.Base(rc.URI().Path())
		data, text, err := u.importFileFromReader(name, rc)
		if err != nil {
			u.showErr(err)
			return
		}
		u.importAndRewrite(name, data, text)
	}, u.window)
}

func (u *readerUI) beginReading(text, filename, sourceDir string) {
	parts := sentences.Split(text)
	if len(parts) == 0 {
		dialog.ShowInformation("No sentences", "Could not split text into sentences.", u.window)
		return
	}
	u.sourceDir = sourceDir
	u.sourceBase = save.BaseName(filename)
	u.sess = u.mgr.Create("gui", parts)
	u.sess.SetSource(u.exportDir(), filename)
	u.loadCard.Hide()
	u.readerCard.Show()
	u.render(u.sess.View())
	u.saveOnLoad()
	u.prepareAsync()
	u.startPoll()
	u.focusReader()
}

func (u *readerUI) prepareAsync() {
	if u.sess == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		u.sess.PrepareWhileReading(ctx, u.llm)
		fyne.Do(func() {
			if u.sess != nil {
				u.render(u.sess.View())
			}
		})
	}()
}

func (u *readerUI) startPoll() {
	u.stopPoll()
	u.pollStop = make(chan struct{})
	go func() {
		t := time.NewTicker(1500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-u.pollStop:
				return
			case <-t.C:
				if u.sess == nil {
					return
				}
				v := u.sess.View()
				fyne.Do(func() {
					if u.bookID != "" {
						u.syncLibraryPrepared(u.bookID)
						v = u.enrichLibraryView(u.sess.View(), u.bookID)
					}
					u.render(v)
				})
				if !v.PrepActive && !v.BookRewriteActive {
					return
				}
			}
		}
	}()
}

func (u *readerUI) stopPoll() {
	if u.pollStop != nil {
		close(u.pollStop)
		u.pollStop = nil
	}
}

func (u *readerUI) render(v session.View) {
	if u.bookID != "" && u.sess != nil {
		u.syncLibraryPrepared(u.bookID)
		v = u.enrichLibraryView(u.sess.View(), u.bookID)
	}
	u.progress.SetText(fmt.Sprintf("%d / %d", v.Index+1, v.Total))
	u.levelBadge.SetText(levelLabel(v.Level))
	u.renderPrepBar(v)
	u.applyViewText(v)
}

func (u *readerUI) adjustTextSize(delta float32) {
	u.textSize += delta
	if u.textSize < fontSizeMin {
		u.textSize = fontSizeMin
	}
	if u.textSize > fontSizeMax {
		u.textSize = fontSizeMax
	}
	u.fontSlider.SetValue(float64(u.textSize))
	u.app.Preferences().SetFloat("textSize", float64(u.textSize))
	u.fontSizeValue.SetText(fmt.Sprintf("%.0f pt", u.textSize))
	u.refreshTextSizes()
}

func (u *readerUI) refreshTextSizes() {
	if u.sess != nil {
		u.applyViewText(u.sess.View())
	}
}

func (u *readerUI) applyViewText(v session.View) {
	u.sentenceScaled.setSize(u.textSize)
	u.sentenceWrap.Refresh()
	setRichText(u.sentenceRT, v.Sentence, true, false)
	if u.readingScroll != nil {
		u.readingScroll.Refresh()
	}
	if v.Previous != "" {
		u.previousLabel.SetText(compareHeading(v.PreviousLevel))
		prevSize := u.textSize - 4
		if prevSize < fontSizeMin {
			prevSize = fontSizeMin
		}
		u.previousScaled.setSize(prevSize)
		u.previousWrap.Refresh()
		setRichText(u.previousRT, v.Previous, false, false)
		u.compareBox.Show()
	} else {
		u.compareBox.Hide()
	}
}

func (u *readerUI) renderPrepBar(v session.View) {
	msg := v.PrepStatus
	if u.busy && msg == "" {
		msg = "Simplifying…"
	}
	u.prepStatus.SetText(msg)
	if v.BookRewriteActive {
		u.libraryRewriteBar.Show()
		if v.BookRewriteTotal > 0 {
			u.libraryRewriteBar.SetValue(float64(v.BookRewriteDone) / float64(v.BookRewriteTotal))
		}
		u.prepBar.Stop()
		u.prepBar.Hide()
	} else {
		u.libraryRewriteBar.Hide()
		if v.PrepActive || u.busy {
			u.prepBar.Show()
			u.prepBar.Start()
		} else {
			u.prepBar.Stop()
			u.prepBar.Hide()
		}
	}
}

func compareHeading(prevLevel int) string {
	if prevLevel == 0 {
		return "Previous (original)"
	}
	return fmt.Sprintf("Previous (level %d)", prevLevel)
}

func levelLabel(level int) string {
	if level == 0 {
		return "Original"
	}
	return fmt.Sprintf("Easier · level %d/3", level)
}

func (u *readerUI) easier() {
	if u.sess == nil || u.busy {
		return
	}
	u.busy = true
	u.renderPrepBar(session.View{PrepStatus: "Simplifying with Gemma 4…", PrepActive: true})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		v, err := u.sess.Easier(ctx, u.llm)
		fyne.Do(func() {
			u.busy = false
			if err != nil {
				u.showErr(err)
				if u.sess != nil {
					u.render(u.sess.View())
				}
				return
			}
			u.render(v)
			u.persistLibraryPosition()
		})
	}()
}

func (u *readerUI) harder() {
	if u.sess == nil || u.busy {
		return
	}
	u.render(u.sess.Harder())
	u.persistLibraryPosition()
}

func (u *readerUI) next() {
	if u.sess == nil || u.busy {
		return
	}
	v := u.sess.Next()
	if u.bookID != "" {
		u.syncLibraryPrepared(u.bookID)
		v = u.enrichLibraryView(v, u.bookID)
	}
	u.render(v)
	u.prepareAsync()
	u.startPoll()
	u.persistLibraryPosition()
}

func (u *readerUI) prev() {
	if u.sess == nil || u.busy {
		return
	}
	v := u.sess.Prev()
	if u.bookID != "" {
		u.syncLibraryPrepared(u.bookID)
		v = u.enrichLibraryView(v, u.bookID)
	}
	u.render(v)
	u.prepareAsync()
	u.startPoll()
	u.persistLibraryPosition()
}

func (u *readerUI) showErr(err error) {
	dialog.ShowError(err, u.window)
}
