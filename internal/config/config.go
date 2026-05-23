package config

import (
	"log"
	"os"
	"path/filepath"
)

const DefaultOllamaModel = "gemma4:latest"

type Config struct {
	OllamaURL   string
	OllamaModel string
	Port        string
	SaveDir     string
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
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = DefaultOllamaModel
		if resolved, err := ResolveOllamaModel(url); err == nil {
			model = resolved
		} else {
			log.Printf("ollama not reachable (%v); will try %q anyway", err, model)
		}
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

	log.Printf("using Ollama model %q", model)
	return Config{
		OllamaURL:   url,
		OllamaModel: model,
		Port:        port,
		SaveDir:     saveDir,
	}
}
