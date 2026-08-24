package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Bulk catalogue import.
//
// Three properties this code owes its caller, none of which the previous
// implementation delivered:
//
//   - It really upserts. The old query only ever INSERTed, so re-uploading a
//     corrected price list duplicated the entire catalogue and the "updated"
//     count it reported was a constant zero.
//   - It is all-or-nothing. A row the database refuses rolls the whole import
//     back, so a half-written catalogue is never left behind.
//   - It says which row failed and why. The old code discarded every per-row
//     error and surfaced "bulk batch close failed" for a file of any size.

// chunkSize bounds one pipelined round trip. Large enough that a 10,000-row
// import is fifty round trips rather than ten thousand, small enough that the
// per-row replay after a failure stays cheap.
const chunkSize = 500

// maxNamedFailures bounds how many offending rows the error message names.
const maxNamedFailures = 20

const insertProductQuery = `
	INSERT INTO catalog.products (
		organization_id, category_id, brand_id, branch_id, name, description,
		sku, barcode, price, discount, old_price, image, image_link, status,
		is_featured, dosage_form, scientific_name, pharmacology, active,
		concentration, unit, manufacturing_companies, institutional_work_ids
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
		$17, $18, $19, $20, $21, $22, $23::bigint[]
	) RETURNING id;
`

// updateProductQuery refreshes an existing catalogue row from an imported one.
//
// Every assignment is guarded so a sparse import cannot erase data the
// catalogue already holds: a supplier price list that carries names and prices
// but no scientific names must update the prices and leave the pharmacology
// alone. COALESCE is not enough for this — the columns are NOT NULL with an
// empty-string default — so the guard is an explicit emptiness test per column.
const updateProductQuery = `
	UPDATE catalog.products SET
		name        = $2,
		description = CASE WHEN $3::jsonb IS NULL OR $3::jsonb = '{}'::jsonb
		                   THEN description ELSE $3::jsonb END,
		sku         = CASE WHEN $4::text  = '' THEN sku      ELSE $4::text  END,
		barcode     = CASE WHEN $5::text  = '' THEN barcode  ELSE $5::text  END,
		price       = CASE WHEN $6::numeric = 0 THEN price   ELSE $6::numeric END,
		discount    = CASE WHEN $7::numeric = 0 THEN discount ELSE $7::numeric END,
		old_price   = CASE WHEN $8::numeric = 0 THEN old_price ELSE $8::numeric END,
		status      = CASE WHEN $9::text = '' THEN status ELSE $9::text END,
		dosage_form = CASE WHEN $10::text = '' THEN dosage_form ELSE $10::text END,
		scientific_name = CASE WHEN $11::text = '' THEN scientific_name ELSE $11::text END,
		active      = CASE WHEN $12::text = '' THEN active ELSE $12::text END,
		concentration = CASE WHEN $13::text = '' THEN concentration ELSE $13::text END,
		unit        = CASE WHEN $14::text = '' THEN unit ELSE $14::text END,
		manufacturing_companies = CASE WHEN $15::text = ''
		                   THEN manufacturing_companies ELSE $15::text END,
		brand_id    = COALESCE($16::bigint, brand_id),
		updated_at  = now()
	WHERE id = $1 AND deleted_at IS NULL;
`

