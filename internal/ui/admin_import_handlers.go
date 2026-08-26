package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The catalogue import wizard.
//
// Upload, review, confirm. The screen this replaced did all three in one POST
// and reported the outcome by redirecting with the message in the query string,
// which is how a failed import of nine thousand rows ended up as a wall of
// percent-encoded Arabic in the address bar. Nothing here writes to the
// catalogue until the admin has seen what the write will do.

// maxImportUploadBytes caps an uploaded catalogue file. The largest real
// distributor export seen is a few megabytes; 32 MB leaves ample headroom while
// keeping a single request from holding that much memory.
const maxImportUploadBytes int64 = 32 << 20

// maxImportRequestBytes caps the whole request body: the file plus generous
// room for the multipart envelope and form fields. Without it a client can
// stream an arbitrarily large "upload" that the server spools to disk in full
// before anything checks the size.
const maxImportRequestBytes int64 = maxImportUploadBytes + (1 << 20)

// requirePlatformAdmin refuses any catalogue-import action from an actor that
// is not platform staff. The routes carry middleware that should already have
// let nothing else through; this re-checks at the feature because these
// endpoints write to the shared master catalogue and every one of them runs
// under AsSystem, which bypasses tenant scoping entirely.
func (h *UIHandler) requirePlatformAdmin(w http.ResponseWriter, r *http.Request) bool {
	actor, ok := authctx.From(r.Context())
	if !ok || !actor.IsPlatformAdmin() {
		http.Error(w, "صلاحيات غير كافية لتنفيذ هذه العملية.", http.StatusForbidden)
		return false
	}
	return true
}

// reviewPageSize is how many staged rows one page of the review table shows.
// A browser handed nine thousand table rows becomes unusable.
const reviewPageSize = 100

// AdminProductsImportPage is step one: choose a file, a strategy, and what the
// system should fill in.
func (h *UIHandler) AdminProductsImportPage(w http.ResponseWriter, r *http.Request) {
	h.renderImportConfigure(w, r, pages.ImportConfigureView{}, http.StatusOK)
}

// renderImportConfigure draws the upload screen, optionally carrying a rejection
// from a previous attempt.
func (h *UIHandler) renderImportConfigure(
	w http.ResponseWriter, r *http.Request, seed pages.ImportConfigureView, status int,
) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	view := pages.NewImportConfigureView(
		h.importCategories(ctx),
		h.recentImportSessions(ctx),
		h.catSvc != nil && h.catSvc.AIAvailable(ctx),
	)
	view.Fatal, view.FatalDetail = seed.Fatal, seed.FatalDetail

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.AdminProductsImportPage(lang, dir, view).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render import wizard", "error", err)
	}
}

// AdminProductsImportSubmit is step one's action: read the file, analyse it,
// stage it, and send the admin to the review screen.
func (h *UIHandler) AdminProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !h.requirePlatformAdmin(w, r) {
		return
	}

	if h.catSvc == nil {
		h.renderImportConfigure(w, r, pages.ImportConfigureView{
			Fatal: "خدمة الكتالوج غير متاحة حالياً. يرجى المحاولة بعد قليل أو التواصل مع الدعم الفني.",
		}, http.StatusServiceUnavailable)
		return
	}

	content, filename, uploadErr := readUploadedFile(r)
	if uploadErr != nil {
		h.log.WarnContext(ctx, "import upload rejected", "error", uploadErr)
		h.renderImportConfigure(w, r, pages.ImportConfigureView{
			Fatal: uploadErr.message, FatalDetail: uploadErr.detail,
		}, http.StatusUnprocessableEntity)
		return
	}

	sysCtx := database.AsSystem(ctx)
	session, _, err := h.catSvc.AnalyzeImport(sysCtx, content, filename, actorUserID(ctx))
	if err != nil {
		h.log.WarnContext(ctx, "import file unreadable", "file", filename, "error", err)
		h.renderImportConfigure(w, r, pages.ImportConfigureView{
			Fatal: h.importMessage(err, r),
		}, http.StatusUnprocessableEntity)
		return
	}

	// The settings the admin chose on the upload screen apply to this first
	// pass, so the review they land on already reflects their choices rather
	// than a default run they would have to redo.
	//
	// Started in the background: with AI enabled a large file is minutes of
	// work, and holding the POST open for it gives the admin a hung browser and
	// a request that may outlive its own timeout.
	if err := h.catSvc.PrepareImportAsync(sysCtx, session.PublicID, readImportSettings(r)); err != nil {
		h.log.ErrorContext(ctx, "import preparation could not start",
			"session", session.PublicID, "error", err)
		h.renderImportConfigure(w, r, pages.ImportConfigureView{
			Fatal: h.importMessage(err, r),
		}, http.StatusUnprocessableEntity)
		return
	}

	http.Redirect(w, r, "/admin/products/import/"+session.PublicID, http.StatusSeeOther)
}

