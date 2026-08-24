package ui_test

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// End-to-end coverage of /admin/products/import against a real database.
//
// The bug these lock down was invisible to every unit test in the repository:
// the bulk INSERT bound institutional_work_ids through COALESCE($23, '{}'),
// which PostgreSQL resolves to text, so every row of every import failed with
// `column "institutional_work_ids" is of type bigint[] but expression is of
// type text`. Nothing but a real INSERT against real column types can catch
// that, which is why these tests talk to the database rather than a mock.

func superAdmin() authctx.Actor {
	return authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin"}
}

// postImportFile uploads content as a multipart file to the import endpoint.
func postImportFile(t *testing.T, h http.Handler, filename, content string) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("import_file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	ctx := authctx.WithActor(context.Background(), superAdmin())
	req, err := http.NewRequestWithContext(ctx, "POST", "/admin/products/import", &body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// importTestSKU namespaces every row these tests write so cleanup can remove
// exactly what was created and nothing else.
const importTestSKU = "UITEST-IMPORT"

func cleanupImportedProducts(t *testing.T, db *database.DB) {
	t.Helper()
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, `DELETE FROM catalog.products WHERE sku LIKE $1`, importTestSKU+"%")
			_, _ = tx.Exec(txCtx, `DELETE FROM catalog.brands WHERE name->>'ar' LIKE 'UITest%'`)
			return nil
		})
	})
}

func countImportedProducts(t *testing.T, db *database.DB) int {
	t.Helper()
	var n int
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT count(*) FROM catalog.products WHERE sku LIKE $1 AND deleted_at IS NULL`,
			importTestSKU+"%").Scan(&n)
	})
	if err != nil {
		t.Fatalf("count imported products: %v", err)
	}
	return n
}

func importFixture(rows ...string) string {
	header := "اسم الصنف,كود الصنف,سعر البيع,الشركة المصنعة"
	return header + "\n" + strings.Join(rows, "\n") + "\n"
}

func TestAdminProductsImportWritesToCatalog(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	rec := postImportFile(t, h, "catalog.csv", importFixture(
		fmt.Sprintf("بانادول اكسترا 500 مجم,%s-1,55.00,UITest Pharma", importTestSKU),
		fmt.Sprintf("كتافلام 50 مجم أقراص,%s-2,42.00,UITest Pharma", importTestSKU),
		fmt.Sprintf("اوجمنتين 1 جم,%s-3,115.00,UITest Labs", importTestSKU),
	))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, truncate(rec.Body.String(), 800))
	}
	if n := countImportedProducts(t, db); n != 3 {
		t.Fatalf("catalogue holds %d imported products, want 3", n)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "تم استيراد الأصناف بنجاح") {
		t.Errorf("success banner missing from the report page")
	}
	// The result is a page, not a redirect carrying the message in the query
	// string — the behaviour that put percent-encoded Arabic in the address bar.
	if rec.Header().Get("Location") != "" {
		t.Errorf("import redirected to %q instead of rendering a report", rec.Header().Get("Location"))
	}
}

func TestAdminProductsImportUpdatesRatherThanDuplicating(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	file := importFixture(
		fmt.Sprintf("بانادول اكسترا 500 مجم,%s-1,55.00,UITest Pharma", importTestSKU),
		fmt.Sprintf("كتافلام 50 مجم أقراص,%s-2,42.00,UITest Pharma", importTestSKU),
	)

	if rec := postImportFile(t, h, "catalog.csv", file); rec.Code != http.StatusOK {
		t.Fatalf("first import status = %d", rec.Code)
	}
	if n := countImportedProducts(t, db); n != 2 {
		t.Fatalf("after first import: %d products, want 2", n)
	}

	// Re-uploading a corrected price list is routine. The old importer only ever
	// INSERTed, so this doubled the catalogue every time.
	corrected := importFixture(
		fmt.Sprintf("بانادول اكسترا 500 مجم,%s-1,60.00,UITest Pharma", importTestSKU),
		fmt.Sprintf("كتافلام 50 مجم أقراص,%s-2,45.00,UITest Pharma", importTestSKU),
	)
	rec := postImportFile(t, h, "catalog.csv", corrected)
	if rec.Code != http.StatusOK {
		t.Fatalf("second import status = %d", rec.Code)
	}
	if n := countImportedProducts(t, db); n != 2 {
		t.Fatalf("after re-import: %d products, want 2 (the rows must be updated, not duplicated)", n)
	}

	var price string
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT price::text FROM catalog.products WHERE sku = $1 AND deleted_at IS NULL`,
			importTestSKU+"-1").Scan(&price)
	})
	if err != nil {
		t.Fatalf("read updated price: %v", err)
	}
	if price != "60.00" {
		t.Errorf("price after re-import = %s, want 60.00", price)
	}
}