// BulkUpsertProducts inserts new products and refreshes existing ones, matching
// on SKU, then barcode, then normalised Arabic name, within each product's
// organisation.
func (r *Repository) BulkUpsertProducts(
	ctx context.Context, prods []*catalog.Product, opts catalog.BulkWriteOptions,
) (catalog.BulkWriteResult, error) {
	result := catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}}
	if len(prods) == 0 {
		return result, nil
	}

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		defaultOrgID, err := resolveDefaultOrg(txCtx, tx)
		if err != nil {
			return err
		}

		for _, p := range prods {
			applyWriteDefaults(p, defaultOrgID)
		}

		brandsCreated, err := resolveBrands(txCtx, tx, prods, opts.CreateBrands)
		if err != nil {
			return err
		}
		result.BrandsCreated = brandsCreated

		categoriesCreated, err := resolveCategories(txCtx, tx, prods, opts.CreateCategories)
		if err != nil {
			return err
		}
		result.CategoriesCreated = categoriesCreated

		existing, err := resolveExistingProducts(txCtx, tx, prods)
		if err != nil {
			return err
		}

		var toInsert, toUpdate []int
		for i, p := range prods {
			if m, ok := existing[i]; ok {
				p.ID = m.id
				result.Matches[i] = m.reason
				toUpdate = append(toUpdate, i)
				continue
			}
			toInsert = append(toInsert, i)
		}

		inserter := &batchWriter{tx: tx, prods: prods, res: &result, queue: queueInsert, scan: scanInsert}
		if err := inserter.write(txCtx, toInsert); err != nil {
			return err
		}
		updater := &batchWriter{tx: tx, prods: prods, res: &result, queue: queueUpdate, scan: scanUpdate}
		return updater.write(txCtx, toUpdate)
	})

	if err != nil {
		// A rollback happened, so nothing was written whatever the counters
		// reached mid-flight. Reporting them would tell the admin rows landed
		// that did not.
		failures := result.Failures
		return catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}, Failures: failures}, err
	}
	return result, nil
}

// applyWriteDefaults fills the values the NOT NULL columns require. The products
// table defaults these at the column level, but a parameter binding of the zero
// value overrides the default, so they are set explicitly here.
//
// Status is deliberately not among them. An empty status means the file did not
// state one, which the insert reads as "active" and the update reads as "leave
// it alone" — so re-importing a supplier's price list cannot reactivate a
// product an admin deliberately took off the catalogue.
func applyWriteDefaults(p *catalog.Product, defaultOrgID int64) {
	if p.OrganizationID <= 0 {
		p.OrganizationID = defaultOrgID
	}
	if p.Price.IsZero() {
		p.Price = money.Zero
	}
	if p.Discount.IsZero() {
		p.Discount = money.Zero
	}
	if p.OldPrice.IsZero() {
		p.OldPrice = money.Zero
	}
	if p.InstitutionalWorkIDs == nil {
		p.InstitutionalWorkIDs = []int64{}
	}
}

// resolveDefaultOrg finds the organisation that owns the master catalogue,
// creating it only when the instance genuinely has none.
func resolveDefaultOrg(ctx context.Context, tx pgx.Tx) (int64, error) {
	orgID, err := lookupDefaultOrg(ctx, tx)
	if err != nil {
		return 0, err
	}
	if orgID > 0 {
		return orgID, nil
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO org.organizations (name, type, status)
		VALUES ('{"ar":"دواء 24 - الكتالوج المعتمد","en":"Dawa24 Master Catalog"}'::jsonb, 'company', 'approved')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: create master catalog organization: %w", err)
	}
	return orgID, nil
}

// lookupDefaultOrg finds the organisation that owns the master catalogue without
// creating one, so it is safe inside a read-only transaction.
func lookupDefaultOrg(ctx context.Context, tx pgx.Tx) (int64, error) {
	var orgID int64
	err := tx.QueryRow(ctx,
		`SELECT id FROM org.organizations WHERE status = 'approved' ORDER BY id ASC LIMIT 1`).Scan(&orgID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("catalog postgres: resolve default organization: %w", err)
	}
	if orgID > 0 {
		return orgID, nil
	}

	err = tx.QueryRow(ctx, `SELECT id FROM org.organizations ORDER BY id ASC LIMIT 1`).Scan(&orgID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("catalog postgres: resolve any organization: %w", err)
	}
	return orgID, nil
}