// AdminProductsImportReviewPage is step two: the staged result.
func (h *UIHandler) AdminProductsImportReviewPage(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlatformAdmin(w, r) {
		return
	}
	h.renderImportReview(w, r, "", http.StatusOK)
}

// renderImportReview draws the review screen for the session in the URL.
func (h *UIHandler) renderImportReview(w http.ResponseWriter, r *http.Request, notice string, status int) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	publicID := chi.URLParam(r, "id")

	if h.catSvc == nil {
		http.Error(w, "catalog service unavailable", http.StatusServiceUnavailable)
		return
	}

	sysCtx := database.AsSystem(ctx)
	filter := pages.ParseStagingFilter(r.URL.Query(), reviewPageSize)

	session, rows, total, err := h.catSvc.ListStagingRows(sysCtx, publicID, filter)
	if err != nil {
		h.log.WarnContext(ctx, "import session unavailable", "session", publicID, "error", err)
		h.renderImportConfigure(w, r, pages.ImportConfigureView{
			Fatal: "لم يتم العثور على جلسة الاستيراد المطلوبة أو انتهت صلاحيتها. يرجى رفع الملف من جديد.",
		}, http.StatusNotFound)
		return
	}

	counts, err := h.catSvc.StagingCounts(sysCtx, session.ID)
	if err != nil {
		h.log.ErrorContext(ctx, "staging counts unavailable", "session", publicID, "error", err)
	}

	view := pages.NewImportReviewView(session, counts, rows, total, filter,
		h.importCategories(ctx), h.catSvc.AIAvailable(ctx))
	view.Notice = notice

	// A run still in flight means the staged rows below belong to the previous
	// pass, so the page shows progress rather than numbers that are about to
	// change under the admin.
	if progress, running := h.catSvc.ImportProgress(publicID); running && !progress.Phase.Terminal() {
		view.Progress, view.Working = progress, true
	} else {
		h.attachImportStructure(sysCtx, &view, session)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.AdminProductsImportReview(lang, dir, view).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render import review", "error", err)
	}
}

// attachImportStructure fills the column-mapping panel by re-reading the stored
// file's header. It is best-effort: a session whose file has already been
// released still shows its counts and rows, just without the mapping table.
func (h *UIHandler) attachImportStructure(
	ctx context.Context, view *pages.ImportReviewView, session *catalog.ImportSession,
) {
	plan, err := h.catSvc.ImportStructure(ctx, session.PublicID)
	if err != nil {
		h.log.DebugContext(ctx, "import structure unavailable",
			"session", session.PublicID, "error", err)
		return
	}
	view.SetStructure(plan)
}

