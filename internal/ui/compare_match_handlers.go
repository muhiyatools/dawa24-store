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
)

// CompareFileMatchSubmit matches one file's rows against the shared catalogue.
func (h *UIHandler) CompareFileMatchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	back := "/compare/tool"

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	if h.compareSvc == nil {
		h.redirectWithNotice(w, r, back, "error", "خدمة المقارنة غير متاحة حالياً.")
		return
	}

	fileID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || fileID <= 0 {
		h.redirectWithNotice(w, r, back, "error", "معرف ملف غير صالح.")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, back, "error", "تعذر قراءة النموذج المرسل.")
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

	stats, err := h.compareSvc.MatchFileRows(ctx, fileID, useAI, orgPtr)
	if err != nil {
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, back, "success", compareMatchNotice(stats, useAI))
}

// compareMatchNotice says what the run resolved, in the order the user cares
// about: how many rows are now tied to the catalogue, then what it cost.
func compareMatchNotice(s compare.MatchStats, useAI bool) string {
	parts := []string{
		fmt.Sprintf("تمت مطابقة %d صنف من أصل %d بالكتالوج المركزي", s.Matched(), s.Rows),
	}
	if s.Saved > 0 {
		parts = append(parts, fmt.Sprintf("%d من ربط سابق محفوظ", s.Saved))
	}
	if s.AI > 0 {
		parts = append(parts, fmt.Sprintf("%d بالذكاء الاصطناعي", s.AI))
	}
	if s.CacheHits > 0 {
		parts = append(parts, fmt.Sprintf("%d من ذاكرة القرارات بلا تكلفة", s.CacheHits))
	}
	if left := s.Rows - s.Matched(); left > 0 {
		parts = append(parts, fmt.Sprintf("%d صنف بقي بلا مطابقة ويمكن ربطه يدوياً", left))
	}
	if s.CeilingHit {
		parts = append(parts,
			"توقفت المراجعة الذكية عند حدّ العملية الواحدة واحتفظت بقية الأصناف بنتيجة المطابقة الحتمية")
	}
	if useAI && s.Requests == 0 && s.AI == 0 && s.CacheHits == 0 {
		parts = append(parts, "لم تُرسل أي طلبات ذكاء اصطناعي: لا يوجد في الكتالوج ما يقارب الأصناف المتبقية")
	}
	return strings.Join(parts, " · ") + "."
}
