package assistant

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The collector has to find rows wherever a tool happened to put them, because
// the tools do not agree on a shape: a listing returns a Page inside a map, a
// detail returns a struct with the row as a field, and both are legitimate.
func TestCollectEntitiesFindsRowsAtAnyDepth(t *testing.T) {
	data := map[string]any{
		"orders": []PurchaseOrderRow{
			{ID: 7, Number: "PO-1042"},
			{ID: 9, Number: "PO-1043"},
		},
		"count": 2,
	}
	got := CollectEntities(data)
	if len(got) != 2 {
		t.Fatalf("want 2 entities from a listing, got %d (%+v)", len(got), got)
	}
	if got[0].Kind != EntityOrder || got[0].ID != 7 || got[0].Label != "PO-1042" {
		t.Fatalf("unexpected first entity: %+v", got[0])
	}

	nested := &OrderDetail{
		Order: PurchaseOrderRow{ID: 7, Number: "PO-1042"},
		Lines: []OrderLineRow{{ProductName: "باراسيتامول"}},
	}
	got = CollectEntities(nested)
	if len(got) != 1 || got[0].ID != 7 {
		t.Fatalf("want the order inside a detail struct, got %+v", got)
	}
}

// A row with no number is not referenceable: there is nothing for the answer to
// say that could be matched back to it.
func TestCollectEntitiesSkipsUnlabelledRows(t *testing.T) {
	if got := CollectEntities([]PurchaseOrderRow{{ID: 3, Number: ""}}); len(got) != 0 {
		t.Fatalf("want no entity for an unnumbered order, got %+v", got)
	}
}

func TestCollectEntitiesDeduplicates(t *testing.T) {
	rows := []PurchaseOrderRow{
		{ID: 7, Number: "PO-1042"},
		{ID: 7, Number: "PO-1042"},
	}
	if got := CollectEntities(rows); len(got) != 1 {
		t.Fatalf("want one entity for a repeated row, got %d", len(got))
	}
}

func TestCollectEntitiesStopsAtTheCeiling(t *testing.T) {
	rows := make([]PurchaseOrderRow, MaxEntitiesPerTurn+20)
	for i := range rows {
		rows[i] = PurchaseOrderRow{ID: int64(i + 1), Number: "PO-" + string(rune('a'+i%26))}
	}
	if got := CollectEntities(rows); len(got) > MaxEntitiesPerTurn {
		t.Fatalf("collector exceeded its ceiling: %d", len(got))
	}
}

// Nothing gets a link on a dashboard where it has no page. A vendor has no
// screen for a pharmacy's purchase order, and a link that refuses on click
// reads as the assistant being wrong rather than as a permission.
func TestResolveLinksDropsEntitiesWithNoPage(t *testing.T) {
	ents := []Entity{{Kind: EntityOrder, ID: 7, Label: "PO-1042"}}

	pharmacy := ResolveLinks(rbac.ScopePharmacy, ents)
	if len(pharmacy) != 1 || pharmacy[0].URL != "/orders/7" {
		t.Fatalf("pharmacy should reach its own order: %+v", pharmacy)
	}
	if len(pharmacy[0].Actions) != 1 || pharmacy[0].Actions[0].URL != "/orders/7/invoice/print" {
		t.Fatalf("an order should offer its invoice: %+v", pharmacy[0].Actions)
	}

	if vendor := ResolveLinks(rbac.ScopeVendor, ents); len(vendor) != 0 {
		t.Fatalf("a vendor has no page for a purchase order, got %+v", vendor)
	}
}

func TestMergeEntitiesDedupesAcrossRounds(t *testing.T) {
	first := []Entity{{Kind: EntityOrder, ID: 1, Label: "A"}}
	merged := MergeEntities(first,
		Entity{Kind: EntityOrder, ID: 1, Label: "A"},
		Entity{Kind: EntityOrder, ID: 2, Label: "B"},
	)
	if len(merged) != 2 {
		t.Fatalf("want two distinct entities, got %+v", merged)
	}
}

// The fold is what makes a link land when the model writes the number in
// Arabic-Indic digits, or spells a supplier with أ where the database has ا.
func TestFoldForMatchNormalisesArabicAndDigits(t *testing.T) {
	cases := [][2]string{
		{"طلب رقم ١٠٤٢", "طلب رقم 1042"},
		{"أحمد", "احمد"},
		{"صيدلية النهضة", "صيدليه النهضه"},
		{"PO-1042", "po-1042"},
		{"a   b", "a b"},
	}
	for _, c := range cases {
		if got := FoldForMatch(c[0]); got != c[1] {
			t.Errorf("FoldForMatch(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}

func TestNumberAliasesOffersTheBareNumberOnlyWhenItIsLongEnough(t *testing.T) {
	if got := numberAliases("PO-1042"); len(got) != 2 || got[1] != "1042" {
		t.Fatalf("want #PO-1042 and 1042, got %v", got)
	}
	// A two-digit tail would match a quantity in the same sentence.
	for _, a := range numberAliases("PO-12") {
		if a == "12" {
			t.Fatalf("a short numeric tail must not become an alias: %v", a)
		}
	}
}
