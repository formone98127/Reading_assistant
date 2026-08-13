package webfs

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web/*
var content embed.FS

// Handler serves the web UI from the embedded filesystem (no long-lived cache).
func Handler() http.Handler {
	root, err := fs.Sub(content, "web")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			p := r.URL.Path
			if p == "/" || p == "" || strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".css") {
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
				w.Header().Set("Pragma", "no-cache")
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
