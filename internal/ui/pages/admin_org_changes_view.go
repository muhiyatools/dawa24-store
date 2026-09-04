package pages

import "net/url"

// orgChangesHref builds one tab's link. An empty status means "all", and it is
// left off the URL rather than sent as status="" so the query string says what
// it means.
func orgChangesHref(status string) string {
	if status == "" {
		return "/admin/organizations/change-requests"
	}
	return "/admin/organizations/change-requests?status=" + url.QueryEscape(status)
}

// orgChangesQuery keeps the active tab across a page change: paging inside
// "قيد المراجعة" must not silently drop back to every request ever made.
func orgChangesQuery(status string) url.Values {
	vals := url.Values{}
	if status != "" {
		vals.Set("status", status)
	}
	return vals
}
