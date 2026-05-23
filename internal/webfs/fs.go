package webfs

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var content embed.FS

// Handler serves the web UI from the embedded filesystem.
func Handler() http.Handler {
	root, err := fs.Sub(content, "web")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(root))
}
