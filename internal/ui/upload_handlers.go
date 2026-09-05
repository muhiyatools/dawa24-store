package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func init() {
	_ = mime.AddExtensionType(".mp4", "video/mp4")
	_ = mime.AddExtensionType(".webm", "video/webm")
	_ = mime.AddExtensionType(".mov", "video/quicktime")
	_ = mime.AddExtensionType(".webp", "image/webp")
}

const (
	UploadBaseDir  = "data/uploads"
	MaxUploadBytes = 50 * 1024 * 1024 // 50 MB

	// uploadMemoryBudget is how much of a multipart request is held in RAM.
	//
	// It is the argument to r.ParseMultipartForm, and that argument is not a
	// size limit — it is how much Go keeps in memory before spilling the rest
	// to a temp file it deletes when the request ends. Passing the ALLOWED
	// UPLOAD SIZE there, which is what every handler on this platform did,
	// reads as "accept files up to 500 MB" and means "hold 500 MB of heap per
	// concurrent request".
	//
	// On the VPS this runs on that is the whole machine. Two vendors uploading
	// price lists at once was enough to push the process into swap, and once it
	// is swapping every OTHER request slows down too — which is why the reports
	// were never limited to the person doing the uploading.
	//
	// Four megabytes. Large enough that ordinary form fields and small images
	// never touch the disk, small enough that a hundred concurrent uploads cost
	// less memory than one used to. The size limit is a separate thing and is
	// enforced with http.MaxBytesReader, which is what actually rejects an
	// oversized file.
	uploadMemoryBudget = 4 << 20

	// maxImportBatchBytes bounds a whole spreadsheet-import request.
	//
	// This is the limit that was missing. The figure the handlers used to pass
	// to ParseMultipartForm looked like one and was not — it governed memory,
	// never size — so the import endpoints accepted a request of ANY length and
	// then held all of it. A cap has to exist somewhere, and the body is where
	// it costs nothing: an oversized request is refused before a byte of it is
	// parsed, rather than after the machine has swallowed it.
	//
	// Two hundred megabytes covers the largest real batch by a wide margin: the
	// compare tool takes at most ten files and a twenty-thousand-row price list
	// is a few megabytes.
	maxImportBatchBytes int64 = 200 << 20
)

// parseImportUpload reads a spreadsheet-import request: size bounded at the
// body, memory bounded at the form.
//
// Both halves matter and they are different limits. MaxBytesReader refuses an
// oversized request outright; uploadMemoryBudget decides how much of an
// acceptable one is held in RAM rather than spilled to a temp file the server
// deletes when the request ends.
func parseImportUpload(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBatchBytes)
	return r.ParseMultipartForm(uploadMemoryBudget)
}

// GetUploadBaseDir returns the configured uploads directory, respecting the
// UPLOAD_DIR and DATA_DIR environment variables.
//
// Whatever this returns is the only place an upload is written. Until now every
// write was mirrored into the relative "data/uploads" as well, which meant
// configuration could redirect reads but never writes: a test run with
// UPLOAD_DIR pointed at a temporary directory still wrote into the working
// tree. Since internal/ui is where the tests run, that filled the repository
// with uploaded files, 166 of which ended up committed and were then deleted
// and rewritten by every subsequent `go test ./...`.
func GetUploadBaseDir() string {
	if dir := os.Getenv("UPLOAD_DIR"); strings.TrimSpace(dir) != "" {
		return strings.TrimSpace(dir)
	}
	if dataDir := os.Getenv("DATA_DIR"); strings.TrimSpace(dataDir) != "" {
		return filepath.Join(strings.TrimSpace(dataDir), "uploads")
	}
	return UploadBaseDir
}

