package gui

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"reading-assistant/internal/config"
)

func (u *readerUI) initOllamaModelSelect(sel *widget.Select) {
	u.modelSelect = sel
	sel.PlaceHolder = "Loading models…"
	sel.Disable()

	current := u.cfg.OllamaModel
	if saved := u.app.Preferences().StringWithFallback("ollamaModel", ""); saved != "" {
		current = saved
		u.llm.Model = current
		u.cfg.OllamaModel = current
	}

	go func() {
		models, err := config.ListOllamaModels(u.cfg.OllamaURL)
		fyne.Do(func() {
			if err != nil {
				sel.Options = []string{current}
				sel.SetSelected(current)
				sel.Enable()
				u.status.SetText("Ollama not reachable — check it is running")
				return
			}
			opts := models
			if current != "" && !containsStr(opts, current) {
				opts = append([]string{current}, opts...)
			}
			if len(opts) == 0 {
				opts = []string{current}
			}
			sel.Options = opts
			sel.SetSelected(current)
			if sel.Selected == "" && len(opts) > 0 {
				sel.SetSelected(opts[0])
				u.applyOllamaModel(opts[0])
			}
			sel.Enable()
			u.status.SetText("")
		})
	}()
}

func (u *readerUI) applyOllamaModel(model string) {
	if model == "" {
		return
	}
	u.llm.Model = model
	u.cfg.OllamaModel = model
	u.app.Preferences().SetString("ollamaModel", model)
	if err := config.SaveUserSettings(u.cfg.SaveDir, config.UserSettings{OllamaModel: model}); err != nil {
		log.Printf("save ollama model: %v", err)
	}
	log.Printf("using Ollama model %q", model)
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
