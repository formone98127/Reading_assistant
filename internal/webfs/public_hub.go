package webfs

import (
	"net/http"
	"strconv"
	"strings"
)

var publicAssets = map[string]string{
	"/public":      "public.html",
	"/public/":     "public.html",
	"/public.html": "public.html",
	"/public.js":   "public.js",
}

// PublicHubHandler serves the public upload/share page from the embedded FS.
func PublicHubHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		file, ok := publicAssets[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := content.ReadFile("web/" + file)
		if err != nil {
			http.Error(w, "public page missing from build", http.StatusInternalServerError)
			return
		}
		ct := "text/plain; charset=utf-8"
		switch {
		case strings.HasSuffix(file, ".html"):
			ct = "text/html; charset=utf-8"
		case strings.HasSuffix(file, ".js"):
			ct = "application/javascript; charset=utf-8"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "no-cache")
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			return
		}
		_, _ = w.Write(data)
	})
}