// AdminProductsImportPrepare re-runs a session under corrected settings.
func (h *UIHandler) AdminProductsImportPrepare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")

	if !h.requirePlatformAdmin(w, r) {
		return
	}

	if h.catSvc == nil {
		http.Error(w, "catalog service unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderImportReview(w, r, "تعذرت قراءة الإعدادات المرسلة.", http.StatusBadRequest)
		return
	}

	if err := h.catSvc.PrepareImportAsync(database.AsSystem(ctx), publicID, readImportSettings(r)); err != nil {
		h.log.ErrorContext(ctx, "import re-preparation could not start", "session", publicID, "error", err)
		h.renderImportReview(w, r, h.importMessage(err, r), http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/admin/products/import/"+publicID, http.StatusSeeOther)
}

// AdminProductsImportRowToggle includes or excludes one staged row.
func (h *UIHandler) AdminProductsImportRowToggle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")

	if !h.requirePlatformAdmin(w, r) {
		return
	}

	if h.catSvc == nil {
		http.Error(w, "catalog service unavailable", http.StatusServiceUnavailable)
		return
	}
	rowID, err := strconv.ParseInt(chi.URLParam(r, "rowID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid row", http.StatusBadRequest)
		return
	}

	included := r.PostFormValue("included") == "1"
	if err := h.catSvc.SetRowIncluded(database.AsSystem(ctx), publicID, rowID, included); err != nil {
		h.log.WarnContext(ctx, "could not toggle staged row",
			"session", publicID, "row", rowID, "error", err)
	}

	if r.Header.Get("HX-Request") == "true" {
		row, err := h.catSvc.GetStagingRow(database.AsSystem(ctx), publicID, rowID)
		if err == nil && row != nil {
			view := pages.ImportReviewView{Session: &catalog.ImportSession{PublicID: publicID}}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := pages.ImportRowLine(view, row).Render(ctx, w); err == nil {
				return
			}
		}
	}

	// Back to the same page and filter the admin was looking at, so toggling a
	// row on page 40 does not throw them back to page 1.
	http.Redirect(w, r, "/admin/products/import/"+publicID+querySuffix(r), http.StatusSeeOther)
}

// AdminProductsImportSelect includes or excludes every row sharing an action.
func (h *UIHandler) AdminProductsImportSelect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")

	if !h.requirePlatformAdmin(w, r) {
		return
	}

	if h.catSvc == nil {
		http.Error(w, "catalog service unavailable", http.StatusServiceUnavailable)
		return
	}

	action := catalog.RowAction(r.PostFormValue("action"))
	included := r.PostFormValue("included") == "1"
	affected, err := h.catSvc.SetRowsIncludedByAction(database.AsSystem(ctx), publicID, action, included)
	if err != nil {
		h.renderImportReview(w, r, h.importMessage(err, r), http.StatusUnprocessableEntity)
		return
	}

	h.log.InfoContext(ctx, "bulk staged row selection",
		"session", publicID, "action", action, "included", included, "rows", affected)
	http.Redirect(w, r, "/admin/products/import/"+publicID+querySuffix(r), http.StatusSeeOther)
}

// AdminProductsImportCommit is step three: write the reviewed rows.
func (h *UIHandler) AdminProductsImportCommit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")

	if !h.requirePlatformAdmin(w, r) {
		return
	}

	if h.catSvc == nil {
		http.Error(w, "catalog service unavailable", http.StatusServiceUnavailable)
		return
	}

	sysCtx := database.AsSystem(ctx)
	session, err := h.catSvc.GetImportSession(sysCtx, publicID)
	if err != nil {
		h.renderImportConfigure(w, r, pages.ImportConfigureView{
			Fatal: "لم يتم العثور على جلسة الاستيراد المطلوبة أو انتهت صلاحيتها.",
		}, http.StatusNotFound)
		return
	}

	// Archiving the catalogue is not something to do on a mis-click, so the
	// destructive strategy needs its own deliberate acknowledgement.
	if session.Mode.IsDestructive() && r.PostFormValue("confirm_destructive") != "1" {
		h.renderImportReview(w, r,
			"يجب تأكيد أرشفة الكتالوج الحالي قبل تنفيذ هذه الطريقة.", http.StatusUnprocessableEntity)
		return
	}

	written, result, err := h.catSvc.CommitImport(sysCtx, publicID)
	if err != nil {
		h.log.ErrorContext(ctx, "import commit failed",
			"session", publicID, "failures", len(result.Failures), "error", err)
		h.renderImportReview(w, r, h.importMessage(err, r), http.StatusUnprocessableEntity)
		return
	}

	h.refreshProductIndex(ctx)
	h.log.InfoContext(ctx, "import committed",
		"session", written.PublicID, "inserted", result.Inserted, "updated", result.Updated)

	h.redirectWithNotice(w, r, "/admin/products", "success", fmt.Sprintf(
		"تم حفظ %d صنف في الكتالوج المعتمد (%d جديد، %d محدَّث).",
		result.Total(), result.Inserted, result.Updated))
}

// AdminProductsImportCancel discards a session without touching the catalogue.
func (h *UIHandler) AdminProductsImportCancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")

	if !h.requirePlatformAdmin(w, r) {
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.CancelImport(database.AsSystem(ctx), publicID); err != nil {
			h.log.WarnContext(ctx, "could not cancel import", "session", publicID, "error", err)
			h.redirectWithNotice(w, r, "/admin/products/import/"+publicID+querySuffix(r), "error",
				h.importMessage(err, r))
			return
		}
	}
	h.redirectWithNotice(w, r, "/admin/products/import", "success",
		"تم إلغاء عملية الاستيراد ولم يتم حفظ أي صنف.")
}

