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
		llm := &simplify.Client{BaseURL: cfg.OllamaURL, Model: cfg.OllamaModel}
		if err := llm.Warm(ctx); err != nil {
			log.Printf("model warm-up: %v", err)
		}
	}()

	addr := ":" + cfg.Port
	log.Printf("Reading Assistant (web) — open http://localhost%s", addr)
	log.Printf("Ollama: %s  model: %s", cfg.OllamaURL, cfg.OllamaModel)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
