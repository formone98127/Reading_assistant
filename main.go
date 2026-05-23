package main

import (
	"log"

	"reading-assistant/internal/config"
	"reading-assistant/internal/gui"
)

func main() {
	cfg := config.Load()
	log.Printf("Reading Assistant (desktop)")
	log.Printf("Ollama: %s  model: %s", cfg.OllamaURL, cfg.OllamaModel)
	gui.Run(cfg)
}