// readImportSettings reads the strategy, the switches, and the column
// corrections out of a submitted form.
func readImportSettings(r *http.Request) catalog.ImportSettings {
	return catalog.ImportSettings{
		Mode: catalog.ParseMode(r.PostFormValue("import_mode")),
		Options: catalog.ImportOptions{
			AutoCreateBrands:     formChecked(r, "auto_create_brands"),
			AssignCategory:       formChecked(r, "assign_category"),
			AutoCreateCategories: formChecked(r, "auto_create_categories"),
			AssignDosageForm:     formChecked(r, "assign_dosage_form"),
			AssignScientificName: formChecked(r, "assign_scientific_name"),
			UseAI:                formChecked(r, "use_ai"),
			DefaultCategoryID:    formInt64(r, "default_category_id"),
		},
		Overrides: readLayoutOverrides(r),
	}
}

// readLayoutOverrides reads the admin's corrections to the detected structure.
//
// An empty box means "keep what was detected"; an explicit zero means "do not
// read this field at all". The two are different instructions and the form has
// to be able to express both.
func readLayoutOverrides(r *http.Request) catalog.LayoutOverrides {
	overrides := catalog.LayoutOverrides{
		HeaderRow:    int(formInt64(r, "header_row")),
		FirstDataRow: int(formInt64(r, "first_data_row")),
		LastDataRow:  int(formInt64(r, "last_data_row")),
		Columns:      map[string]int{},
	}

	for key, values := range r.PostForm {
		field, isColumn := strings.CutPrefix(key, "col_")
		if !isColumn || len(values) == 0 {
			continue
		}
		raw := strings.TrimSpace(values[0])
		if raw == "" {
			continue // untouched: leave the detection alone
		}
		column, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		if column <= 0 {
			overrides.Columns[field] = catalog.IgnoreColumn
			continue
		}
		overrides.Columns[field] = column
	}

	if len(overrides.Columns) == 0 {
		overrides.Columns = nil
	}
	return overrides
}

func formChecked(r *http.Request, name string) bool {
	value := r.PostFormValue(name)
	return value == "1" || value == "on" || value == "true"
}

func formInt64(r *http.Request, name string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue(name)), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// querySuffix carries the review table's filter and page across a redirect.
func querySuffix(r *http.Request) string {
	if raw := r.URL.RawQuery; raw != "" {
		return "?" + raw
	}
	return ""
}

// actorUserID is the signed-in admin, or zero when the request has no actor.
func actorUserID(ctx context.Context) int64 {
	if actor, ok := authctx.From(ctx); ok {
		return actor.UserID
	}
	return 0
}

// importCategories is the taxonomy offered in the wizard's category chooser.
func (h *UIHandler) importCategories(ctx context.Context) []catalog.TaxonomyOption {
	if h.catSvc == nil {
		return nil
	}
	vocab, err := h.catSvc.ImportVocabulary(database.AsSystem(ctx), 0)
	if err != nil {
		h.log.DebugContext(ctx, "import categories unavailable", "error", err)
		return nil
	}
	return vocab.Categories
}

// recentImportSessions backs the history panel on the upload screen.
func (h *UIHandler) recentImportSessions(ctx context.Context) []*catalog.ImportSession {
	if h.catSvc == nil {
		return nil
	}
	sessions, err := h.catSvc.RecentImportSessions(database.AsSystem(ctx), 0, 8)
	if err != nil {
		h.log.DebugContext(ctx, "import history unavailable", "error", err)
		return nil
	}
	return sessions
}

// importMessage prefers the domain's own Arabic message over a raw error.
func (h *UIHandler) importMessage(err error, r *http.Request) string {
	if msg := h.safeMessage(err, langOf(r)); msg != "" && msg != "حدث خطأ غير متوقع" {
		return msg
	}
	return err.Error()
}

// uploadError carries an admin-facing reason alongside the technical detail,
// which is shown smaller and logged rather than being the headline.
type uploadError struct {
	message string
	detail  string
}

func (e *uploadError) Error() string { return e.message }

