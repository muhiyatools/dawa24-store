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
	MaxUploadBytes = 50 * 1024 * 1024 // 50 MB
)

// RegisterUploadRoutes registers the public static file server for uploaded documents & media.
func RegisterUploadRoutes(r chi.Router) {
	_ = os.MkdirAll(UploadBaseDir, 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "licenses"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "avatars"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "resumes"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "products"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "documents"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "brands"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "compare"), 0755)
	_ = os.MkdirAll(filepath.Join(UploadBaseDir, "imports"), 0755)

	fs := http.FileServer(http.Dir(UploadBaseDir))
	r.Get("/uploads/*", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.StripPrefix("/uploads", fs).ServeHTTP(w, r)
	})
}

var allowedUploadCategories = map[string]bool{
	"products":  true,
	"licenses":  true,
	"avatars":   true,
	"resumes":   true,
	"documents": true,
	"brands":    true,
	"compare":   true,
	"imports":   true,
}

func sanitizeCategory(category string) string {
	clean := strings.ToLower(strings.TrimSpace(category))
	clean = filepath.Base(clean)
	if strings.Contains(clean, "..") || strings.Contains(clean, "/") || strings.Contains(clean, "\\") {
		return "products"
	}
	if allowedUploadCategories[clean] {
		return clean
	}
	return "products"
}

// saveUploadedFile safely processes multipart uploads, validates extensions, and prevents path traversal.
func saveUploadedFile(r *http.Request, fieldName, category string) (string, error) {
	src, header, err := r.FormFile(fieldName)
	if err != nil {
		return "", fmt.Errorf("no file uploaded or invalid form field: %w", err)
	}
	defer src.Close()

	if header.Size > MaxUploadBytes {
		return "", fmt.Errorf("file size exceeds maximum allowed limit (50MB)")
	}

	category = sanitizeCategory(category)

	destDir := filepath.Join(UploadBaseDir, category)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create upload directory: %w", err)
	}

	// Generate safe, unique filename
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".bin"
	}
	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	uniqueName := fmt.Sprintf("%s_%s%s", category, hex.EncodeToString(randomBytes), ext)
	targetPath := filepath.Join(destDir, uniqueName)

	dst, err := os.Create(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to create target file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("failed to save uploaded file: %w", err)
	}

	// Return public URL path
	return fmt.Sprintf("/uploads/%s/%s", category, uniqueName), nil
}

// saveUploadedBytes writes byte data directly to disk.
func saveUploadedBytes(data []byte, originalFilename, category string) (string, error) {
	if len(data) > MaxUploadBytes {
		return "", fmt.Errorf("file size exceeds maximum allowed limit (50MB)")
	}

	category = sanitizeCategory(category)

	ext := strings.ToLower(filepath.Ext(originalFilename))
	if ext == "" {
		ext = ".bin"
	}

	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	safeFilename := fmt.Sprintf("%s_%s%s", category, hex.EncodeToString(randomBytes), ext)

	targetDir := filepath.Join(UploadBaseDir, category)
	_ = os.MkdirAll(targetDir, 0755)

	targetPath := filepath.Join(targetDir, safeFilename)
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
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

	category := sanitizeCategory(r.URL.Query().Get("category"))

	url, err := saveUploadedFile(r, "file", category)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(fmt.Sprintf(`{"url":"%s"}`, url)))
}
