package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) AdminApprovalsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)
	lang, dir := h.localeAndDir(r)

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "organizations"
	}
	statusParam := r.URL.Query().Get("status")

	data := &pages.AdminApprovalsData{
		ActiveTab:    tab,
		StatusFilter: statusParam,
		OrgDocs:      make(map[int64][]*attachments.Document),
		OrgNames:     make(map[int64]string),
	}

	sysCtx := database.AsSystem(ctx)

	if h.orgSvc != nil {
		allList, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0)
		data.AllOrganizations = allList
		for _, o := range allList {
			if o != nil {
				data.OrgNames[o.ID] = o.LegalName
			}
		}

		var filterStatus *org.OrganizationStatus
		if statusParam != "" {
			st := org.OrganizationStatus(statusParam)
			filterStatus = &st
		} else if tab == "organizations" {
			st := org.StatusPending
			filterStatus = &st
		}
		list, err := h.orgSvc.ListOrganizations(sysCtx, nil, filterStatus, 150, 0)
		if err != nil {
			h.log.WarnContext(ctx, "admin approvals: list organizations", "error", err)
		} else {
			data.Organizations = list
		}
	}

	if h.attSvc != nil {
		for _, o := range data.Organizations {
			if o != nil {
				docs, _ := h.attSvc.ListByOrganization(sysCtx, o.ID)
				if len(docs) > 0 {
					data.OrgDocs[o.ID] = docs
				}
			}
		}

		docs, _, err := h.attSvc.ListAll(sysCtx, attachments.DocumentFilter{Limit: 200})
		if err == nil {
			data.UploadedDocs = docs
		}

		reqs, err := h.attSvc.ListDocumentRequests(sysCtx, actor, nil)
		if err == nil {
			data.DocRequests = reqs
		}
	}

	h.renderPage(ctx, w, "render admin approvals page", pages.AdminApprovals(data, lang, dir))
}

// AdminOrgReviewSubmit handles full administrative approval/rejection with custom reason and document categorization.
func (h *UIHandler) AdminOrgReviewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals", "error", i18n.T(lang, "admin.approvals.invalid_org_id"))
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/approvals", "error", i18n.T(lang, "admin.approvals.org_service_unavailable"))
		return
	}

	status := org.OrganizationStatus(r.PostFormValue("status"))
	notes := r.PostFormValue("verification_notes")
	rejectionReason := r.PostFormValue("rejection_reason")
	docTypeVal := attachments.DocumentType(strings.TrimSpace(r.PostFormValue("document_type")))

	if err := h.orgSvc.ReviewOrganization(ctx, id, status, notes, rejectionReason, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, "/admin/approvals", "error", h.safeMessage(err, lang))
		return
	}

	// When approved, classify and verify the organization's registration documents
	if status == org.StatusApproved && h.attSvc != nil {
		sysCtx := database.AsSystem(ctx)
		o, _ := h.orgSvc.GetOrganization(sysCtx, id)
		if docTypeVal == "" {
			if o != nil && o.Type == org.TypeCustomer {
				docTypeVal = attachments.DocPharmacyLicense
			} else {
				docTypeVal = attachments.DocCommercialRegister
			}
		}

		docs, _ := h.attSvc.ListByOrganization(sysCtx, id)
		for _, d := range docs {
			if d != nil {
				_ = h.attSvc.VerifyDocumentWithType(sysCtx, actor, d.ID, docTypeVal, attachments.StatusVerified, notes)
			}
		}
		go h.provisionOrgAIAndSubscription(context.Background(), id)
	}

	msg := i18n.T(lang, "admin.approvals.approved_and_verified_success")
	if status == org.StatusRejected {
		msg = i18n.T(lang, "admin.approvals.rejected_success")
	} else if status == org.StatusSuspended {
		msg = i18n.T(lang, "admin.approvals.suspended_notice")
	}

	h.redirectWithNotice(w, r, "/admin/approvals", "success", msg)
}

// Platform settings keys. These live in platform_admin.system_settings.
const (
	settingSupportEmail   = "platform.support_email"
	settingCommissionRate = "platform.commission_rate"
)

// Organization approval actions.
//
// The approvals page posted straight to the JSON API and swapped the response
// into the table row, so a successful approval replaced the row with the text
// {"status":"approved"}. These do the work and return the refreshed table.

