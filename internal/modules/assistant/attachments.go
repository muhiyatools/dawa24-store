package assistant

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

// Allowed MIME classifications
const (
	KindImage    = "image"
	KindAudio    = "audio"
	KindVideo    = "video"
	KindDocument = "document"
	KindUnknown  = "unknown"
)

var allowedMIMEs = map[string]string{
	// Images
	"image/png":  KindImage,
	"image/jpeg": KindImage,
	"image/jpg":  KindImage,
	"image/webp": KindImage,
	"image/gif":  KindImage,

	// Audio
	"audio/mpeg":  KindAudio,
	"audio/mp3":   KindAudio,
	"audio/wav":   KindAudio,
	"audio/x-wav": KindAudio,
	"audio/webm":  KindAudio,
	"audio/ogg":   KindAudio,
	"audio/m4a":   KindAudio,
	"audio/x-m4a": KindAudio,

	// Video
	"video/mp4":  KindVideo,
	"video/webm": KindVideo,

	// Documents
	"application/pdf": KindDocument,
	"text/plain":      KindDocument,
	"text/csv":        KindDocument,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       KindDocument,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": KindDocument,
}

var forbiddenExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".bat": true, ".cmd": true, ".sh": true,
	".zip": true, ".tar": true, ".gz": true, ".rar": true, ".7z": true, ".iso": true,
	".bin": true, ".com": true, ".msi": true, ".js": true, ".vbs": true,
}

// ClassifyMIME returns the kind classification for a MIME type or KindUnknown.
func ClassifyMIME(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if idx := strings.Index(mime, ";"); idx != -1 {
		mime = strings.TrimSpace(mime[:idx])
	}
	if kind, ok := allowedMIMEs[mime]; ok {
		return kind
	}
	return KindUnknown
}

// SniffAndValidate checks file bytes and declared filename against the security allowlist.
func SniffAndValidate(content []byte, declaredFilename string) (string, string, error) {
	if len(content) == 0 {
		return "", "", errors.New("empty file content")
	}

	ext := strings.ToLower(filepath.Ext(declaredFilename))
	if forbiddenExtensions[ext] {
		return "", "", fmt.Errorf("forbidden file extension: %s", ext)
	}

	detectedMIME := http.DetectContentType(content)
	if idx := strings.Index(detectedMIME, ";"); idx != -1 {
		detectedMIME = strings.TrimSpace(detectedMIME[:idx])
	}

	// For specific document formats (like CSV or plain text), DetectContentType returns text/plain
	if ext == ".csv" && (detectedMIME == "text/plain" || detectedMIME == "application/octet-stream") {
		detectedMIME = "text/csv"
	}
	if ext == ".pdf" && (detectedMIME == "application/octet-stream" || strings.HasPrefix(string(content[:min(len(content), 10)]), "%PDF")) {
		detectedMIME = "application/pdf"
	}

	kind := ClassifyMIME(detectedMIME)
	if kind == KindUnknown {
		return "", "", fmt.Errorf("unsupported file type: %s", detectedMIME)
	}

	return detectedMIME, kind, nil
}

// ComputeContentHash returns the SHA256 hex string of content bytes.
func ComputeContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// SanitiseFilename strips directory traversal characters while keeping the safe base name.
func SanitiseFilename(name string) string {
	base := filepath.Base(filepath.Clean(name))
	base = strings.ReplaceAll(base, "\\", "_")
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.TrimSpace(base)
	if base == "" || base == "." {
		return "attachment"
	}
	return base
}
