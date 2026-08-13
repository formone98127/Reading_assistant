package gui

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"reading-assistant/internal/config"
	"reading-assistant/internal/simplify"
)

func applyGUILLM(cfg config.Config, llm *simplify.Client) {
	llm.Backend = simplify.Backend(config.NormalizeLLMProvider(cfg.LLMProvider))
	if llm.Backend == simplify.BackendFreebuff {
		llm.BaseURL = cfg.FreebuffURL
		llm.Model = cfg.FreebuffModel
		llm.APIKey = cfg.FreebuffAPIKey
		return
	}
	llm.Backend = simplify.BackendOllama
	llm.BaseURL = cfg.OllamaURL
	llm.Model = cfg.OllamaModel
	llm.APIKey = ""
}

func (u *readerUI) initLLMSelects(providerSel, modelSel *widget.Select) {
	u.providerSelect = providerSel
	u.modelSelect = modelSel
	providerSel.Options = []string{"Ollama", "FreeBuff"}
	providerSel.OnChanged = func(string) {
		u.onProviderChanged()
	}
	p := config.NormalizeLLMProvider(u.cfg.LLMProvider)
	if p == config.LLMProviderFreebuff {
		providerSel.SetSelected("FreeBuff")
	} else {
		providerSel.SetSelected("Ollama")
	}
	u.refreshModelSelect()
}

func (u *readerUI) onProviderChanged() {
	if u.providerSelect == nil {
		return
	}
	switch u.providerSelect.Selected {
	case "FreeBuff":
		u.cfg.LLMProvider = config.LLMProviderFreebuff
	default:
		u.cfg.LLMProvider = config.LLMProviderOllama
	}
	applyGUILLM(u.cfg, u.llm)
	u.persistLLMSettings()
	u.refreshModelSelect()
}

func (u *readerUI) refreshModelSelect() {
	if u.modelSelect == nil {
		return
	}
	sel := u.modelSelect
	sel.Disable()
	sel.PlaceHolder = "Loading models…"
	provider := config.NormalizeLLMProvider(u.cfg.LLMProvider)
	current := config.LLMModel(u.cfg)

	go func() {
		var models []string
		var err error
		if provider == config.LLMProviderFreebuff {
			raw, err := config.ListOpenAIModels(u.cfg.FreebuffURL, u.cfg.FreebuffAPIKey)
			if err == nil {
				models = config.FilterFreebuffModels(raw)
			}
		} else {
			models, err = config.ListOllamaModels(u.cfg.OllamaURL)
		}
		fyne.Do(func() {
			if err != nil {
				sel.Options = []string{current}
				sel.SetSelected(current)
				sel.Enable()
				label := "Ollama"
				if provider == config.LLMProviderFreebuff {
					label = "FreeBuff proxy"
				}
				u.status.SetText(label + " not reachable — check it is running")
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
				u.applyLLMModel(opts[0])
			}
			sel.Enable()
			u.status.SetText("")
		})
	}()
}

func (u *readerUI) applyLLMModel(model string) {
	if model == "" {
		return
	}
	u.llm.Model = model
	if config.NormalizeLLMProvider(u.cfg.LLMProvider) == config.LLMProviderFreebuff {
		resolved, err := config.PickFreebuffModel(model, u.cfg.FreebuffURL, u.cfg.FreebuffAPIKey)
		if err != nil {
			log.Printf("freebuff model: %v", err)
			u.status.SetText(err.Error())
			return
		}
		u.cfg.FreebuffModel = resolved
		u.app.Preferences().SetString("freebuffModel", model)
	} else {
		u.cfg.OllamaModel = model
		u.app.Preferences().SetString("ollamaModel", model)
	}
	u.persistLLMSettings()
	log.Printf("using %s model %q", u.cfg.LLMProvider, model)
}

func (u *readerUI) persistLLMSettings() {
	user := config.LoadUserSettings(u.cfg.SaveDir)
	user.LLMProvider = u.cfg.LLMProvider
	user.OllamaModel = u.cfg.OllamaModel
	user.FreebuffURL = u.cfg.FreebuffURL
	user.FreebuffModel = u.cfg.FreebuffModel
	user.FreebuffAPIKey = u.cfg.FreebuffAPIKey
	if err := config.SaveUserSettings(u.cfg.SaveDir, user); err != nil {
		log.Printf("save llm settings: %v", err)
	}
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
