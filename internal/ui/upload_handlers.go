package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

const (
	UploadBaseDir  = "data/uploads"
	MaxUploadBytes = 20 * 1024 * 1024 // 20 MB
)

// RegisterUploadRoutes registers the public static file server for uploaded documents & media.
func RegisterUploadRoutes(r chi.Router) {
	_ = os.MkdirAll(UploadBaseDir, 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "licenses"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "avatars"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "resumes"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "products"), 0755)

	fs := http.FileServer(http.Dir(UploadBaseDir))
	r.Get("/uploads/*", func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fsHandler := http.StripPrefix(pathPrefix, fs)

		// Set caching & inline header for images/PDFs
		w.Header().Set("Cache-Control", "public, max-age=86400")
		fsHandler.ServeHTTP(w, r)
	})
}

// saveUploadedFile handles parsing a multipart file header and writing it safely to disk.
func saveUploadedFile(r *http.Request, formKey, category string) (string, error) {
	file, header, err := r.FormFile(formKey)
	if err != nil {
		if err == http.ErrMissingFile {
			return "", nil // No file was attached, which is optional or handled by validator
		}
		return "", err
	}
	defer file.Close()

	if header.Size > MaxUploadBytes {
		return "", fmt.Errorf("file size exceeds maximum allowed limit (20MB)")
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".bin"
	}

	// Generate random safe filename
	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	safeFilename := fmt.Sprintf("%s_%s%s", category, hex.EncodeToString(randomBytes), ext)

	targetDir := filepath.Join(UploadBaseDir, category)
	_ = os.MkdirAll(targetDir, 0755)

	targetPath := filepath.Join(targetDir, safeFilename)
	destFile, err := os.Create(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, file); err != nil {
		return "", fmt.Errorf("failed to write file contents: %w", err)
	}

	return fmt.Sprintf("/uploads/%s/%s", category, safeFilename), nil
}

// UploadAPISubmit allows asynchronous HTMX or JavaScript file uploads.
func (h *UIHandler) UploadAPISubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	category := r.URL.Query().Get("category")
	if category == "" {
		category = "products"
	}

	url, err := saveUploadedFile(r, "file", category)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(fmt.Sprintf(`{"url":"%s"}`, url)))
}