// resolveBrands binds each product's manufacturer to a brand row, registering
// manufacturers the catalogue has not seen before.
//
// The previous version issued one INSERT per unrecognised manufacturer inside
// the product loop and ignored the error, so a failed brand insert aborted the
// surrounding transaction invisibly and every subsequent statement failed with
// a message about the wrong thing.
func resolveBrands(
	ctx context.Context, tx pgx.Tx, prods []*catalog.Product, allowCreate bool,
) (int, error) {
	brands, err := loadBrandIndex(ctx, tx)
	if err != nil {
		return 0, err
	}

	created := 0
	for _, p := range prods {
		if p.ManufacturingCompanies == "" || (p.BrandID != nil && *p.BrandID > 0) {
			continue
		}
		key := catalog.NormalizeKey(p.ManufacturingCompanies)
		// A manufacturer of one or two folded characters is a stray cell, not a
		// company; registering it would litter the brand list permanently.
		if len([]rune(key)) < 3 {
			continue
		}

		// An existing manufacturer is reused whatever the toggle says. The
		// toggle governs creating one, not linking to one that is already
		// there — refusing to link would leave the product unbranded and the
		// next import would ask the same question again.
		id, known := brands[key]
		if !known {
			if !allowCreate {
				continue
			}
			if id, err = insertBrand(ctx, tx, p.ManufacturingCompanies); err != nil {
				return created, err
			}
			// Cached immediately, so a file naming the same new manufacturer on
			// a thousand rows creates it once.
			brands[key] = id
			created++
		}
		p.BrandID = &id
	}
	return created, nil
}

// resolveCategories links products to categories by their folded name, creating
// the missing ones only when the admin allowed it.
//
// It mirrors resolveBrands exactly, and for the same reason: an import that
// runs twice must land on the same taxonomy rows, not a second copy of them.
func resolveCategories(
	ctx context.Context, tx pgx.Tx, prods []*catalog.Product, allowCreate bool,
) (int, error) {
	needed := false
	for _, p := range prods {
		if p.SourceCategory != "" && (p.CategoryID == nil || *p.CategoryID <= 0) {
			needed = true
			break
		}
	}
	if !needed {
		return 0, nil
	}

	categories, err := loadCategoryIndex(ctx, tx)
	if err != nil {
		return 0, err
	}

	created := 0
	for _, p := range prods {
		if p.SourceCategory == "" || (p.CategoryID != nil && *p.CategoryID > 0) {
			continue
		}
		key := catalog.NormalizeKey(p.SourceCategory)
		if len([]rune(key)) < 2 {
			continue
		}

		id, known := categories[key]
		if !known {
			if !allowCreate {
				continue
			}
			if id, err = insertCategory(ctx, tx, p.SourceCategory); err != nil {
				return created, err
			}
			categories[key] = id
			created++
		}
		resolved := id
		p.CategoryID = &resolved
	}
	return created, nil
}

// loadCategoryIndex reads every live category into a folded-name lookup.
func loadCategoryIndex(ctx context.Context, tx pgx.Tx) (map[string]int64, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, COALESCE(name->>'ar', ''), COALESCE(name->>'en', '')
		 FROM catalog.categories WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load categories: %w", err)
	}
	defer rows.Close()

	categories := map[string]int64{}
	for rows.Next() {
		var id int64
		var nameAR, nameEN string
		if err := rows.Scan(&id, &nameAR, &nameEN); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan category: %w", err)
		}
		for _, name := range []string{nameAR, nameEN} {
			key := catalog.NormalizeKey(name)
			if key == "" {
				continue
			}
			if _, taken := categories[key]; !taken {
				categories[key] = id
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog postgres: read categories: %w", err)
	}
	return categories, nil
}

