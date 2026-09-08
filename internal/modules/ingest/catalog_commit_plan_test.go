package ingest

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// The review screen's figures are the commit's own decisions, counted. These
// pin that the plan says the same thing the run then does, per mode — which is
// the whole reason it shares decide() rather than describing the mode in prose.
func TestPreviewCommitCountsWhatEachModeWouldWrite(t *testing.T) {
	ctx := context.Background()
	existing := int64(202)
	fresh := int64(303)

	// One row about a product the vendor already stocks, one about a product
	// they do not.
	rows := []*RowOutcome{
		stagedRow(1, &existing, "SKU-HAVE", "Existing item", 3, true),
		stagedRow(2, &fresh, "SKU-NEW", "New item", 5, true),
	}
	keys := []catalog.VariantKey{
		{ID: 501, ProductID: existing, SKU: "SKU-HAVE", Active: true},
		{ID: 502, ProductID: 909, SKU: "SKU-UNRELATED", Active: true},
	}

	cases := []struct {
		mode        Mode
		wantInsert  int
		wantUpdate  int
		wantSkipped int
		wantRetire  int
	}{
		{ModeUpsert, 1, 1, 0, 0},
		{ModeAddOnly, 1, 0, 1, 0},
		{ModeUpdateOnly, 0, 1, 1, 0},
		// Replace writes both and retires the one variant the file never
		// mentions.
		{ModeReplace, 1, 1, 0, 1},
	}

	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			settings := DefaultSettings()
			settings.WarehouseID = 1
			settings.Mode = tc.mode

			svc, _, _, store := commitFixture(session("imp-plan", settings), rows, keys, nil)
			store.mentions = []RowMention{
				{ProductID: existing, SourceCode: "SKU-HAVE"},
				{ProductID: fresh, SourceCode: "SKU-NEW"},
			}

			plan, err := svc.PreviewCommit(ctx, "imp-plan")
			if err != nil {
				t.Fatalf("PreviewCommit: %v", err)
			}
			if plan.Insert != tc.wantInsert || plan.Update != tc.wantUpdate ||
				plan.SkippedByMode != tc.wantSkipped || plan.Retire != tc.wantRetire {
				t.Errorf("plan = insert %d, update %d, skipped %d, retire %d;"+
					" want insert %d, update %d, skipped %d, retire %d",
					plan.Insert, plan.Update, plan.SkippedByMode, plan.Retire,
					tc.wantInsert, tc.wantUpdate, tc.wantSkipped, tc.wantRetire)
			}
		})
	}
}

// The preview must not write anything, and must not disturb the session it
// previews: a vendor refreshing the review screen would otherwise commit their
// import by looking at it.
func TestPreviewCommitWritesNothing(t *testing.T) {
	ctx := context.Background()
	product := int64(202)
	settings := DefaultSettings()
	settings.WarehouseID = 1
	settings.Mode = ModeReplace

	rows := []*RowOutcome{stagedRow(1, &product, "SKU-HAVE", "Existing item", 3, true)}
	keys := []catalog.VariantKey{
		{ID: 501, ProductID: product, SKU: "SKU-HAVE", Active: true},
		{ID: 502, ProductID: 909, SKU: "SKU-GONE", Active: true},
	}

	svc, cat, inv, store := commitFixture(session("imp-preview", settings), rows, keys, nil)
	store.mentions = []RowMention{{ProductID: product, SourceCode: "SKU-HAVE"}}

	plan, err := svc.PreviewCommit(ctx, "imp-preview")
	if err != nil {
		t.Fatalf("PreviewCommit: %v", err)
	}
	if plan.Retire != 1 {
		t.Errorf("plan.Retire = %d, want 1", plan.Retire)
	}
	if len(cat.writtenVariants) != 0 {
		t.Errorf("preview wrote %d variants", len(cat.writtenVariants))
	}
	if cat.deactivatedExcept != nil {
		t.Error("preview retired variants")
	}
	if len(inv.writtenRows) != 0 {
		t.Errorf("preview wrote %d balances", len(inv.writtenRows))
	}
	if store.finishedCalled {
		t.Error("preview finished the import")
	}
}

// A file that names the same product twice is one variant, not two, and the
// plan has to say so — otherwise the review screen promises an insert the run
// will not make.
func TestPreviewCommitCollapsesRepeatedProducts(t *testing.T) {
	ctx := context.Background()
	product := int64(404)
	settings := DefaultSettings()
	settings.WarehouseID = 1

	rows := []*RowOutcome{
		stagedRow(1, &product, "", "New item", 3, true),
		stagedRow(2, &product, "", "New item again", 4, true),
	}

	svc, _, _, _ := commitFixture(session("imp-dup", settings), rows, nil, nil)
	plan, err := svc.PreviewCommit(ctx, "imp-dup")
	if err != nil {
		t.Fatalf("PreviewCommit: %v", err)
	}
	if plan.Insert != 1 || plan.Update != 1 {
		t.Errorf("plan = insert %d, update %d; want one insert and one update",
			plan.Insert, plan.Update)
	}
}
