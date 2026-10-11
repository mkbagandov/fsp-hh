package httptransport

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// spaHandler раздаёт собранный фронтенд (web/dist): существующие файлы — как есть
// (хэшированные ассеты с долгим кэшем), остальные пути — index.html, чтобы работал
// клиентский роутер. Пути с «..» и запросы к каталогам не обслуживаются.
func spaHandler(dir string) http.Handler {
	root := filepath.Clean(dir)
	index := filepath.Join(root, "index.html")
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeProblem(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		clean := path.Clean("/" + r.URL.Path)
		if strings.Contains(clean, "..") {
			writeProblem(w, http.StatusBadRequest, "Bad Request")
			return
		}
		full := filepath.Join(root, filepath.FromSlash(clean))
		if rel, err := filepath.Rel(root, full); err != nil || strings.HasPrefix(rel, "..") {
			writeProblem(w, http.StatusBadRequest, "Bad Request")
			return
		}
		if st, err := os.Stat(full); err == nil && !st.IsDir() { //nolint:gosec // путь очищен path.Clean и проверен filepath.Rel относительно корня
			if strings.HasSuffix(clean, ".html") {
				w.Header().Set("Content-Security-Policy", spaCSP)
			}
			if strings.HasPrefix(clean, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=3600")
			}
			r.URL.Path = clean
			fs.ServeHTTP(w, r)
			return
		}
		// SPA-fallback: любой «страничный» путь отдаёт index.html без кэша.
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy", spaCSP)
		http.ServeFile(w, r, index)
	})
}