func insertCategory(ctx context.Context, tx pgx.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO catalog.categories (name, status)
		VALUES (jsonb_build_object('ar', $1::text, 'en', $1::text), 'active')
		RETURNING id
	`, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: register category %q: %w", name, err)
	}
	return id, nil
}

// loadBrandIndex reads every live brand into a folded-name lookup, so the loop
// above resolves a manufacturer without a query per product.
func loadBrandIndex(ctx context.Context, tx pgx.Tx) (map[string]int64, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, COALESCE(name->>'ar', ''), COALESCE(name->>'en', '')
		 FROM catalog.brands WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load brands: %w", err)
	}
	defer rows.Close()

	brands := map[string]int64{}
	for rows.Next() {
		var id int64
		var nameAR, nameEN string
		if err := rows.Scan(&id, &nameAR, &nameEN); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan brand: %w", err)
		}
		for _, name := range []string{nameAR, nameEN} {
			key := catalog.NormalizeKey(name)
			if key == "" {
				continue
			}
			if _, taken := brands[key]; !taken {
				brands[key] = id
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog postgres: read brands: %w", err)
	}
	return brands, nil
}

func insertBrand(ctx context.Context, tx pgx.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO catalog.brands (name, status)
		VALUES (jsonb_build_object('ar', $1::text, 'en', $1::text), 'active')
		RETURNING id
	`, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: register brand %q: %w", name, err)
	}
	return id, nil
}

// productMatch is an incoming row tied to a catalogue row already on file.
type productMatch struct {
	id     int64
	reason catalog.MatchReason
}

// resolveExistingProducts finds, for each incoming product, the catalogue row it
// should update — matching on SKU first, then barcode, then normalised name.
//
// The lookups are set-based: three statements per organisation regardless of how
// many rows the file holds. Matching row by row would be nine thousand round
// trips for one of the real supplier files.
//
// Name matching runs platform.normalize_arabic on both sides rather than folding
// in Go and comparing. Same function, same input, so the two can never drift —
// and drift here would mean silently duplicating products whose names differ
// only by a hamza.
func resolveExistingProducts(ctx context.Context, tx pgx.Tx, prods []*catalog.Product) (map[int]productMatch, error) {
	byOrg := map[int64][]int{}
	for i, p := range prods {
		byOrg[p.OrganizationID] = append(byOrg[p.OrganizationID], i)
	}

	m := &productMatcher{ctx: ctx, tx: tx, prods: prods, matches: map[int]productMatch{}}
	for orgID, idxs := range byOrg {
		if err := m.matchOrg(orgID, idxs); err != nil {
			return nil, err
		}
	}
	return m.matches, nil
}

// productMatcher resolves incoming rows to catalogue ids one organisation at a
// time, accumulating decisions as it goes.
type productMatcher struct {
	ctx     context.Context
	tx      pgx.Tx
	prods   []*catalog.Product
	matches map[int]productMatch
}

// matchOrg applies the three strategies in strength order. SKU and barcode are
// exact identifiers; a name match is a fallback and must never override one, so
// an index already matched is left alone by every later strategy.
func (m *productMatcher) matchOrg(orgID int64, idxs []int) error {
	for _, column := range []struct {
		reason catalog.MatchReason
		name   string
		key    func(*catalog.Product) string
	}{
		{catalog.MatchSKU, "sku", func(p *catalog.Product) string { return p.SKU }},
		{catalog.MatchBarcode, "barcode", func(p *catalog.Product) string { return p.Barcode }},
	} {
		wanted := m.pending(idxs, func(p *catalog.Product) string {
			return strings.ToLower(strings.TrimSpace(column.key(p)))
		})
		if len(wanted) == 0 {
			continue
		}
		found, err := lookupByColumn(m.ctx, m.tx, orgID, column.name, keysOf(wanted))
		if err != nil {
			return err
		}
		m.record(wanted, found, column.reason)
	}

	wanted := m.pending(idxs, func(p *catalog.Product) string { return p.Name.Get(i18n.AR) })
	if len(wanted) == 0 {
		return nil
	}
	found, err := lookupByName(m.ctx, m.tx, orgID, keysOf(wanted))
	if err != nil {
		return err
	}
	m.record(wanted, found, catalog.MatchName)
	return nil
}

// pending groups the still-unmatched rows by the key one strategy looks them up
// with, so the lookup is a single set-based query rather than one per row.
func (m *productMatcher) pending(idxs []int, key func(*catalog.Product) string) map[string][]int {
	wanted := map[string][]int{}
	for _, i := range idxs {
		if _, done := m.matches[i]; done {
			continue
		}
		if k := key(m.prods[i]); strings.TrimSpace(k) != "" {
			wanted[k] = append(wanted[k], i)
		}
	}
	return wanted
}