func (h *UIHandler) adminApprovalAction(
	w http.ResponseWriter,
	r *http.Request,
	action func(ctx context.Context, orgID int64) error,
) {
	ctx := r.Context()

	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/approvals", http.StatusSeeOther)
		return
	}
	if h.orgSvc == nil {
		h.renderError(w, r, apperr.Unavailable("org", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}

	if err := action(ctx, id); err != nil {
		h.renderError(w, r, err)
		return
	}

	if !h.isHTMX(r) {
		h.redirectWithNotice(w, r, "/admin/approvals", "success", i18n.T(langOf(r), "admin.approvals.status_updated_success"))
		return
	}

	pendingStatus := org.StatusPending
	pending, err := h.orgSvc.ListOrganizations(ctx, nil, &pendingStatus, 50, 0)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	orgDocs := make(map[int64][]*attachments.Document)
	if h.attSvc != nil && len(pending) > 0 {
		sysCtx := database.AsSystem(ctx)
		for _, o := range pending {
			if o != nil {
				docs, _ := h.attSvc.ListByOrganization(sysCtx, o.ID)
				if len(docs) > 0 {
					orgDocs[o.ID] = docs
				}
			}
		}
	}

	h.renderPage(ctx, w, "render approvals table after action", pages.AdminApprovalsTable(pending, orgDocs))
}

// AdminApproveOrgSubmit approves a pending organization.
func (h *UIHandler) AdminApproveOrgSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminApprovalAction(w, r, func(ctx context.Context, orgID int64) error {
		if err := h.orgSvc.ApproveOrganization(ctx, orgID); err != nil {
			return err
		}
		go h.provisionOrgAIAndSubscription(context.Background(), orgID)
		return nil
	})
}

// AdminRejectOrgSubmit rejects a pending organization.
func (h *UIHandler) AdminRejectOrgSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminApprovalAction(w, r, func(ctx context.Context, orgID int64) error {
		return h.orgSvc.RejectOrganization(ctx, orgID)
	})
}

// AdminOrgApproveSubmit approves an organization.
func (h *UIHandler) AdminOrgApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.ApproveOrganization(ctx, id)
		go h.provisionOrgAIAndSubscription(context.Background(), id)
	}
	h.redirectWithNotice(w, r, "/admin/organizations", "success", i18n.T(langOf(r), "admin.approvals.account_activated_success"))
}

// provisionOrgAIAndSubscription gives a newly approved منشأة both of the things
// it needs before anyone in it can use AI: a subscription, and the Gateway
// identity its employees will spend against.
//
// Approval is the right moment for this. Until now the only trigger was a
// dashboard render, so an organisation that was approved and never visited its
// own dashboard had no Gateway account at all — and the first AI call anyone in
// it made was billed to the platform's key instead.
func (h *UIHandler) provisionOrgAIAndSubscription(ctx context.Context, orgID int64) {
	if orgID <= 0 || h.orgSvc == nil {
		return
	}
	sysCtx := database.AsSystem(ctx)

	// 1. Ensure company starter roles & RBAC permissions are populated
	if o, err := h.orgSvc.GetOrganization(sysCtx, orgID); err == nil && o != nil {
		h.ensureCompanyRoles(sysCtx, orgID, string(o.Type))
	}

	// 2. The subscription comes first: it is what decides which Gateway plan — and
	// therefore which quota — the identity below is created under.
	if h.billSvc != nil {
		if _, err := h.billSvc.AssignDefaultSubscription(sysCtx, 0, &orgID); err != nil {
			h.log.WarnContext(ctx, "could not assign default subscription on approval",
				"org_id", orgID, "error", err)
		}
	}

	// 3. AI Gateway key
	if h.tenantKeys != nil {
		if key := h.tenantKeys.Key(sysCtx, orgID); key == "" {
			h.log.WarnContext(ctx, "organisation approved without a gateway identity",
				"org_id", orgID)
		}
	}

	// 4. Invalidate resolver cached grants so active sessions immediately see approval
	if h.resolver != nil {
		h.resolver.InvalidateAll()
	}
}

// AdminOrgRejectSubmit rejects an organization.
func (h *UIHandler) AdminOrgRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.RejectOrganization(ctx, id)
	}
	http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
}

// AdminOrgSuspendSubmit suspends an organization.
func (h *UIHandler) AdminOrgSuspendSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.SuspendOrganization(ctx, id)
	}
	http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
}
