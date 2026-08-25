package aicapabilities_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// Black-hole resiliency for batch adjudication.
//
// RFC 5737 reserves 192.0.2.0/24 for documentation, so nothing routes there: a
// request to it hangs until the timeout rather than failing fast, which is the
// harder case. A pharmacy must be able to place an order while the Gateway is
// unreachable, so the only acceptable outcome is a prompt error the caller can
// degrade on — never a panic, and never an indefinite wait.
func TestAdjudicateBatchSurvivesAnUnroutableGateway(t *testing.T) {
	cfg := config.Gateway{
		BaseURL:    "https://192.0.2.1",
		VirtualKey: "sk-virt-test",
		Enabled:    true,
		Timeout:    2 * time.Second,
	}
	client := gateway.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc := aicapabilities.NewService(client, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = svc.AdjudicateBatch(ctx, []aicapabilities.AdjudicateItem{{
			LineID: 1,
			Text:   "بانادول اكسترا",
			Candidates: []aicapabilities.AdjudicateCandidate{
				{ProductID: 10, Name: "بانادول اكسترا 500 مجم"},
			},
		}})
	}()

	select {
	case <-done:
		if err == nil {
			t.Fatal("an unroutable gateway must produce an error, not a silent success")
		}
	case <-time.After(25 * time.Second):
		t.Fatal("AdjudicateBatch hung against an unroutable gateway; the caller can never degrade")
	}
}

// A disabled gateway is refused immediately rather than attempted.
func TestAdjudicateBatchRefusesWithoutAGateway(t *testing.T) {
	svc := aicapabilities.NewService(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := svc.AdjudicateBatch(context.Background(), []aicapabilities.AdjudicateItem{{LineID: 1}}); err == nil {
		t.Fatal("expected an error when no gateway is configured")
	}
}

// An empty batch is a no-op, not a wasted request.
func TestAdjudicateBatchOfNothingMakesNoRequest(t *testing.T) {
	svc := aicapabilities.NewService(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	decisions, err := svc.AdjudicateBatch(context.Background(), nil)
	if err != nil || decisions != nil {
		t.Fatalf("an empty batch should be a no-op, got %v / %v", decisions, err)
	}
}