func (m *productMatcher) record(wanted map[string][]int, found map[string]int64, reason catalog.MatchReason) {
	for k, id := range found {
		for _, i := range wanted[k] {
			if _, done := m.matches[i]; !done {
				m.matches[i] = productMatch{id: id, reason: reason}
			}
		}
	}
}

func keysOf(m map[string][]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// lookupByColumn resolves an exact identifier column to product ids. The column
// name is chosen from a fixed set in the caller, never from input.
func lookupByColumn(ctx context.Context, tx pgx.Tx, orgID int64, column string, keys []string) (map[string]int64, error) {
	query := fmt.Sprintf(`
		SELECT lower(btrim(p.%s)) AS k, min(p.id) AS id
		FROM catalog.products p
		WHERE p.organization_id = $1
		  AND p.deleted_at IS NULL
		  AND btrim(p.%s) <> ''
		  AND lower(btrim(p.%s)) = ANY($2::text[])
		GROUP BY 1
	`, column, column, column)

	rows, err := tx.Query(ctx, query, orgID, keys)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: match products by %s: %w", column, err)
	}
	defer rows.Close()

	out := make(map[string]int64, len(keys))
	for rows.Next() {
		var key string
		var id int64
		if err := rows.Scan(&key, &id); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan %s match: %w", column, err)
		}
		out[key] = id
	}
	return out, rows.Err()
}

// lookupByName resolves products by Arabic name, normalised on both sides by
// the database so the folding is identical.
func lookupByName(ctx context.Context, tx pgx.Tx, orgID int64, names []string) (map[string]int64, error) {
	const query = `
		SELECT k.raw, min(p.id) AS id
		FROM unnest($2::text[]) AS k(raw)
		JOIN catalog.products p
		  ON p.organization_id = $1
		 AND p.deleted_at IS NULL
		 AND platform.normalize_arabic(p.name->>'ar') = platform.normalize_arabic(k.raw)
		WHERE platform.normalize_arabic(k.raw) <> ''
		GROUP BY k.raw
	`

	rows, err := tx.Query(ctx, query, orgID, names)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: match products by name: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int64, len(names))
	for rows.Next() {
		var name string
		var id int64
		if err := rows.Scan(&name, &id); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan name match: %w", err)
		}
		out[name] = id
	}
	return out, rows.Err()
}

func queueInsert(batch *pgx.Batch, p *catalog.Product) {
	batch.Queue(insertProductQuery,
		p.OrganizationID, p.CategoryID, p.BrandID, p.BranchID, p.Name, p.Description,
		p.SKU, p.Barcode, p.Price, p.Discount, p.OldPrice, p.Image, p.ImageLink,
		string(insertStatus(p)), p.IsFeatured, p.DosageForm, p.ScientificName,
		p.Pharmacology, p.Active, p.Concentration, p.Unit, p.ManufacturingCompanies,
		p.InstitutionalWorkIDs,
	)
}

// insertStatus is the status a brand-new catalogue row gets: whatever the file
// stated, or active when it stated nothing. The column is NOT NULL with a CHECK,
// so an empty string would be refused.
func insertStatus(p *catalog.Product) catalog.ProductStatus {
	if p.Status == "" {
		return catalog.StatusActive
	}
	return p.Status
}

func scanInsert(br pgx.BatchResults, p *catalog.Product, res *catalog.BulkWriteResult) error {
	var id int64
	if err := br.QueryRow().Scan(&id); err != nil {
		return err
	}
	p.ID = id
	res.Inserted++
	return nil
}

func queueUpdate(batch *pgx.Batch, p *catalog.Product) {
	batch.Queue(updateProductQuery,
		p.ID, p.Name, p.Description, p.SKU, p.Barcode,
		p.Price, p.Discount, p.OldPrice, string(p.Status), p.DosageForm,
		p.ScientificName, p.Active, p.Concentration, p.Unit,
		p.ManufacturingCompanies, p.BrandID,
	)
}

