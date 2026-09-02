package ui

// Running the catalogue matching stage over one uploaded price list.
//
// The compare tool has always had a place to put a catalogue link on every row
// and no way to produce one, so every comparison it has made was built by
// joining supplier lines to each other on a normalised string. This is the
// button that fills that column, and the AI switch beside it is the same switch
// the vendor import and the smart order offer, doing the same thing: it decides
// whether the rows the deterministic engine cannot settle get a second opinion.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// CompareFileMatchSubmit matches one file's rows against the shared catalogue.
func (h *UIHandler) CompareFileMatchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	back := "/compare/tool"

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	if h.compareSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "compare.match.service_unavailable"))
		return
	}

	fileID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || fileID <= 0 {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "compare.match.form_error"))
		return
	}

	// The switch is opt-in per run rather than a stored setting, because it is
	// a spending decision and the person making it is watching the screen.
	useAI := r.PostFormValue("use_ai") == "1" || r.PostFormValue("use_ai") == "on"
	if useAI && !h.compareSvc.AIMatchingAvailable() {
		useAI = false
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	err = h.compareSvc.StartBackgroundCatalogMatch(fileID, useAI, orgPtr)
	if err != nil {
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, back, "success", "بدأت عملية المطابقة مع الكتالوج في الخلفية بنجاح. يمكنك متابعة العمل وسيتحدث الملف تلقائياً.")
}

// compareMatchNotice says what the run resolved, in the order the user cares
// about: how many rows are now tied to the catalogue, then what it cost.
func compareMatchNotice(s compare.MatchStats, useAI bool, lang any) string {
	parts := []string{
		fmt.Sprintf(i18n.T(lang, "compare.match.matched_summary"), s.Matched(), s.Rows),
	}
	if s.Saved > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T(lang, "compare.match.from_saved"), s.Saved))
	}
	if s.AI > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T(lang, "compare.match.by_ai"), s.AI))
	}
	if s.CacheHits > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T(lang, "compare.match.cache_hits"), s.CacheHits))
	}
	if left := s.Rows - s.Matched(); left > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T(lang, "compare.match.unmatched_left"), left))
	}
	if s.CeilingHit {
		parts = append(parts,
			i18n.T(lang, "compare.match.ceiling_hit"))
	}
	if useAI && s.Requests == 0 && s.AI == 0 && s.CacheHits == 0 {
		parts = append(parts, i18n.T(lang, "compare.match.no_ai_candidates"))
	}
	return strings.Join(parts, " · ") + "."
}