func TestAdminProductsImportRefusesBadRowAndKeepsTheRest(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	// A price beyond NUMERIC(12,2) is rejected at parse time, so the good rows
	// still land and the bad row is named. Partial catalogues are what this
	// whole design exists to avoid.
	rec := postImportFile(t, h, "catalog.csv", importFixture(
		fmt.Sprintf("صنف سليم,%s-1,55.00,UITest Pharma", importTestSKU),
		fmt.Sprintf("صنف بسعر فلكي,%s-2,99999999999999.99,UITest Pharma", importTestSKU),
		fmt.Sprintf("صنف سليم آخر,%s-3,42.00,UITest Pharma", importTestSKU),
	))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if n := countImportedProducts(t, db); n != 2 {
		t.Fatalf("catalogue holds %d products, want 2 (the over-range row must be refused)", n)
	}
	if !strings.Contains(rec.Body.String(), "تتجاوز الحد الأقصى") {
		t.Error("the report does not explain why the row was refused")
	}
}

func TestAdminProductsImportReportsUnreadableFile(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)

	// A legacy .xls is the single most common bad upload, and it deserves the
	// specific fix rather than "invalid format".
	rec := postImportFile(t, h, "old.xls",
		string(append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...)))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, ".xlsx") {
		t.Error("the report does not tell the admin to resave the file as .xlsx")
	}
}

func TestAdminProductsImportExplainsColumnMapping(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	// The header layout that used to bind the barcode column as the item code.
	// The report must show the reading so an admin can catch a wrong one.
	file := "اسم الصنف التجاري / الوصف,الباركود الدولي,كود الصنف,سعر البيع للجمهور,الشركة المصنعة\n" +
		fmt.Sprintf("بانادول اكسترا,6221234567890,%s-1,55.00,UITest Pharma\n", importTestSKU)

	rec := postImportFile(t, h, "catalog.csv", file)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{"كيف تمت قراءة أعمدة الملف", "الباركود", "كود الصنف"} {
		if !strings.Contains(body, want) {
			t.Errorf("the report does not mention %q", want)
		}
	}

	var sku, barcode string
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT sku, barcode FROM catalog.products WHERE sku = $1 AND deleted_at IS NULL`,
			importTestSKU+"-1").Scan(&sku, &barcode)
	})
	if err != nil {
		t.Fatalf("read imported row: %v", err)
	}
	if barcode != "6221234567890" {
		t.Errorf("barcode = %q, want the barcode column's value, not the item code", barcode)
	}
}

func TestAdminProductsImportRejectsEmptyUpload(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)

	rec := postImportFile(t, h, "empty.csv", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "فارغ") {
		t.Error("the report does not say the file was empty")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func TestAdminProductsImportDoesNotReactivateDisabledProducts(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	file := importFixture(
		fmt.Sprintf("بانادول اكسترا 500 مجم,%s-1,55.00,UITest Pharma", importTestSKU),
	)
	if rec := postImportFile(t, h, "catalog.csv", file); rec.Code != http.StatusOK {
		t.Fatalf("first import status = %d", rec.Code)
	}

	// An admin takes the product off the catalogue by hand.
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx,
			`UPDATE catalog.products SET status = 'inactive' WHERE sku = $1`, importTestSKU+"-1")
		return err
	})
	if err != nil {
		t.Fatalf("deactivate product: %v", err)
	}

	// Re-importing the supplier's price list must refresh the price and leave
	// that decision alone. The file carries no status column, so it has nothing
	// to say about whether the product is on sale.
	if rec := postImportFile(t, h, "catalog.csv", file); rec.Code != http.StatusOK {
		t.Fatalf("second import status = %d", rec.Code)
	}

	var status string
	err = db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT status FROM catalog.products WHERE sku = $1`, importTestSKU+"-1").Scan(&status)
	})
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "inactive" {
		t.Errorf("status = %q after re-import, want inactive (the file said nothing about status)", status)
	}
}
