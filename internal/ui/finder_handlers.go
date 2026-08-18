package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// FinderPage renders the first question of the guided product finder.
func (h *UIHandler) FinderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !features.Enabled(ctx, "finder.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if h.catSvc == nil {

		http.Redirect(w, r, "/catalog", http.StatusSeeOther)
		return
	}
	q, err := h.catSvc.GetFirstFinderQuestion(ctx)
	if err != nil {
		// No questionnaire configured yet.
		h.renderFinderQuestion(w, r, &catalog.FinderQuestion{Question: i18n.New("لا توجد أسئلة بعد", "No questions yet")})
		return
	}
	h.renderFinderQuestion(w, r, q)
}

// FinderQuestionByIDPage renders a specific question (used after answering).
func (h *UIHandler) FinderQuestionByIDPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		http.Redirect(w, r, "/catalog", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/finder", http.StatusSeeOther)
		return
	}
	q, err := h.catSvc.GetFinderQuestion(ctx, id)
	if err != nil {
		http.Redirect(w, r, "/finder", http.StatusSeeOther)
		return
	}
	h.renderFinderQuestion(w, r, q)
}

// FinderAnswerSubmit resolves an answer to the next question or a result.
func (h *UIHandler) FinderAnswerSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		http.Redirect(w, r, "/finder", http.StatusSeeOther)
		return
	}
	optionID, _ := strconv.ParseInt(r.PostFormValue("option_id"), 10, 64)
	questionID, _ := strconv.ParseInt(r.PostFormValue("question_id"), 10, 64)
	options, _ := h.catSvc.ListFinderOptions(ctx, questionID)
	for _, o := range options {
		if o.ID == optionID {
			if o.NextQuestionID != nil {
				http.Redirect(w, r, fmt.Sprintf("/finder/%d", *o.NextQuestionID), http.StatusSeeOther)
				return
			}
			if o.ResultID != nil {
				http.Redirect(w, r, fmt.Sprintf("/finder/result/%d", *o.ResultID), http.StatusSeeOther)
				return
			}
		}
	}
	http.Redirect(w, r, "/finder", http.StatusSeeOther)
}

// FinderResultByIDPage renders the terminal recommendation.
func (h *UIHandler) FinderResultByIDPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	if h.catSvc == nil {
		http.Redirect(w, r, "/finder", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/finder", http.StatusSeeOther)
		return
	}
	res, err := h.catSvc.GetFinderResult(ctx, id)
	if err != nil {
		http.Redirect(w, r, "/finder", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.FinderResultPage(lang, dir, res).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render finder result", "error", err)
	}
}

func (h *UIHandler) renderFinderQuestion(w http.ResponseWriter, r *http.Request, q *catalog.FinderQuestion) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	options, _ := h.catSvc.ListFinderOptions(ctx, q.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.FinderQuestionPage(lang, dir, q, options).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render finder question", "error", err)
	}
}
