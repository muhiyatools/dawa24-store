package ui_test

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// End-to-end coverage of the catalogue import wizard against a real database.
//
// The bug the mapping test locks down was invisible to every unit test in the
// repository: the bulk INSERT bound institutional_work_ids through
// COALESCE($23, '{}'), which PostgreSQL resolves to text, so every row of every
// import failed with `column "institutional_work_ids" is of type bigint[] but
// expression is of type text`. Nothing but a real INSERT against real column
// types catches that, which is why these talk to the database.
//
// Every fixture is namespaced. Name matching is one of the behaviours under
// test and it runs against whatever catalogue the test database holds — on a
// shared one that is a real catalogue of thousands of products, and a fixture
// called "بانادول" would match a real row and quietly change what the test
// measures.

func superAdmin() authctx.Actor {
	return authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin"}
}

// importTestSKU and importTestTag namespace the rows these tests create.
const (
	importTestSKU = "UITEST-IMPORT"
	importTestTag = "UITEST"
)

// sessionIDPattern pulls the session id out of the redirect the upload returns.
var sessionIDPattern = regexp.MustCompile(`/admin/products/import/([0-9a-f-]{36})`)

// uploadImportFile posts a file to the wizard and returns the session id it
// staged, failing the test if the upload was rejected.
func uploadImportFile(t *testing.T, h http.Handler, content string, settings url.Values) string {
	t.Helper()
	rec := postImportFile(t, h, "catalog.csv", content, settings)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want 303; body: %s", rec.Code, truncate(rec.Body.String(), 900))
	}
	match := sessionIDPattern.FindStringSubmatch(rec.Header().Get("Location"))
	if match == nil {
		t.Fatalf("no session id in redirect %q", rec.Header().Get("Location"))
	}
	return match[1]
}

// postImportFile uploads content as a multipart file with the wizard's settings.
func postImportFile(
	t *testing.T, h http.Handler, filename, content string, settings url.Values,
) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, values := range settings {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatalf("write form field: %v", err)
			}
		}
	}
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

// postSessionAction drives one of the review screen's buttons.
func postSessionAction(
	t *testing.T, h http.Handler, sessionID, verb string, form url.Values,
) *httptest.ResponseRecorder {
	t.Helper()

	ctx := authctx.WithActor(context.Background(), superAdmin())
	req, err := http.NewRequestWithContext(ctx, "POST",
		"/admin/products/import/"+sessionID+"/"+verb, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cleanupImportedProducts(t *testing.T, db *database.DB) {
	t.Helper()
	// Reported, not swallowed. A cleanup that silently fails leaves rows behind
	// that make the next run of these tests measure the wrong thing — which is
	// exactly what happened while this suite was being written.
	t.Cleanup(func() {
		err := deleteWithRetry(context.Background(), db, func(txCtx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(txCtx,
				`DELETE FROM catalog.products WHERE sku LIKE $1`, importTestSKU+"%"); err != nil {
				return err
			}
			if _, err := tx.Exec(txCtx,
				`DELETE FROM catalog.brands WHERE name->>'ar' LIKE 'UITest%'`); err != nil {
				return err
			}
			_, err := tx.Exec(txCtx,
				`DELETE FROM catalog.import_sessions WHERE filename IN ('catalog.csv','old.xls','empty.csv')`)
			return err
		})
		if err != nil {
			t.Errorf("cleanup failed, leaving rows behind for the next run: %v", err)
		}
	})
}

// deleteWithRetry runs a cleanup transaction, retrying briefly.
//
// Committing an import kicks off a background rebuild of catalog.product_index,
// which holds locks these deletes also need. Losing that race is expected and
// transient; leaving rows behind is not, because the next run of these tests
// would then measure a catalogue it did not create.
func deleteWithRetry(ctx context.Context, db *database.DB, fn func(context.Context, pgx.Tx) error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
		}
		if err = db.InTx(database.AsSystem(ctx), fn); err == nil {
			return nil
		}
	}
	return err
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

// defaultSettings is the wizard's own default posture: update what matches, add
// what does not, register manufacturers, infer the pharmaceutical form.
func defaultSettings() url.Values {
	return url.Values{
		"import_mode":        {"update_and_add"},
		"auto_create_brands": {"1"},
		"assign_dosage_form": {"1"},
	}
}

func threeProductFixture() string {
	return importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,55.00,UITest Pharma", importTestTag, importTestSKU),
		fmt.Sprintf("%s كتافلام 50 مجم أقراص,%s-2,42.00,UITest Pharma", importTestTag, importTestSKU),
		fmt.Sprintf("%s اوجمنتين 1 جم,%s-3,115.00,UITest Labs", importTestTag, importTestSKU),
	)
}

