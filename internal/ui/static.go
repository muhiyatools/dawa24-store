package ui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/ui/layouts"
)

//go:embed static/*
var staticFS embed.FS

type staticAsset struct {
	content     []byte
	etag        string
	contentType string
}

var (
	assetCache = make(map[string]*staticAsset)
	assetOnce  sync.Once
)

func initStaticAssetCache() {
	_ = fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := staticFS.ReadFile(path)
		if err != nil {
			return nil
		}
		hash := sha256.Sum256(data)
		etag := "\"" + hex.EncodeToString(hash[:16]) + "\""

		ext := filepath.Ext(path)
		cType := mime.TypeByExtension(ext)
		if cType == "" {
			switch ext {
			case ".css":
				cType = "text/css; charset=utf-8"
			case ".js":
				cType = "application/javascript; charset=utf-8"
			case ".svg":
				cType = "image/svg+xml"
			case ".png":
				cType = "image/png"
			case ".jpg", ".jpeg":
				cType = "image/jpeg"
			case ".webp":
				cType = "image/webp"
			case ".woff2":
				cType = "font/woff2"
			case ".txt":
				cType = "text/plain; charset=utf-8"
			default:
				cType = "application/octet-stream"
			}
		}

		relPath := strings.TrimPrefix(path, "static")
		if !strings.HasPrefix(relPath, "/") {
			relPath = "/" + relPath
		}

		assetCache[relPath] = &staticAsset{
			content:     data,
			etag:        etag,
			contentType: cType,
		}
		return nil
	})
}

// AssetURL returns the public URL for an embedded asset with its content hash
// appended as a cache-busting query parameter.
//
// It replaces a version string that was typed into the layout by hand and had
// already drifted: the stylesheets carried ?v=2026082903 while app.js carried
// ?v=2026082901, so a CSS change shipped without the JS change that went with
// it, and both relied on someone remembering to edit the layout at all. The
// hash is computed from the file, so it cannot be forgotten and cannot be
// wrong.
//
// path is the URL path, e.g. "/static/css/app.css". An unknown path is returned
// unchanged rather than failing the render — a missing stylesheet is a visible
// bug, and a blank page is not a better way to report it.
func AssetURL(path string) string {
	assetOnce.Do(initStaticAssetCache)

	key := strings.TrimPrefix(path, "/static")
	asset, ok := assetCache[key]
	if !ok {
		return path
	}
	// etag is a quoted hex digest; eight characters distinguish any build.
	digest := strings.Trim(asset.etag, `"`)
	if len(digest) > 8 {
		digest = digest[:8]
	}
	return path + "?v=" + digest
}

// RegisterStaticRoutes mounts embedded static assets with in-RAM caching, strong ETags, and 304 Not Modified support.
func RegisterStaticRoutes(r chi.Router) {
	assetOnce.Do(initStaticAssetCache)

	r.Get("/static/*", func(w http.ResponseWriter, req *http.Request) {
		urlPath := "/" + strings.TrimPrefix(chi.URLParam(req, "*"), "/")
		asset, exists := assetCache[urlPath]
		if !exists {
			http.NotFound(w, req)
			return
		}

		w.Header().Set("Content-Type", asset.contentType)
		w.Header().Set("ETag", asset.etag)
		w.Header().Set("Vary", "Accept-Encoding")

		// Long-term immutable caching for fonts & vendor assets; 24h with stale-while-revalidate for app assets
		if strings.Contains(urlPath, "vendor") || strings.Contains(urlPath, "fonts") || strings.HasSuffix(urlPath, ".woff2") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		}

		// Conditional GET: 304 Not Modified for zero-payload instant responses
		if match := req.Header.Get("If-None-Match"); match != "" {
			if match == asset.etag || match == "*" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(asset.content)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(asset.content)
	})

	r.Get("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
		asset, exists := assetCache["/robots.txt"]
		if !exists {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("ETag", asset.etag)
		if match := req.Header.Get("If-None-Match"); match != "" && (match == asset.etag || match == "*") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(asset.content)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(asset.content)
	})
}

// Layout templates resolve asset URLs through layouts.Asset, which cannot call
// into this package directly without an import cycle. Installing the resolver
// here keeps the hash computation in one place.
func init() {
	layouts.Asset = AssetURL
}
