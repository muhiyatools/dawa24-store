package productmatch

import ()

// Header match strengths. The gaps are wide on purpose: an exact header match
// must beat any number of partial ones, and a full multi-word term a field owns
// must beat a single word several fields share.
const (
	scoreExact  = 100
	scoreStrong = 60
	scoreWeak   = 25
	scoreFloor  = scoreWeak
)

// headerSpec is how one field is recognised in a header cell.
//
// exact matches the whole normalised header. strong and weak are substring
// tests. blocked disqualifies the pair outright, which is how "سعر التكلفة" is
// kept away from the public price and "الباركود الدولي" away from the item
// code — the two collisions that quietly corrupt an imported price list.
type headerSpec struct {
	field   Field
	exact   []string
	strong  []string
	weak    []string
	blocked []string
}
