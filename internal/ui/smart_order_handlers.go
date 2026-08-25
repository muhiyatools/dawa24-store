package ui

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/pipeline"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The smart ordering wizard, server-rendered.
//
// The JSON API in modules/smartorder/http serves the same use cases for
// scripted callers; these handlers are what the pharmacy actually uses. Both go
// through the same service, so there is one set of rules rather than two.

// maxUploadBytes caps a direct upload. Larger files go through the chunked
// endpoint the ingest module already provides.
const maxUploadBytes = 20 << 20

// SmartOrderNewPage renders step 1: upload and configuration.
func (h *UIHandler) SmartOrderNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/smart-order/new", http.StatusSeeOther)
		return
	}

	data := pages.SmartOrderNewData{}

	if h.orgSvc != nil {
		branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		if err == nil {
			for _, b := range branches {
				data.Branches = append(data.Branches, pages.SmartOrderBranch{
					ID:     b.ID,
					Name:   b.Name.Get(i18n.Lang(lang)),
					City:   b.Address,
					IsMain: b.IsMain,
				})
			}
		}
	}

	if h.smartOrderSvc != nil {
		if p, err := h.smartOrderSvc.Profile(ctx, actor.OrganizationID); err == nil {
			data.Profile = p
		}
	}

	// The AI toggle is only offered when it would actually work. Rendering it
	// enabled against an unreachable Gateway produces a run that silently does
	// no AI at all, which reads as the feature being broken.
	data.AIAvailable, data.AIUnavailableReason = h.smartOrderAIState(ctx, actor.OrganizationID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SmartOrderNewPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render smart order import page", "error", err)
	}
}

// SmartOrderCreateSubmit handles the upload and creates the run.
func (h *UIHandler) SmartOrderCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	if h.smartOrderSvc == nil {
		http.Error(w, "smart ordering is not configured", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		h.smartOrderFail(w, r, "تعذّر رفع الملف. تأكد من أن حجمه لا يتجاوز 20 ميجابايت.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.smartOrderFail(w, r, "اختر ملف الأصناف المطلوبة.")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		h.smartOrderFail(w, r, "الملف فارغ أو تعذّرت قراءته.")
		return
	}

	// Inspect before creating anything: an unreadable file should not leave a
	// half-formed run in the buyer's history.
	parsed, err := pipeline.Inspect(content, header.Filename)
	if err != nil {
		h.smartOrderFail(w, r, "تعذّرت قراءة الملف: "+err.Error())
		return
	}

	branchID, _ := strconv.ParseInt(r.FormValue("branch_id"), 10, 64)
	tolerance, _ := strconv.ParseFloat(r.FormValue("tolerance_pct"), 64)
	defaultQty, _ := strconv.Atoi(r.FormValue("default_quantity"))

	var budget *money.Amount
	if s := strings.TrimSpace(r.FormValue("max_budget")); s != "" {
		if amt, err := money.Parse(s); err == nil {
			budget = &amt
		}
	}

	run, err := h.smartOrderSvc.Start(ctx, smartorder.StartOptions{
		UserID:            actor.UserID,
		OrganizationID:    actor.OrganizationID,
		BranchID:          branchID,
		Filename:          header.Filename,
		Criteria:          orderedCriteria(r),
		TolerancePct:      tolerance,
		DefaultQuantity:   defaultQty,
		MaxBudget:         budget,
		UseSavingProducts: r.FormValue("use_saving_products") != "",
		UseAIMatching:     r.FormValue("use_ai_matching") != "",
	})
	if err != nil {
		h.smartOrderFail(w, r, translateSmartOrderError(err))
		return
	}

	// The file is held for the mapping step. Storing it against the run rather
	// than in the session is what lets the buyer come back to it later.
	h.smartOrderFiles.put(run.PublicID, content, header.Filename)
	_ = parsed

	http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/mapping", http.StatusSeeOther)
}

// orderedCriteria reads the enabled criteria in the priority the buyer set.
//
// The form posts a checkbox and a priority number per criterion, because a
// drag-ordered list degrades badly without JavaScript and this screen must work
// without it.
func orderedCriteria(r *http.Request) []smartorder.Criterion {
	type entry struct {
		key      smartorder.Criterion
		priority int
	}
	var entries []entry
	for _, key := range r.Form["criteria"] {
		p, err := strconv.Atoi(r.FormValue("priority_" + key))
		if err != nil {
			p = 99
		}
		entries = append(entries, entry{key: smartorder.Criterion(key), priority: p})
	}
	// Insertion sort: at most three entries.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].priority < entries[j-1].priority; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	out := make([]smartorder.Criterion, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.key)
	}
	return out
}