// RegisterUploadRoutes registers the public static file server for uploaded documents & media.
func RegisterUploadRoutes(r chi.Router) {
	baseDir := GetUploadBaseDir()
	_ = os.MkdirAll(baseDir, 0755)
	for cat := range allowedUploadCategories {
		_ = os.MkdirAll(filepath.Join(baseDir, cat), 0755)
	}

	r.Get("/uploads/*", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("Accept-Ranges", "bytes")
		rctx := chi.RouteContext(r.Context())
		path := rctx.URLParam("*")
		cleanPath := filepath.Clean(filepath.FromSlash(path))
		if strings.Contains(cleanPath, "..") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ext := strings.ToLower(filepath.Ext(cleanPath))
		switch ext {
		case ".mp4":
			w.Header().Set("Content-Type", "video/mp4")
		case ".webm":
			w.Header().Set("Content-Type", "video/webm")
		case ".mov":
			w.Header().Set("Content-Type", "video/quicktime")
		}

		fullPath := filepath.Join(baseDir, cleanPath)
		if _, err := os.Stat(fullPath); err == nil {
			http.ServeFile(w, r, fullPath)
			return
		}
		// Resilient fallback check in default relative directory
		if baseDir != "data/uploads" {
			fallbackPath := filepath.Join("data/uploads", cleanPath)
			if _, err := os.Stat(fallbackPath); err == nil {
				http.ServeFile(w, r, fallbackPath)
				return
			}
		}
		http.NotFound(w, r)
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
	"receipts":  true,
	"ads":       true,
	"offers":    true,
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
	baseDir := GetUploadBaseDir()

	destDir := filepath.Join(baseDir, category)
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

// uploadedFileMeta describes a stored upload for document rows.
type uploadedFileMeta struct {
	URL          string
	OriginalName string
	MimeType     string
	SizeBytes    int64
}

// saveUploadedFileFull stores one multipart upload and returns its public URL
// together with the original filename, detected MIME type and byte size, read
// from the single FormFile call. Callers that only need the URL keep using
// saveUploadedFile; callers that persist document rows need the metadata so
// previews (image-vs-PDF sniff, filenames) keep working.
func saveUploadedFileFull(r *http.Request, fieldName, category string) (uploadedFileMeta, error) {
	var meta uploadedFileMeta
	src, header, err := r.FormFile(fieldName)
	if err != nil {
		return meta, fmt.Errorf("no file uploaded or invalid form field: %w", err)
	}
	defer src.Close()

	if header.Size > MaxUploadBytes {
		return meta, fmt.Errorf("file size exceeds maximum allowed limit (50MB)")
	}

	category = sanitizeCategory(category)
	baseDir := GetUploadBaseDir()

	destDir := filepath.Join(baseDir, category)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return meta, fmt.Errorf("failed to create upload directory: %w", err)
	}

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
		return meta, fmt.Errorf("failed to create target file: %w", err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		return meta, fmt.Errorf("failed to save uploaded file: %w", err)
	}

	meta.URL = fmt.Sprintf("/uploads/%s/%s", category, uniqueName)
	meta.OriginalName = header.Filename
	meta.SizeBytes = written
	if ct := header.Header.Get("Content-Type"); ct != "" && ct != "application/octet-stream" {
		meta.MimeType = ct
	} else if mt := mime.TypeByExtension(ext); mt != "" {
		meta.MimeType = mt
	}
	return meta, nil
}

// saveUploadedBytes writes byte data directly to disk.
func saveUploadedBytes(data []byte, originalFilename, category string) (string, error) {
	if len(data) > MaxUploadBytes {
		return "", fmt.Errorf("file size exceeds maximum allowed limit (50MB)")
	}

	category = sanitizeCategory(category)
	baseDir := GetUploadBaseDir()

	ext := strings.ToLower(filepath.Ext(originalFilename))
	if ext == "" {
		ext = ".bin"
	}

	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	safeFilename := fmt.Sprintf("%s_%s%s", category, hex.EncodeToString(randomBytes), ext)

	targetDir := filepath.Join(baseDir, category)
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