func scanUpdate(br pgx.BatchResults, _ *catalog.Product, res *catalog.BulkWriteResult) error {
	if _, err := br.Exec(); err != nil {
		return err
	}
	res.Updated++
	return nil
}

// batchWriter pipelines one kind of statement — inserts or updates — over the
// products it is handed.
//
// The two operations differ only in what they queue and how they read the
// result, so they share everything else: the chunking, the savepoint discipline,
// and the per-row diagnosis after a failure.
type batchWriter struct {
	tx    pgx.Tx
	prods []*catalog.Product
	res   *catalog.BulkWriteResult
	// queue appends one product's statement to a batch.
	queue func(*pgx.Batch, *catalog.Product)
	// scan reads that statement's result and updates the running counts.
	scan func(pgx.BatchResults, *catalog.Product, *catalog.BulkWriteResult) error
}

// write sends the given product indices in pipelined chunks.
//
// On failure the chunk is replayed one statement at a time behind a savepoint,
// which is the only way to learn which row PostgreSQL objected to: once a
// statement in a batch fails the transaction is aborted, and every later
// statement reports the same "current transaction is aborted" error regardless
// of its own content. The replay costs one extra round trip per row of a single
// failing chunk, and only on the path that was going to fail anyway.
func (w *batchWriter) write(ctx context.Context, idxs []int) error {
	for start := 0; start < len(idxs); start += chunkSize {
		chunk := idxs[start:min(start+chunkSize, len(idxs))]

		// The savepoint is what makes the per-row diagnosis possible: a failed
		// statement aborts the transaction, and only a rollback to a savepoint
		// taken before the batch can make it usable again.
		if _, err := w.tx.Exec(ctx, `SAVEPOINT bulk_chunk`); err != nil {
			return fmt.Errorf("catalog postgres: open bulk savepoint: %w", err)
		}

		batchErr := w.sendChunk(ctx, chunk)
		if batchErr == nil {
			if _, err := w.tx.Exec(ctx, `RELEASE SAVEPOINT bulk_chunk`); err != nil {
				return fmt.Errorf("catalog postgres: release bulk savepoint: %w", err)
			}
			continue
		}

		w.res.Failures = w.diagnose(ctx, chunk)
		if len(w.res.Failures) == 0 {
			// The replay found nothing, so the fault was not row-specific.
			return fmt.Errorf("catalog postgres: bulk write failed: %w", batchErr)
		}
		return errors.New(w.res.Error())
	}
	return nil
}