// Uploading stages the file for review and writes nothing. This is the whole
// point of the confirmation step: the admin sees the outcome before it happens.
func TestAdminProductsImportStagesWithoutWriting(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(), defaultSettings())

	if n := countImportedProducts(t, db); n != 0 {
		t.Fatalf("catalogue holds %d products after upload, want 0 — nothing may be written before confirmation", n)
	}

	rec := doGET(t, h, "/admin/products/import/"+sessionID, superAdmin())
	if rec.Code != http.StatusOK {
		t.Fatalf("review status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"مراجعة ما سيتم تنفيذه", "بانادول اكسترا", "كيف تمت قراءة الملف"} {
		if !strings.Contains(body, want) {
			t.Errorf("the review screen does not show %q", want)
		}
	}
}

func TestAdminProductsImportCommitsAfterConfirmation(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(), defaultSettings())

	rec := postSessionAction(t, h, sessionID, "commit", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d, want 303; body: %s", rec.Code, truncate(rec.Body.String(), 600))
	}
	if n := countImportedProducts(t, db); n != 3 {
		t.Fatalf("catalogue holds %d products after commit, want 3", n)
	}
}

func TestAdminProductsImportCancelWritesNothing(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(), defaultSettings())

	if rec := postSessionAction(t, h, sessionID, "cancel", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("cancel status = %d, want 303", rec.Code)
	}
	if n := countImportedProducts(t, db); n != 0 {
		t.Fatalf("catalogue holds %d products after cancelling, want 0", n)
	}

	// A cancelled session cannot then be committed behind the admin's back.
	postSessionAction(t, h, sessionID, "commit", url.Values{})
	if n := countImportedProducts(t, db); n != 0 {
		t.Fatalf("a cancelled session wrote %d products, want 0", n)
	}
}

// Re-uploading a corrected price list is routine. The importer this replaced
// only ever INSERTed, so it doubled the catalogue every time.
func TestAdminProductsImportUpdatesRatherThanDuplicating(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	first := importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,55.00,UITest Pharma", importTestTag, importTestSKU),
	)
	sessionID := uploadImportFile(t, h, first, defaultSettings())
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("first commit status = %d", rec.Code)
	}
	if n := countImportedProducts(t, db); n != 1 {
		t.Fatalf("after first import: %d products, want 1", n)
	}

	corrected := importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,60.00,UITest Pharma", importTestTag, importTestSKU),
	)
	sessionID = uploadImportFile(t, h, corrected, defaultSettings())
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("second commit status = %d", rec.Code)
	}
	if n := countImportedProducts(t, db); n != 1 {
		t.Fatalf("after re-import: %d products, want 1 (updated, not duplicated)", n)
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

// The add-new-only strategy must leave an existing product alone.
func TestAdminProductsImportAddNewOnlySkipsExisting(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	seed := importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,55.00,UITest Pharma", importTestTag, importTestSKU),
	)
	sessionID := uploadImportFile(t, h, seed, defaultSettings())
	postSessionAction(t, h, sessionID, "commit", url.Values{})

	settings := defaultSettings()
	settings.Set("import_mode", "add_new_only")
	both := importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,99.00,UITest Pharma", importTestTag, importTestSKU),
		fmt.Sprintf("%s كتافلام 50 مجم أقراص,%s-2,42.00,UITest Pharma", importTestTag, importTestSKU),
	)
	sessionID = uploadImportFile(t, h, both, settings)
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
	}

	if n := countImportedProducts(t, db); n != 2 {
		t.Fatalf("catalogue holds %d products, want 2", n)
	}

	var price string
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT price::text FROM catalog.products WHERE sku = $1 AND deleted_at IS NULL`,
			importTestSKU+"-1").Scan(&price)
	})
	if err != nil {
		t.Fatalf("read price: %v", err)
	}
	if price != "55.00" {
		t.Errorf("price = %s, want 55.00 — add-new-only must not touch an existing product", price)
	}
}

// The archive strategy is destructive and needs its own acknowledgement, which
// is what stops it happening on a mis-click.
func TestAdminProductsImportArchiveModeRequiresConfirmation(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	settings := defaultSettings()
	settings.Set("import_mode", "clear_and_add")
	sessionID := uploadImportFile(t, h, threeProductFixture(), settings)

	rec := postSessionAction(t, h, sessionID, "commit", url.Values{})
	if rec.Code == http.StatusSeeOther {
		t.Fatal("the archive strategy committed without an acknowledgement")
	}
	if !strings.Contains(rec.Body.String(), "يجب تأكيد أرشفة") {
		t.Error("the refusal does not explain that the acknowledgement is required")
	}
	if n := countImportedProducts(t, db); n != 0 {
		t.Fatalf("catalogue holds %d products, want 0", n)
	}
}

// A row the admin deselects must not be written.
func TestAdminProductsImportRespectsDeselectedRows(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(), defaultSettings())

	var rowID int64
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT r.id FROM catalog.import_staging_rows r
			JOIN catalog.import_sessions s ON s.id = r.session_id
			WHERE s.public_id = $1 ORDER BY r.source_row LIMIT 1
		`, sessionID).Scan(&rowID)
	})
	if err != nil {
		t.Fatalf("read staged row: %v", err)
	}

	rec := postSessionAction(t, h, sessionID, fmt.Sprintf("rows/%d", rowID), url.Values{"included": {"0"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("row toggle status = %d, want 303", rec.Code)
	}
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
	}

	if n := countImportedProducts(t, db); n != 2 {
		t.Fatalf("catalogue holds %d products, want 2 — the deselected row must be skipped", n)
	}
}

