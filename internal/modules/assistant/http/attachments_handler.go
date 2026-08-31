package http

import (
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// AssistantUpload handles file uploads, validation, and opaque handle issuance.
func (h *Handler) AssistantUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, `{"error":"file_too_large","message":i18n.TDefault("w4_mod.10_61")}`, http.StatusRequestEntityTooLarge)
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 {
		http.Error(w, `{"error":"no_files_provided"}`, http.StatusBadRequest)
		return
	}
	if len(files) > 5 {
		http.Error(w, `{"error":"too_many_files","message":i18n.TDefault("w4_mod.5_231")}`, http.StatusBadRequest)
		return
	}

	type handleResp struct {
		Handle   string  `json:"handle"`
		Filename string  `json:"filename"`
		MIMEType string  `json:"mime_type"`
		SizeMB   float64 `json:"size_mb"`
	}

	var results []handleResp
	for _, fh := range files {
		if fh.Size > 10<<20 {
			http.Error(w, `{"error":"file_too_large","message":i18n.TDefault("w4_mod.s_232_232")}`, http.StatusRequestEntityTooLarge)
			return
		}

		f, err := fh.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			continue
		}

		mime, _, err := assistant.SniffAndValidate(content, fh.Filename)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"unsupported_type","message":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		safeName := assistant.SanitiseFilename(fh.Filename)
		handle := uuid.New().String()
		hash := assistant.ComputeContentHash(content)
		sizeMB := float64(len(content)) / (1024 * 1024)
		dataURL := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(content))

		att := assistant.Attachment{
			Handle:      handle,
			Filename:    safeName,
			MIMEType:    mime,
			SizeMB:      sizeMB,
			DataURL:     dataURL,
			ContentHash: hash,
			UserID:      actor.UserID,
			OrgID:       actor.OrgID,
		}

		h.handleMu.Lock()
		h.handles[handle] = att
		h.handleMu.Unlock()

		results = append(results, handleResp{
			Handle:   handle,
			Filename: safeName,
			MIMEType: mime,
			SizeMB:   sizeMB,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"attachments": results,
	})
}

// AssistantTranscribe falls back to Whisper-1 when browser native speech is absent.
func (h *Handler) AssistantTranscribe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"missing_audio_file"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	transcript, err := h.gwClient.Transcribe(ctx, file, header.Filename, header.Header.Get("Content-Type"))
	if err != nil {
		h.log.WarnContext(ctx, "transcription error", "error", err)
		http.Error(w, `{"error":"transcribe_failed","message":i18n.TDefault("w4_mod.w4str_62_62")}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"text": transcript,
	})
}

// AssistantHealth reports the live connectivity status of the Gateway.
func (h *Handler) AssistantHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if !h.gwClient.Enabled() {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "disabled"})
		return
	}

	if err := h.gwClient.Health(ctx); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "degraded"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "connected"})
}

// AssistantConversations lists recent sessions for the authenticated user.
func (h *Handler) AssistantConversations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"conversations": []any{}})
		return
	}

	convs, err := h.repo.ListConversations(ctx, actor.OrgID, actor.UserID, 30, 0)
	if err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	if convs == nil {
		convs = []*assistant.Conversation{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"conversations": convs})
}

// AssistantHistory returns message turns for a conversation.
func (h *Handler) AssistantHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	convID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || convID <= 0 {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}

	conv, err := h.repo.GetConversation(ctx, convID)
	if err != nil || conv == nil || (conv.OrganizationID != actor.OrgID && !actor.IsStaff) || conv.UserID != actor.UserID {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}

	msgs, err := h.repo.ListMessages(ctx, convID, 50)
	if err != nil {
		http.Error(w, `{"error":"db_error"}`, http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []*assistant.Message{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"conversation": conv,
		"messages":     msgs,
	})
}

// AssistantDeleteConversation soft-deletes a conversation belonging to the authenticated caller.
func (h *Handler) AssistantDeleteConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	convID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || convID <= 0 {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	if h.repo != nil {
		_ = h.repo.DeleteConversation(ctx, convID, actor.OrgID, actor.UserID)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}