// sendChunk pipelines one chunk and returns the first error it produced.
func (w *batchWriter) sendChunk(ctx context.Context, chunk []int) error {
	batch := &pgx.Batch{}
	for _, i := range chunk {
		w.queue(batch, w.prods[i])
	}

	br := w.tx.SendBatch(ctx, batch)
	var firstErr error
	for _, i := range chunk {
		if err := w.scan(br, w.prods[i], w.res); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := br.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// diagnose replays a failed chunk row by row to attribute the failure.
//
// The transaction is already aborted by the time this runs, so it rolls back to
// the chunk savepoint, then tries each statement inside its own savepoint.
// Nothing it writes survives: the caller returns an error either way and the
// whole transaction is rolled back.
func (w *batchWriter) diagnose(ctx context.Context, chunk []int) []catalog.WriteFailure {
	if _, err := w.tx.Exec(ctx, `ROLLBACK TO SAVEPOINT bulk_chunk`); err != nil {
		// No savepoint to return to; the caller still reports the batch error.
		return nil
	}

	var failures []catalog.WriteFailure
	for _, i := range chunk {
		err := w.tryOne(ctx, i)
		if err == nil {
			continue
		}
		failures = append(failures, catalog.WriteFailure{
			Index:  i,
			Name:   w.prods[i].Name.Get(i18n.AR),
			SKU:    w.prods[i].SKU,
			Reason: explainWriteError(err),
		})
		// Twenty named rows is already more than an admin will act on in one
		// pass, and the total count is reported separately.
		if len(failures) >= maxNamedFailures {
			break
		}
	}
	return failures
}

// tryOne runs a single product's statement behind its own savepoint, leaving the
// transaction usable whether it succeeds or fails.
func (w *batchWriter) tryOne(ctx context.Context, idx int) error {
	if _, err := w.tx.Exec(ctx, `SAVEPOINT bulk_row`); err != nil {
		return err
	}

	batch := &pgx.Batch{}
	w.queue(batch, w.prods[idx])
	br := w.tx.SendBatch(ctx, batch)
	_, err := br.Exec()
	if closeErr := br.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		_, _ = w.tx.Exec(ctx, `ROLLBACK TO SAVEPOINT bulk_row`)
		return err
	}
	_, _ = w.tx.Exec(ctx, `RELEASE SAVEPOINT bulk_row`)
	return nil
}

// explainWriteError turns a PostgreSQL error into something an admin can act on.
func explainWriteError(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err.Error()
	}

	switch pgErr.Code {
	case "23505": // unique_violation
		return "يوجد صنف آخر بنفس كود الصنف أو الباركود في الكتالوج"
	case "23503": // foreign_key_violation
		return "قيمة مرتبطة غير موجودة (التصنيف أو الشركة المصنعة أو الفرع)"
	case "23514": // check_violation
		return fmt.Sprintf("قيمة غير مسموح بها في العمود (%s)", pgErr.ConstraintName)
	case "23502": // not_null_violation
		return fmt.Sprintf("قيمة مطلوبة مفقودة في العمود «%s»", pgErr.ColumnName)
	case "22001": // string_data_right_truncation
		return "قيمة أطول من الحد المسموح به للعمود"
	case "22003": // numeric_value_out_of_range
		return "قيمة رقمية خارج النطاق المسموح به (الحد الأقصى للسعر 9,999,999,999.99)"
	case "22P02": // invalid_text_representation
		return "صيغة قيمة غير صالحة للعمود"
	default:
		return fmt.Sprintf("%s (رمز الخطأ %s)", pgErr.Message, pgErr.Code)
	}
}

// MatchExistingProducts resolves which catalogue products the incoming rows
// correspond to, without writing anything.
//
// The review screen needs the same answer the commit will reach — "this row
// updates product 4,812" — before the admin approves it. Sharing the resolver
// rather than reimplementing it is what keeps the preview honest: a row shown as
// an update cannot turn into an insert at commit time.
func (r *Repository) MatchExistingProducts(
	ctx context.Context, prods []*catalog.Product,
) (map[int]catalog.ExistingMatch, error) {
	out := map[int]catalog.ExistingMatch{}
	if len(prods) == 0 {
		return out, nil
	}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Matching is scoped per organisation, and a freshly parsed product has
		// none yet — the write path assigns it. The same assignment has to
		// happen here or the preview would match every row against organisation
		// zero, find nothing, and promise an insert for rows the commit would
		// then update.
		defaultOrgID, err := lookupDefaultOrg(txCtx, tx)
		if err != nil {
			return err
		}
		if defaultOrgID <= 0 {
			// No organisation means an empty catalogue: nothing to match.
			return nil
		}
		for _, p := range prods {
			if p.OrganizationID <= 0 {
				p.OrganizationID = defaultOrgID
			}
		}

		matches, err := resolveExistingProducts(txCtx, tx, prods)
		if err != nil {
			return err
		}
		for idx, m := range matches {
			out[idx] = catalog.ExistingMatch{ProductID: m.id, Reason: m.reason}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DefaultCatalogOrg returns the organisation that owns the master catalogue,
// creating it on a fresh deployment that has none.
//
// The import session is tied to it at creation, so a staged row and the product
// it eventually writes are scoped to the same organisation — matching a preview
// against one organisation and committing into another would show the admin one
// answer and perform a different one.
func (r *Repository) DefaultCatalogOrg(ctx context.Context) (int64, error) {
	var orgID int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var err error
		orgID, err = resolveDefaultOrg(txCtx, tx)
		return err
	})
	return orgID, err
}
