package ingest

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest/engine"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Which of the vendor's own variants is this row about?
//
// This is a different question from "which catalogue product is this", and it
// has to be answered second, because the answer usually depends on the first.
// A vendor may stock the same catalogue product in three pack sizes and two
// batches; those are three variants, and updating the wrong one silently moves
// stock between them.
//
// The whole of a vendor's catalogue is loaded once and indexed here, so the
// question costs a map lookup per row rather than the three queries the
// previous importer issued.

// variantIndex resolves a parsed row onto an existing variant.
type variantIndex struct {
	bySKU     map[string]int64
	byBarcode map[string]int64
	// byProductPack keys on the catalogue product plus the packaging that
	// distinguishes two variants of it: the unit and the batch.
	byProductPack map[string]int64
	// byProduct is the last resort and holds an id only where the vendor has
	// exactly one variant of that product. Where they have several, the entry
	// is deliberately absent rather than arbitrary.
	byProduct map[int64]int64
}

// newVariantIndex indexes a vendor's existing variants.
func newVariantIndex(keys []catalog.VariantKey) *variantIndex {
	idx := &variantIndex{
		bySKU:         make(map[string]int64, len(keys)),
		byBarcode:     make(map[string]int64, len(keys)),
		byProductPack: make(map[string]int64, len(keys)),
		byProduct:     make(map[int64]int64, len(keys)),
	}
	// ambiguous marks a product the vendor stocks more than once, so the
	// product-only fallback refuses to guess.
	ambiguous := make(map[int64]bool, len(keys))

	for _, k := range keys {
		if key := sheet.NormalizeKey(k.SKU); key != "" {
			idx.bySKU[key] = k.ID
		}
		if code := sheet.DigitsOnly(k.Barcode); code != "" {
			idx.byBarcode[code] = k.ID
		}
		if k.ProductID > 0 {
			idx.byProductPack[packKey(k.ProductID, k.Unit, k.BatchNumber)] = k.ID
			if _, seen := idx.byProduct[k.ProductID]; seen {
				ambiguous[k.ProductID] = true
				continue
			}
			idx.byProduct[k.ProductID] = k.ID
		}
	}
	for id := range ambiguous {
		delete(idx.byProduct, id)
	}
	return idx
}

// packKey identifies one packaging of one catalogue product.
func packKey(productID int64, unit, batch string) string {
	return sheet.NormalizeKey(unit) + "|" + sheet.NormalizeKey(batch) + "|" + itoa64(productID)
}

// resolve finds the existing variant a row refers to, and says how.
//
// The order is by how much the identifier means. The vendor's own item code is
// the strongest: it is the number they use to mean "this line of my catalogue".
// The barcode is next but weaker than it looks — several pack sizes of one
// product legitimately share one — so it only settles a row that carried no
// code. Everything after that is inference and is treated as such.
func (idx *variantIndex) resolve(row *engine.Row, productID int64) (int64, string) {
	if key := sheet.NormalizeKey(row.SKU); key != "" {
		if id, ok := idx.bySKU[key]; ok {
			return id, "مطابقة بكود الصنف لديك"
		}
	}
	if code := sheet.DigitsOnly(row.Barcode); code != "" {
		if id, ok := idx.byBarcode[code]; ok {
			return id, "مطابقة بالباركود"
		}
	}
	if productID > 0 {
		if id, ok := idx.byProductPack[packKey(productID, row.Unit, row.BatchNumber)]; ok {
			return id, "مطابقة بالصنف والعبوة ورقم التشغيلة"
		}
		if id, ok := idx.byProduct[productID]; ok {
			return id, "مطابقة بالصنف المرتبط"
		}
	}
	return 0, ""
}

// remember records a newly written variant so a later row in the same file
// updates it rather than inserting a second copy of it.
func (idx *variantIndex) remember(row *engine.Row, productID, variantID int64) {
	if variantID <= 0 {
		return
	}
	if key := sheet.NormalizeKey(row.SKU); key != "" {
		idx.bySKU[key] = variantID
	}
	if code := sheet.DigitsOnly(row.Barcode); code != "" {
		idx.byBarcode[code] = variantID
	}
	if productID > 0 {
		idx.byProductPack[packKey(productID, row.Unit, row.BatchNumber)] = variantID
		if _, seen := idx.byProduct[productID]; !seen {
			idx.byProduct[productID] = variantID
		}
	}
}

// itoa64 renders an id without pulling in strconv for one call site.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