// SmartOrderMappingPage renders step 2.
func (h *UIHandler) SmartOrderMappingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	content, filename, found := h.smartOrderFiles.get(run.PublicID)
	if !found {
		h.smartOrderFail(w, r, "انتهت صلاحية الملف المرفوع. يرجى رفعه مرة أخرى.")
		return
	}

	parsed, err := pipeline.Inspect(content, filename)
	if err != nil {
		h.smartOrderFail(w, r, "تعذّرت قراءة الملف: "+err.Error())
		return
	}

	data := pages.SmartOrderMappingData{
		Run:       run,
		Headers:   parsed.Headers,
		Preview:   parsed.Preview,
		HeaderRow: parsed.HeaderRow,
	}
	for _, f := range pipeline.TargetFields {
		field := pages.SmartOrderMappingField{
			Key: f.Key, Label: f.LabelAR, Required: f.Required,
			Confidence: parsed.Confidence[f.Key],
		}
		for col, assigned := range parsed.Detected {
			if assigned == f.Key {
				field.Column, field.Assigned = col, true
				break
			}
		}
		data.Fields = append(data.Fields, field)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SmartOrderMappingPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render smart order mapping page", "error", err)
	}
}

// SmartOrderMappingSubmit stages the rows and queues the run.
func (h *UIHandler) SmartOrderMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		h.smartOrderFail(w, r, "تعذّرت قراءة النموذج.")
		return
	}

	content, filename, found := h.smartOrderFiles.get(run.PublicID)
	if !found {
		h.smartOrderFail(w, r, "انتهت صلاحية الملف المرفوع. يرجى رفعه مرة أخرى.")
		return
	}

	headerRow, _ := strconv.Atoi(r.FormValue("header_row"))
	fields := make(map[int]string)
	for _, f := range pipeline.TargetFields {
		v := strings.TrimSpace(r.FormValue("mapping_" + f.Key))
		if v == "" {
			continue
		}
		if col, err := strconv.Atoi(v); err == nil {
			fields[col] = f.Key
		}
	}

	m := &smartorder.Mapping{HeaderRow: headerRow, Fields: fields, UserOverridden: true}
	if err := h.smartOrderSvc.ConfirmMapping(ctx, run, m); err != nil {
		h.smartOrderFail(w, r, translateSmartOrderError(err))
		return
	}

	lines, err := pipeline.Stage(content, filename, m, run.ID, run.OrganizationID)
	if err != nil {
		h.smartOrderFail(w, r, "تعذّر تجهيز الصفوف: "+err.Error())
		return
	}
	if len(lines) == 0 {
		h.smartOrderFail(w, r, "لم يُعثر على أي صنف في الملف بعد تطبيق التعيين.")
		return
	}
	if err := h.smartOrderSvc.StageLines(ctx, lines); err != nil {
		h.smartOrderFail(w, r, "تعذّر حفظ الصفوف.")
		return
	}
	if err := h.smartOrderSvc.Queue(ctx, run); err != nil {
		h.smartOrderFail(w, r, translateSmartOrderError(err))
		return
	}
	if h.smartOrderEnqueue != nil {
		if err := h.smartOrderEnqueue(ctx, run.ID, run.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, "could not queue smart order run", "run_id", run.ID, "error", err)
		}
	}

	h.smartOrderFiles.drop(run.PublicID)
	http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/progress", http.StatusSeeOther)
}

// smartOrderRun resolves the run in the URL for the caller's organisation.
func (h *UIHandler) smartOrderRun(w http.ResponseWriter, r *http.Request) (*smartorder.Run, bool) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return nil, false
	}
	if h.smartOrderSvc == nil {
		http.Error(w, "smart ordering is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	run, err := h.smartOrderSvc.Get(ctx, actor.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		// Not Found rather than Forbidden: a 403 would confirm the run exists.
		http.NotFound(w, r)
		return nil, false
	}
	return run, true
}

func (h *UIHandler) smartOrderFail(w http.ResponseWriter, r *http.Request, message string) {
	h.log.WarnContext(r.Context(), "smart order step refused", "message", message)
	http.Redirect(w, r, "/customer/smart-order/new?error="+message, http.StatusSeeOther)
}

// translateSmartOrderError turns a domain error into something a pharmacist can
// act on, rather than an error code.
func translateSmartOrderError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "branch_required"):
		return "اختر فرع التسليم قبل المتابعة."
	case strings.Contains(msg, "mapping_incomplete"):
		return "عيّن عمودًا لاسم الصنف أو كوده أو الباركود قبل بدء المطابقة."
	case strings.Contains(msg, "already_finalized"):
		return "تم اعتماد هذا الطلب من قبل."
	case strings.Contains(msg, "stale"):
		return "تغيّرت الإعدادات بعد إنشاء النتائج. أعد التشغيل قبل الاعتماد."
	}
	return "تعذّر إكمال العملية."
}
