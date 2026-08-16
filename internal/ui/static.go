package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

//go:embed static/*
var staticFS embed.FS

// RegisterStaticRoutes mounts embedded static assets at /static.
func RegisterStaticRoutes(r chi.Router) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(sub))

	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fsHandler := http.StripPrefix(pathPrefix, fileServer)

		// Ensure client always receives up-to-date static assets
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		fsHandler.ServeHTTP(w, r)
	})

	r.Get("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/robots.txt"
		fileServer.ServeHTTP(w, r)
	})
}
