package assistant

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

type mockGateway struct {
	mu           sync.Mutex
	streamCalls  int
	calledRoles  []gateway.Role
	primaryCaps  gateway.ModelCapabilities
	attachCaps   gateway.ModelCapabilities
	streamOutput string
}

func (m *mockGateway) Invoke(ctx context.Context, req gateway.Request) (*gateway.Response, error) {
	return nil, nil
}

func (m *mockGateway) Stream(ctx context.Context, req gateway.ChatRequest) (<-chan gateway.StreamEvent, error) {
	m.mu.Lock()
	m.streamCalls++
	m.calledRoles = append(m.calledRoles, req.Role)
	m.mu.Unlock()

	ch := make(chan gateway.StreamEvent, 10)
	out := m.streamOutput
	if out == "" {
		out = "تحليل افتراضي للملف"
	}
	ch <- gateway.StreamEvent{Delta: out}
	ch <- gateway.StreamEvent{Done: true}
	close(ch)
	return ch, nil
}

func (m *mockGateway) Transcribe(ctx context.Context, audio io.Reader, filename, mime string) (string, error) {
	return "تفريغ صوتي", nil
}

func (m *mockGateway) Capabilities(ctx context.Context, role gateway.Role) (gateway.ModelCapabilities, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if role == gateway.RolePrimary {
		return m.primaryCaps, nil
	}
	if role == gateway.RoleAttachment {
		return m.attachCaps, nil
	}
	return gateway.ConservativeDefaultCapabilities(), nil
}

func (m *mockGateway) Health(ctx context.Context) error {
	return nil
}

func (m *mockGateway) Enabled() bool {
	return true
}

func TestPhase3_RoutingTableAndPrePass(t *testing.T) {
	gw := &mockGateway{
		primaryCaps: gateway.ModelCapabilities{
			Vision:          true,
			Video:           true,
			Documents:       false, // Qwen has no docs
			Audio:           false, // Qwen has no audio
			MaxAttachmentMB: 10,
		},
		attachCaps: gateway.ModelCapabilities{
			Vision:          false,
			Video:           false,
			Documents:       true, // Voxtral takes docs
			Audio:           true, // Voxtral takes audio
			MaxAttachmentMB: 25,
		},
	}

	svc := NewService(nil, gw, slog.Default())

	// T3.1: Image + video -> DirectParts, PrePass empty, attachment model NEVER called
	imgAtt := Attachment{
		Filename: "photo.png",
		MIMEType: "image/png",
		SizeMB:   1.5,
		DataURL:  "data:image/png;base64,123",
	}
	vidAtt := Attachment{
		Filename: "demo.mp4",
		MIMEType: "video/mp4",
		SizeMB:   4.0,
		DataURL:  "https://example.com/demo.mp4",
	}

	msgs, plan, err := svc.BuildTurn(context.Background(), 0, "ما هذا الدواء؟", []Attachment{imgAtt, vidAtt}, nil)
	if err != nil {
		t.Fatalf("BuildTurn failed: %v", err)
	}

	if len(plan.DirectParts) != 2 {
		t.Errorf("T3.1 failed: expected 2 direct parts, got %d", len(plan.DirectParts))
	}
	if len(plan.PrePass) != 0 {
		t.Errorf("T3.1 failed: expected 0 prepass attachments, got %d", len(plan.PrePass))
	}
	if gw.streamCalls != 0 {
		t.Errorf("T3.1 failed: expected ZERO attachment model calls, got %d", gw.streamCalls)
	}

	// T3.2 & T3.3: PDF and Audio -> PrePass, digest rendered in user message
	pdfAtt := Attachment{
		Filename:    "prescription.pdf",
		MIMEType:    "application/pdf",
		SizeMB:      2.0,
		ContentHash: "hash-pdf-1",
	}
	audioAtt := Attachment{
		Filename:    "voice.wav",
		MIMEType:    "audio/wav",
		SizeMB:      1.0,
		ContentHash: "hash-audio-1",
	}

	msgs2, plan2, err2 := svc.BuildTurn(context.Background(), 0, "اشرح الروشتة", []Attachment{pdfAtt, audioAtt}, nil)
	if err2 != nil {
		t.Fatalf("BuildTurn with PDF failed: %v", err2)
	}

	if len(plan2.PrePass) != 2 {
		t.Errorf("T3.2/T3.3 failed: expected 2 prepass attachments, got %d", len(plan2.PrePass))
	}
	if gw.streamCalls != 2 {
		t.Errorf("T3.2/T3.3 failed: expected 2 attachment model calls, got %d", gw.streamCalls)
	}

	userMsgText := msgs2[len(msgs2)-1].Text
	if !strings.Contains(userMsgText, "[مرفق: prescription.pdf — تحليل]") {
		t.Errorf("T3.2 failed: missing PDF digest block in user message: %s", userMsgText)
	}
	if !strings.Contains(userMsgText, "[مرفق: voice.wav — تحليل]") {
		t.Errorf("T3.3 failed: missing audio digest block in user message: %s", userMsgText)
	}

	// T3.5: Oversize -> Rejected before any model call
	overAtt := Attachment{
		Filename: "huge.pdf",
		MIMEType: "application/pdf",
		SizeMB:   100.0,
	}
	callsBefore := gw.streamCalls
	planOver, err := svc.PlanTurn(context.Background(), []Attachment{overAtt})
	if err != nil {
		t.Fatalf("PlanTurn error: %v", err)
	}
	if len(planOver.Rejected) != 1 || planOver.Rejected[0].Reason != "too_large" {
		t.Errorf("T3.5 failed: expected rejected too_large, got %+v", planOver.Rejected)
	}
	if gw.streamCalls != callsBefore {
		t.Errorf("T3.5 failed: expected zero model calls on oversize check, made %d", gw.streamCalls-callsBefore)
	}

	// T3.6: Unknown MIME -> Rejected unsupported_type
	badAtt := Attachment{
		Filename: "script.exe",
		MIMEType: "application/x-msdownload",
		SizeMB:   1.0,
	}
	planBad, err := svc.PlanTurn(context.Background(), []Attachment{badAtt})
	if err != nil {
		t.Fatalf("PlanTurn error: %v", err)
	}
	if len(planBad.Rejected) != 1 || planBad.Rejected[0].Reason != "unsupported_type" {
		t.Errorf("T3.6 failed: expected rejected unsupported_type, got %+v", planBad.Rejected)
	}

	// T3.8: Cache digest by content hash
	callsBefore = gw.streamCalls
	_, _, _ = svc.BuildTurn(context.Background(), 0, "سؤال مكرر", []Attachment{pdfAtt}, nil)
	if gw.streamCalls != callsBefore {
		t.Errorf("T3.8 failed: expected cached digest reuse (0 new calls), got %d new calls", gw.streamCalls-callsBefore)
	}

	_ = msgs
}

