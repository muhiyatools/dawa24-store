package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestNameExpr(t *testing.T) {
	expr := nameExpr("col")
	if !strings.Contains(expr, "col->>'ar'") {
		t.Errorf("expected Arabic fallback in %s", expr)
	}
	if !strings.Contains(expr, "col->>'en'") {
		t.Errorf("expected English fallback in %s", expr)
	}
	if !strings.Contains(expr, "col#>>'{}'") {
		t.Errorf("expected scalar JSON fallback in %s", expr)
	}
}

func TestVendorCatalogQueriesNoInvalidColumns(t *testing.T) {
	content, err := os.ReadFile("read_vendor_catalog.go")
	if err != nil {
		t.Fatalf("read read_vendor_catalog.go: %v", err)
	}
	s := string(content)

	// min_order_value was dropped in migration 065 in favour of min_order_amount.
	if strings.Contains(s, "min_order_value") {
		t.Errorf("read_vendor_catalog.go references dropped column min_order_value")
	}

	// inventory.warehouses.name is TEXT, so calling nameExpr on w.name causes a postgres syntax error.
	if strings.Contains(s, "w.name->>") || strings.Contains(s, "w.name#>>") {
		t.Errorf("read_vendor_catalog.go treats w.name (TEXT) as JSONB")
	}
}
