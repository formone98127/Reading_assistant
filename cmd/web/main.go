// Web UI: go run ./cmd/web  →  http://localhost:8080
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"reading-assistant/internal/config"
	"reading-assistant/internal/server"
	"reading-assistant/internal/simplify"
	"reading-assistant/internal/webfs"
)

func main() {
	cfg := config.Load()
	srv := server.New(cfg, webfs.Handler())

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		llm := &simplify.Client{}
		if config.NormalizeLLMProvider(cfg.LLMProvider) == config.LLMProviderFreebuff {
			llm.Backend = simplify.BackendFreebuff
			llm.BaseURL = cfg.FreebuffURL
			llm.Model = cfg.FreebuffModel
			llm.APIKey = cfg.FreebuffAPIKey
		} else {
			llm.Backend = simplify.BackendOllama
			llm.BaseURL = cfg.OllamaURL
			llm.Model = cfg.OllamaModel
		}
		if err := llm.Warm(ctx); err != nil {
			log.Printf("model warm-up: %v", err)
		}
	}()

	addr := ":" + cfg.Port
	log.Printf("Reading Assistant (web) — open http://localhost%s", addr)
	if config.NormalizeLLMProvider(cfg.LLMProvider) == config.LLMProviderFreebuff {
		log.Printf("FreeBuff: %s  model: %s", cfg.FreebuffURL, cfg.FreebuffModel)
	} else {
		log.Printf("Ollama: %s  model: %s", cfg.OllamaURL, cfg.OllamaModel)
	}
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
