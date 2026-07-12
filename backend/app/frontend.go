package app

import (
	"io/fs"
	"net/http"
	"strings"
)

type FrontendMode string

const (
	FrontendEmbedded FrontendMode = "embedded"
	FrontendDisabled FrontendMode = "disabled"
)

type FrontendConfig struct {
	Mode   FrontendMode
	Assets fs.FS
}

func registerFrontendRoutes(router interface {
	Handle(pattern string, h http.Handler)
	Get(pattern string, h http.HandlerFunc)
}, cfg FrontendConfig) {
	if cfg.Mode != FrontendEmbedded || cfg.Assets == nil {
		return
	}

	router.Handle("/assets/*", http.FileServer(http.FS(cfg.Assets)))
	router.Get("/*", spaHandler(cfg.Assets))
}

func spaHandler(frontend fs.FS) http.HandlerFunc {
	index, err := fs.ReadFile(frontend, "index.html")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(frontend))

	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			writeIndex(w, index)
			return
		}
		if _, err := fs.Stat(frontend, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		writeIndex(w, index)
	}
}

func writeIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}