// Re-running under a corrected column mapping must change the reading without
// writing anything, which is what makes the review step worth having.
func TestAdminProductsImportReprocessesWithCorrectedColumns(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(), defaultSettings())

	settings := defaultSettings()
	// Read column 3 as the public price rather than the selling price.
	settings.Set("col_public_price", "3")
	settings.Set("col_price", "0")
	if rec := postSessionAction(t, h, sessionID, "prepare", settings); rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare status = %d, want 303", rec.Code)
	}
	if n := countImportedProducts(t, db); n != 0 {
		t.Fatalf("re-processing wrote %d products; it must write none", n)
	}

	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
	}

	var oldPrice string
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT old_price::text FROM catalog.products WHERE sku = $1 AND deleted_at IS NULL`,
			importTestSKU+"-1").Scan(&oldPrice)
	})
	if err != nil {
		t.Fatalf("read prices: %v", err)
	}
	if oldPrice != "55.00" {
		t.Errorf("public price = %s, want 55.00 from the rebound column", oldPrice)
	}
}

func TestAdminProductsImportReportsUnreadableFile(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)

	// A legacy .xls is the single most common bad upload, and it deserves the
	// specific fix rather than "invalid format".
	rec := postImportFile(t, h, "old.xls",
		string(append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...)),
		defaultSettings())

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), ".xlsx") {
		t.Error("the screen does not tell the admin to resave the file as .xlsx")
	}
}

func TestAdminProductsImportRejectsEmptyUpload(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)

	rec := postImportFile(t, h, "empty.csv", "", defaultSettings())
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "فارغ") {
		t.Error("the screen does not say the file was empty")
	}
}

// The header layout that used to bind the barcode column as the item code.
func TestAdminProductsImportExplainsColumnMapping(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	file := "اسم الصنف التجاري / الوصف,الباركود الدولي,كود الصنف,سعر البيع للجمهور,الشركة المصنعة\n" +
		fmt.Sprintf("%s بانادول اكسترا,6221234567890,%s-1,55.00,UITest Pharma\n", importTestTag, importTestSKU)

	sessionID := uploadImportFile(t, h, file, defaultSettings())

	rec := doGET(t, h, "/admin/products/import/"+sessionID, superAdmin())
	if rec.Code != http.StatusOK {
		t.Fatalf("review status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"كيف تمت قراءة الملف", "الباركود", "كود الصنف"} {
		if !strings.Contains(body, want) {
			t.Errorf("the review screen does not mention %q", want)
		}
	}

	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
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

// A file whose header is reprinted mid-way must import every block, and the
// reprints must not arrive as products.
func TestAdminProductsImportReadsPaginatedExport(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	cleanupImportedProducts(t, db)

	const header = "اسم الصنف,كود الصنف,سعر البيع,الشركة المصنعة"
	rows := []string{"قائمة أصناف - تصدير", header}
	for i := 0; i < 30; i++ {
		if i > 0 && i%10 == 0 {
			rows = append(rows, "", header)
		}
		rows = append(rows, fmt.Sprintf("%s صنف رقم %d أقراص,%s-%03d,25.00,UITest Pharma",
			importTestTag, i, importTestSKU, i))
	}

	sessionID := uploadImportFile(t, h, strings.Join(rows, "\n")+"\n", defaultSettings())
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
	}

	if n := countImportedProducts(t, db); n != 30 {
		t.Fatalf("catalogue holds %d products, want 30 — every block must be read and no header imported", n)
	}
}

// truncate reports why a wizard screen was rendered instead of a redirect.
//
// The screens are full HTML documents, so the first 900 bytes are all boilerplate
// and tell a failing test nothing. The banner is the part that carries the reason.
func truncate(s string, n int) string {
	// Start after the banner element's opening tag, so the tag-stripper below
	// begins outside a tag rather than half-way through one.
	if start := strings.Index(s, "import-banner-title"); start >= 0 {
		if open := strings.IndexByte(s[start:], '>'); open >= 0 {
			start += open + 1
		}
		return stripTags(s[start:min(start+n, len(s))])
	}
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// stripTags reduces a fragment of rendered HTML to its text.
func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
