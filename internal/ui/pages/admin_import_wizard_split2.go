package pages

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// SessionStatusBadge picks the badge colour for a session state.
func SessionStatusBadge(status catalog.SessionStatus) string {
	switch status {
	case catalog.SessionCommitted:
		return "badge-emerald"
	case catalog.SessionProcessing:
		return "badge-sky"
	case catalog.SessionReady:
		return "badge-amber"
	case catalog.SessionFailed:
		return "badge-rose"
	default:
		return "badge-slate"
	}
}

// ProductSummaryLine renders a staged product's details for the review table.
func ProductSummaryLine(row *catalog.StagingRow) string {
	if row == nil {
		return ""
	}
	return catalog.SummarizeProduct(row.Product)
}

// FormatCount renders a number with thousands separators, which matters when
// the figure on screen is 8,790 rather than 12.
func FormatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(digit)
	}
	return b.String()
}

// URL helpers for the review screen.
//
// templ.SafeURL is the escape hatch that says "this string is a trusted URL".
// Every one of these is built from a session's own UUID and fixed path
// segments, never from anything a file or a query string supplied.

// importAction builds a POST target on the session.
func importAction(view ImportReviewView, verb string) templ.SafeURL {
	return templ.SafeURL(fmt.Sprintf("/admin/products/import/%s/%s", view.Session.PublicID, verb))
}

// importRowAction builds the include/exclude target for one staged row.
func importRowAction(view ImportReviewView, row *catalog.StagingRow) templ.SafeURL {
	return templ.SafeURL(fmt.Sprintf("/admin/products/import/%s/rows/%d",
		view.Session.PublicID, row.ID))
}

// includedToggleValue is what the row's button submits: the opposite of where
// the row is now, so one click flips it.
func includedToggleValue(row *catalog.StagingRow) string {
	if row.Included {
		return "0"
	}
	return "1"
}

// importPageURL keeps the active filter while moving through pages.
func importPageURL(view ImportReviewView, page int) templ.SafeURL {
	return templ.SafeURL("/admin/products/import/" + view.Session.PublicID +
		"?" + importQuery(view, view.currentFilterKey(), page).Encode())
}

// importFilterURL switches the active filter and returns to the first page.
func importFilterURL(view ImportReviewView, key string) templ.SafeURL {
	return templ.SafeURL("/admin/products/import/" + view.Session.PublicID +
		"?" + importQuery(view, key, 1).Encode())
}

func importQuery(view ImportReviewView, filterKey string, page int) url.Values {
	q := url.Values{}
	if filterKey != "" {
		q.Set("filter", filterKey)
	}
	if view.Filter.Search != "" {
		q.Set("q", view.Filter.Search)
	}
	if page > 1 {
		q.Set("page", fmt.Sprintf("%d", page))
	}
	return q
}

// currentFilterKey renders the active filter back into its query value.
func (v ImportReviewView) currentFilterKey() string {
	switch {
	case v.Filter.OnlyIssues:
		return "issues"
	case v.Filter.OnlyAI:
		return "ai"
	case v.Filter.Action != "":
		return string(v.Filter.Action)
	default:
		return ""
	}
}

// FilterIsActive reports whether a chip is the one currently applied.
func (v ImportReviewView) FilterIsActive(key string) bool {
	return v.currentFilterKey() == key
}

// ParseStagingFilter reads the review table's controls out of a query string.
func ParseStagingFilter(values url.Values, pageSize int) catalog.StagingFilter {
	filter := catalog.StagingFilter{
		Search: strings.TrimSpace(values.Get("q")),
		Limit:  pageSize,
	}

	switch values.Get("filter") {
	case string(catalog.ActionInsert):
		filter.Action = catalog.ActionInsert
	case string(catalog.ActionUpdate):
		filter.Action = catalog.ActionUpdate
	case string(catalog.ActionSkip):
		filter.Action = catalog.ActionSkip
	case "issues":
		filter.OnlyIssues = true
	case "ai":
		filter.OnlyAI = true
	}

	if page, err := strconv.Atoi(values.Get("page")); err == nil && page > 1 {
		filter.Offset = (page - 1) * pageSize
	}
	return filter
}

// SetRows fills the review table after the rows have been read.
//
// Paging is computed here rather than at construction because the rows are
// fetched only for a session that has finished preparing; one in flight is
// rendered without them.
func (v *ImportReviewView) SetRows(rows []*catalog.StagingRow, total int, counts catalog.StagingCounts) {
	v.Rows, v.Total, v.Counts = rows, total, counts

	limit := v.Filter.Limit
	if limit <= 0 {
		limit = 100
	}
	v.Page = v.Filter.Offset/limit + 1
	v.Pages = max((total+limit-1)/limit, 1)
}
