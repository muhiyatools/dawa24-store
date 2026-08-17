package org

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestCreateHighlightSection(t *testing.T) {
	svc := NewService(newMockOrgRepo(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	if _, err := svc.CreateHighlightSection(ctx, 1, i18n.Text{}, ""); err == nil {
		t.Fatal("expected error for empty title")
	}

	sec, err := svc.CreateHighlightSection(ctx, 1, i18n.New("الأكثر مبيعاً", "Best sellers"), "best")
	if err != nil {
		t.Fatalf("CreateHighlightSection failed: %v", err)
	}
	if sec.ID == 0 {
		t.Fatal("expected section id to be set")
	}
	if !sec.IsActive {
		t.Fatal("expected new section to be active")
	}
}
