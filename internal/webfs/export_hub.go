package webfs

import (
	"net/http"
	"strconv"
	"strings"
)

// exportAssets maps URL paths to files under web/.
var exportAssets = map[string]string{
	"/export":      "export.html",
	"/export/":     "export.html",
	"/export.html": "export.html",
	"/export.js":   "export.js",
	"/export.css":  "export.css",
}

// ExportHubHandler serves the export books page and its assets from the embedded FS.
func ExportHubHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		file, ok := exportAssets[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := content.ReadFile("web/" + file)
		if err != nil {
			http.Error(w, "export page missing from build", http.StatusInternalServerError)
			return
		}
		ct := "text/plain; charset=utf-8"
		switch {
		case strings.HasSuffix(file, ".html"):
			ct = "text/html; charset=utf-8"
		case strings.HasSuffix(file, ".js"):
			ct = "application/javascript; charset=utf-8"
		case strings.HasSuffix(file, ".css"):
			ct = "text/css; charset=utf-8"
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
