package platformadmin

import (
	"context"
	"fmt"
	"strings"
)

// Soft-delete recovery. /what-in lists "Trash & Deleted Items (استرجاع أو الحذف
// النهائي للبيانات مع التاريخ)" as an admin pillar, and Laravel has six routes
// for it. The Go screens existed with a hardcoded model list and restore/purge
// buttons that logged a line and reported success — this is the backend that
// makes them true.

// TrashModel is one soft-deletable table the admin can browse.
type TrashModel struct {
	Key         string // URL segment, e.g. "catalog.products"
	Schema      string
	Table       string
	NameAr      string
	NameEn      string
	TotalCount  int64
	TrashedRows int64
}

// TrashRow is one soft-deleted record, rendered generically. Different tables
// have different columns, so the display fields are resolved from whichever of
// name / title / trade_name / email the table happens to have.
type TrashRow struct {
	ID        int64
	Label     string
	DeletedAt string
}

// trashTableLabels gives the tables we expect to surface a readable Arabic name.
// A table not listed here still appears, labelled by its schema.table — the
// registry is built from information_schema, not from this map, so a new
// soft-deletable table shows up without a code change.
var trashTableLabels = map[string][2]string{
	"catalog.products":         {"المنتجات والأدوية", "Products"},
	"catalog.product_variants": {"أصناف الموردين", "Product Variants"},
	"catalog.categories":       {"التصنيفات", "Categories"},
	"catalog.brands":           {"الشركات المصنعة", "Brands"},
	"org.organizations":        {"المنشآت والشركات", "Organizations"},
	"org.branches":             {"الفروع والمستودعات", "Branches"},
	"identity.users":           {"المستخدمين", "Users"},
	"commerce.orders":          {"الطلبات وأوامر التوريد", "Orders"},
	"billing.invoices":         {"الفواتير", "Invoices"},
	"promo.offers":             {"العروض والخصومات", "Offers"},
}

// ListTrashModels returns every table that carries a deleted_at column, with
// live row counts. The list is discovered, not hand-maintained, so it cannot
// drift out of date the way a hardcoded slice does.
func (s *Service) ListTrashModels(ctx context.Context) ([]*TrashModel, error) {
	models, err := s.repo.ListSoftDeletableTables(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		if lbl, ok := trashTableLabels[m.Key]; ok {
			m.NameAr, m.NameEn = lbl[0], lbl[1]
		} else {
			m.NameAr, m.NameEn = m.Key, m.Key
		}
	}
	return models, nil
}

// ListTrashedRows returns the soft-deleted records of one table.
func (s *Service) ListTrashedRows(ctx context.Context, key string, limit, offset int) ([]*TrashRow, error) {
	schema, table, err := splitTrashKey(key)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListTrashedRows(ctx, schema, table, limit, offset)
}

// RestoreTrashedRow clears deleted_at. It refuses when the row's parent is
// itself still deleted, because restoring a child under a deleted parent
// produces a row nothing can reach.
func (s *Service) RestoreTrashedRow(ctx context.Context, key string, id int64, actorID int64) error {
	schema, table, err := splitTrashKey(key)
	if err != nil {
		return err
	}
	return s.repo.RestoreTrashedRow(ctx, schema, table, id, actorID)
}

// PurgeTrashedRow hard-deletes. This is irreversible, so the repository records
// the row's contents in the audit log before removing it.
func (s *Service) PurgeTrashedRow(ctx context.Context, key string, id int64, actorID int64) error {
	schema, table, err := splitTrashKey(key)
	if err != nil {
		return err
	}
	return s.repo.PurgeTrashedRow(ctx, schema, table, id, actorID)
}

// splitTrashKey validates a "schema.table" key. The key reaches SQL as an
// identifier, so it is checked against a strict shape here and re-validated
// against information_schema in the repository — a table name from a URL is
// never interpolated on trust alone.
func splitTrashKey(key string) (schema, table string, err error) {
	parts := strings.Split(key, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("trash: %q is not a schema.table key", key)
	}
	for _, p := range parts {
		if p == "" || !isPlainIdentifier(p) {
			return "", "", fmt.Errorf("trash: %q is not a valid identifier", key)
		}
	}
	return parts[0], parts[1], nil
}

func isPlainIdentifier(s string) bool {
	if len(s) > 63 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
