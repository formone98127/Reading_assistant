package config

import (
	"log"
	"os"
	"path/filepath"

	"reading-assistant/internal/tts"
)

const DefaultOllamaModel = "gemma4:latest"

type Config struct {
	LLMProvider    string
	OllamaURL      string
	OllamaModel    string
	FreebuffURL    string
	FreebuffModel  string
	FreebuffAPIKey string
	VoxCPMURL      string
	VoxCPMControl  string
	VoxCPMCfg      float64
	VoxCPMTimesteps int
	Port           string
	SaveDir        string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if v := os.Getenv("READING_ASSISTANT_PORT"); v != "" {
		port = v
	}
	url := os.Getenv("OLLAMA_URL")
	if url == "" {
		url = "http://127.0.0.1:11434"
	}
	saveDir := os.Getenv("SAVE_DIR")
	if saveDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			saveDir = filepath.Join(home, "reading-assistant", "saves")
		} else {
			saveDir = "saved"
		}
	}
	_ = os.MkdirAll(saveDir, 0o755)

	user := LoadUserSettings(saveDir)

	provider := os.Getenv("LLM_PROVIDER")
	if provider == "" {
		provider = user.LLMProvider
	}
	provider = NormalizeLLMProvider(provider)

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = user.OllamaModel
	}
	if model == "" {
		model = DefaultOllamaModel
		if resolved, err := ResolveOllamaModel(url); err == nil {
			model = resolved
		} else {
			log.Printf("ollama not reachable (%v); will try %q anyway", err, model)
		}
	}

	fbURL := os.Getenv("FREEBUFF_URL")
	if fbURL == "" {
		fbURL = user.FreebuffURL
	}
	if fbURL == "" {
		fbURL = DefaultFreebuffURL
	}
	fbKey := os.Getenv("FREEBUFF_API_KEY")
	if fbKey == "" {
		fbKey = user.FreebuffAPIKey
	}
	fbModel := os.Getenv("FREEBUFF_MODEL")
	if fbModel == "" {
		fbModel = user.FreebuffModel
	}
	if fbModel == "" {
		if resolved, err := ResolveFreebuffModel(fbURL, fbKey); err == nil {
			fbModel = resolved
		} else {
			fbModel = DefaultFreebuffModel
			log.Printf("freebuff not reachable (%v); will try %q — run Freebuff2API on %s", err, fbModel, fbURL)
		}
	} else {
		if resolved, err := PickFreebuffModel(fbModel, fbURL, fbKey); err == nil {
			if resolved != fbModel {
				log.Printf("freebuff model adjusted %q -> %q", fbModel, resolved)
			}
			fbModel = resolved
		} else {
			log.Printf("freebuff model %q: %v", fbModel, err)
			if resolved, err2 := ResolveFreebuffModel(fbURL, fbKey); err2 == nil {
				fbModel = resolved
			} else {
				fbModel = NormalizeFreebuffModel(fbModel)
				log.Printf("freebuff fallback to %q (%v)", fbModel, err2)
			}
		}
	}

	if provider == LLMProviderFreebuff {
		log.Printf("using FreeBuff model %q at %s", fbModel, fbURL)
	} else {
		log.Printf("using Ollama model %q at %s", model, url)
	}

	voxURL := os.Getenv("VOXCPM_URL")
	if voxURL == "" {
		voxURL = user.VoxCPMURL
	}
	if voxURL == "" {
		voxURL = tts.DefaultURL
	}
	voxControl := os.Getenv("VOXCPM_CONTROL")
	if voxControl == "" {
		voxControl = user.VoxCPMControl
	}
	if voxControl == "" {
		voxControl = "clear calm American English narrator, warm steady pace"
	}
	voxCfg := user.VoxCPMCfg
	if voxCfg <= 0 {
		voxCfg = 2.0
	}
	voxSteps := user.VoxCPMTimesteps
	if voxSteps <= 0 {
		voxSteps = 10
	}

	return Config{
		LLMProvider:    provider,
		OllamaURL:      url,
		OllamaModel:    model,
		FreebuffURL:    fbURL,
		FreebuffModel:  fbModel,
		FreebuffAPIKey: fbKey,
		VoxCPMURL:      voxURL,
		VoxCPMControl:  voxControl,
		VoxCPMCfg:      voxCfg,
		VoxCPMTimesteps: voxSteps,
		Port:           port,
		SaveDir:        saveDir,
	}
}