// readUploadedFile pulls the spreadsheet out of the multipart request.
//
// The field is accepted under two names because two different screens post here
// — the import wizard sends "import_file" and the older warehouse upload form
// sends "file".
func readUploadedFile(r *http.Request) ([]byte, string, *uploadError) {
	// The body cap is enforced here rather than trusting the multipart parser's
	// memory limit, which bounds what is held in RAM, not what a client may
	// stream. A w of nil only means "cannot flag the connection as too large";
	// the read itself still fails past the cap.
	r.Body = http.MaxBytesReader(nil, r.Body, maxImportRequestBytes)

	if err := r.ParseMultipartForm(maxImportUploadBytes); err != nil {
		return nil, "", &uploadError{
			message: fmt.Sprintf("تعذرت قراءة الملف المرفوع. الحد الأقصى لحجم الملف هو %d ميجابايت.",
				maxImportUploadBytes>>20),
			detail: err.Error(),
		}
	}

	file, header, err := r.FormFile("import_file")
	if err != nil {
		file, header, err = r.FormFile("file")
	}
	if err != nil {
		return nil, "", &uploadError{
			message: "لم يتم اختيار أي ملف. يرجى اختيار ملف Excel (.xlsx) أو CSV ثم الضغط على «تحليل الملف».",
		}
	}
	defer func() { _ = file.Close() }()

	// One byte past the cap, so a file exactly at the limit still reads whole
	// and anything larger is detectable without buffering all of it.
	content, err := io.ReadAll(io.LimitReader(file, maxImportUploadBytes+1))
	if err != nil {
		return nil, "", &uploadError{
			message: "تعذرت قراءة محتوى الملف المرفوع. يرجى إعادة المحاولة.",
			detail:  err.Error(),
		}
	}
	if int64(len(content)) > maxImportUploadBytes {
		return nil, "", &uploadError{
			message: fmt.Sprintf("حجم الملف يتجاوز الحد الأقصى المسموح به (%d ميجابايت).",
				maxImportUploadBytes>>20),
		}
	}

	filename := ""
	if header != nil {
		filename = header.Filename
	}
	return content, filename, nil
}

// refreshProductIndex rebuilds the denormalised search table after an import.
//
// catalog.product_index is what the storefront and the fast search read; it is
// populated from catalog.products and does not update itself. Without this an
// admin imports nine thousand products, sees them in the admin list, and cannot
// find a single one from the customer-facing search.
func (h *UIHandler) refreshProductIndex(ctx context.Context) {
	if h.catSvc == nil {
		return
	}
	// Detached from the request: the admin should not wait on it, and a client
	// disconnect must not abort a rebuild that is already underway.
	go func() {
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		count, err := h.catSvc.RebuildProductIndex(database.AsSystem(bg))
		if err != nil {
			h.log.ErrorContext(bg, "rebuild product index after import", "error", err)
			return
		}
		h.log.InfoContext(bg, "product index rebuilt after import", "rows", count)
	}()
}

// AdminProductsImportProgress reports a running preparation as JSON.
//
// The review screen polls it while a file is being processed. It is a small
// endpoint rather than a stream because the answer changes a few times a second
// at most, and a poll survives a reverse proxy that would buffer an event
// stream into uselessness.
func (h *UIHandler) AdminProductsImportProgress(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")

	if !h.requirePlatformAdmin(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if h.catSvc == nil {
		http.Error(w, `{"phase":"failed","message":"catalog service unavailable"}`,
			http.StatusServiceUnavailable)
		return
	}

	progress, running := h.catSvc.ImportProgress(publicID)
	if !running {
		// No run in flight. The session itself is the durable answer: a staged
		// session is finished, anything else never started here.
		phase := catalog.ImportPhaseDone
		if session, err := h.catSvc.GetImportSession(database.AsSystem(r.Context()), publicID); err != nil ||
			session.Status == catalog.SessionDraft {
			phase = catalog.ImportPhaseFailed
		}
		progress = catalog.ImportProgress{Phase: phase, Message: phase.Label()}
	}

	payload := map[string]any{
		"phase":   string(progress.Phase),
		"message": progress.Message,
		"current": progress.Current,
		"total":   progress.Total,
		"percent": progress.Percent(),
		"done":    progress.Phase.Terminal(),
		"failed":  progress.Phase == catalog.ImportPhaseFailed,
		"elapsed": int(progress.Elapsed().Seconds()),
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.WarnContext(r.Context(), "write import progress", "session", publicID, "error", err)
	}
}