func TestPhase3_SystemPromptSafety(t *testing.T) {
	// T3.10: Prompt contains safety and no-access sections
	if !strings.Contains(DefaultSystemPrompt, "كبسولة") {
		t.Errorf("missing identity in DefaultSystemPrompt")
	}
	if !strings.Contains(DefaultSystemPrompt, "ليس لديك أي صلاحية وصول مباشر لقواعد بيانات") {
		t.Errorf("missing no-system-access invariant in DefaultSystemPrompt")
	}
	if !strings.Contains(DefaultSystemPrompt, "السلامة الطبية والدوائية") {
		t.Errorf("missing medical safety section in DefaultSystemPrompt")
	}
	if !strings.Contains(DefaultSystemPrompt, "لست بديلاً عن الطبيب") {
		t.Errorf("missing doctor substitute warning in DefaultSystemPrompt")
	}
	if SystemPromptVersion == "" {
		t.Errorf("SystemPromptVersion must not be empty")
	}
}

func TestPhase3_DigestTruncationFlag(t *testing.T) {
	// T3.9: Digest over DigestMaxChars is truncated and flagged
	gw := &mockGateway{
		attachCaps:   gateway.ModelCapabilities{Documents: true, MaxAttachmentMB: 10},
		streamOutput: strings.Repeat("نص طويل جداً ", 1000), // > 6000 chars
	}
	svc := NewService(nil, gw, slog.Default())

	att := Attachment{
		Filename:    "large.pdf",
		MIMEType:    "application/pdf",
		SizeMB:      1.0,
		ContentHash: "hash-large",
	}

	digests, err := svc.ExecutePrePass(context.Background(), []Attachment{att}, nil)
	if err != nil {
		t.Fatalf("ExecutePrePass error: %v", err)
	}
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(digests))
	}
	if !digests[0].Truncated {
		t.Errorf("T3.9 failed: expected Truncated=true")
	}
	if len(digests[0].Summary) > DigestMaxChars {
		t.Errorf("T3.9 failed: summary length %d exceeds %d", len(digests[0].Summary), DigestMaxChars)
	}
	block := digests[0].RenderBlock()
	if !strings.Contains(block, "تم اقتصاص التحليل") {
		t.Errorf("T3.9 failed: rendered block does not mention truncation: %s", block)
	}
}
