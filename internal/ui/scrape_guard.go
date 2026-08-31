package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/antiscrape"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// SetScrapeGuard installs the anti-scraping guard for the public surface.
//
// A setter rather than a constructor argument for the same reason as every
// other optional dependency here: NewUIHandler already takes fifteen, and the
// guard needs a Redis handle that is not dialled when routes are mounted.
func (h *UIHandler) SetScrapeGuard(g *antiscrape.Guard) { h.scrape = g }

// SetGuestListingLimits bounds how far a signed-out caller may page into a
// public listing.
//
// This is the ceiling on total exposure, and it is the part of the defence a
// forged User-Agent cannot walk around: budgets decide how fast the catalogue
// can be read, this decides how much of it is reachable without an account at
// all. Zero for either value leaves that dimension unbounded.
func (h *UIHandler) SetGuestListingLimits(maxPage, maxPageSize int) {
	h.guestMaxPage, h.guestMaxPageSize = maxPage, maxPageSize
}

// guestListingBounds returns the page and page-size ceilings that apply to this
// request: the configured guest limits when nobody is signed in, and the
// caller-supplied defaults otherwise.
func (h *UIHandler) guestListingBounds(r *http.Request, maxPage, maxPageSize int) (int, int) {
	if actor, ok := authctx.From(r.Context()); ok && actor.UserID > 0 {
		return maxPage, maxPageSize
	}
	if h.guestMaxPage > 0 && h.guestMaxPage < maxPage {
		maxPage = h.guestMaxPage
	}
	if h.guestMaxPageSize > 0 && h.guestMaxPageSize < maxPageSize {
		maxPageSize = h.guestMaxPageSize
	}
	return maxPage, maxPageSize
}

// ScrapeTrap answers the honeypot paths. See the route comment for what makes
// a caller reach one.
func (h *UIHandler) ScrapeTrap(w http.ResponseWriter, r *http.Request) {
	if h.scrape == nil {
		http.NotFound(w, r)
		return
	}
	h.scrape.Trap(w, r)
}
