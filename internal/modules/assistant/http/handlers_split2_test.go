package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
)

func TestPhase5_AttachmentValidationAndHandles(t *testing.T) {
	gw := &mockGateway{}
	svc := assistant.NewService(nil, gw, slog.Default())
	handler := NewHandler(svc, gw, nil, slog.Default())

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// T5.1: Upload file and get user-scoped handle
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "prescription.pdf")
	part.Write([]byte("%PDF-1.4 prescription content"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/assistant/attachments", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.WithContext(authContext(50, 10))
	req = req.WithContext(authContext(50, 10))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 upload, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Attachments []struct {
			Handle   string `json:"handle"`
			Filename string `json:"filename"`
			MIMEType string `json:"mime_type"`
		} `json:"attachments"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Attachments) != 1 {
		t.Fatalf("expected 1 attachment handle, got %d", len(resp.Attachments))
	}
	handle := resp.Attachments[0].Handle

	// T5.4: Another user attempting to stream with user 50's handle gets 0 resolved attachments
	handler.handleMu.RLock()
	att, exists := handler.handles[handle]
	handler.handleMu.RUnlock()
	if !exists || att.UserID != 50 {
		t.Fatalf("handle not stored properly: %+v", att)
	}

	// T5.3: Executable file rejected
	var exeBody bytes.Buffer
	exeWriter := multipart.NewWriter(&exeBody)
	exePart, _ := exeWriter.CreateFormFile("file", "malware.exe")
	exePart.Write([]byte("MZ..."))
	exeWriter.Close()

	exeReq := httptest.NewRequest("POST", "/api/v1/assistant/attachments", &exeBody)
	exeReq.Header.Set("Content-Type", exeWriter.FormDataContentType())
	exeReq = exeReq.WithContext(authContext(50, 10))
	exeRec := httptest.NewRecorder()

	r.ServeHTTP(exeRec, exeReq)
	if exeRec.Code != http.StatusBadRequest {
		t.Errorf("T5.3 failed: expected 400 rejected executable, got %d", exeRec.Code)
	}
}

func TestPhase6_VoiceTranscribeEndpoint(t *testing.T) {
	// T6.2: Transcribe endpoint sends audio to Gateway and returns transcript
	gw := &mockGateway{
		transOutput: "طلب دواء بروفين 400",
	}
	svc := assistant.NewService(nil, gw, slog.Default())
	handler := NewHandler(svc, gw, nil, slog.Default())

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "voice.webm")
	part.Write([]byte("fake-audio-bytes"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/assistant/transcribe", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(authContext(50, 10))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.Text != "طلب دواء بروفين 400" {
		t.Errorf("T6.2 failed: expected 'طلب دواء بروفين 400', got %q", res.Text)
	}
}

func TestPhase7_ConversationsListAndHistoryEndpoints(t *testing.T) {
	repo := newMemoryRepo()
	_ = repo.CreateConversation(context.Background(), &assistant.Conversation{
		OrganizationID: 10,
		UserID:         50,
		Title:          "استفسار عن أسعار البنادول",
	})
	_ = repo.SaveMessage(context.Background(), &assistant.Message{
		ConversationID: 1,
		OrganizationID: 10,
		Role:           "user",
		Content:        "ما هو سعر بنادول إكسترا؟",
	})
	_ = repo.SaveMessage(context.Background(), &assistant.Message{
		ConversationID: 1,
		OrganizationID: 10,
		Role:           "assistant",
		Content:        "سعر بنادول إكسترا هو 50 جنيهاً.",
	})

	svc := assistant.NewService(repo, &mockGateway{}, slog.Default())
	handler := NewHandler(svc, &mockGateway{}, repo, slog.Default())
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// 1. List conversations
	req := httptest.NewRequest("GET", "/api/v1/assistant/conversations", nil)
	req = req.WithContext(authContext(50, 10))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 list conversations, got %d", rec.Code)
	}

	var listResp struct {
		Conversations []assistant.Conversation `json:"conversations"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(listResp.Conversations))
	}
	if listResp.Conversations[0].Title != "استفسار عن أسعار البنادول" {
		t.Errorf("unexpected conversation title: %s", listResp.Conversations[0].Title)
	}

	// 2. Fetch conversation history
	reqHist := httptest.NewRequest("GET", "/api/v1/assistant/conversations/1", nil)
	reqHist = reqHist.WithContext(authContext(50, 10))
	recHist := httptest.NewRecorder()
	r.ServeHTTP(recHist, reqHist)

	if recHist.Code != http.StatusOK {
		t.Fatalf("expected 200 history, got %d", recHist.Code)
	}
	var histResp struct {
		Conversation assistant.Conversation `json:"conversation"`
		Messages     []assistant.Message    `json:"messages"`
	}
	_ = json.Unmarshal(recHist.Body.Bytes(), &histResp)
	if len(histResp.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(histResp.Messages))
	}

	// 3. Unauthorized access from another user
	reqOther := httptest.NewRequest("GET", "/api/v1/assistant/conversations/1", nil)
	reqOther = reqOther.WithContext(authContext(999, 10))
	recOther := httptest.NewRecorder()
	r.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusNotFound {
		t.Errorf("expected 404 for another user's conversation, got %d", recOther.Code)
	}
}

func TestPhase8_DeleteConversationEndpoint(t *testing.T) {
	repo := newMemoryRepo()
	_ = repo.CreateConversation(context.Background(), &assistant.Conversation{
		OrganizationID: 10,
		UserID:         50,
		Title:          "محادثة للحذف",
	})

	svc := assistant.NewService(repo, &mockGateway{}, slog.Default())
	handler := NewHandler(svc, &mockGateway{}, repo, slog.Default())
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("DELETE", "/api/v1/assistant/conversations/1", nil)
	req = req.WithContext(authContext(50, 10))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", rec.Code)
	}

	conv, _ := repo.GetConversation(context.Background(), 1)
	if conv != nil {
		t.Errorf("expected conversation 1 to be deleted from repo")
	}
}
