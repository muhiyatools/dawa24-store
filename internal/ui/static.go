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

		// Allow browser caching with revalidation fallback (1 day for general static assets, 1 year for immutable fonts/vendor scripts)
		if strings.Contains(r.URL.Path, "vendor") || strings.Contains(r.URL.Path, "fonts") || strings.Contains(r.URL.Path, ".woff") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		}
		fsHandler.ServeHTTP(w, r)
	})

	r.Get("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/robots.txt"
		fileServer.ServeHTTP(w, r)
	})
}
