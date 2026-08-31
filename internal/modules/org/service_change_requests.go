package org

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// SubmitChangeRequest records a new profile change request for administrator review.
func (s *Service) SubmitChangeRequest(ctx context.Context, orgID int64, requestedBy int64, proposed ProfileValues) (*OrganizationChangeRequest, error) {
	if orgID <= 0 {
		return nil, apperr.Validation("org_id.invalid", "معرف المنشأة غير صالح", nil)
	}

	// Snapshot current values
	var current ProfileValues
	if prof, err := s.repo.GetSupplierProfile(ctx, orgID); err == nil && prof != nil {
		current = ProfileValues{
			NameAr:             prof.NameAr,
			NameEn:             prof.NameEn,
			Type:               prof.Type,
			MinOrderPrice:      prof.MinOrderPrice,
			MaxOrderPrice:      prof.MaxOrderPrice,
			OrganizationNumber: prof.OrganizationNumber,
			Email:              prof.Email,
			Phone:              prof.Phone,
			TaxNumber:          prof.TaxNumber,
			Address:            prof.Address,
			DescriptionAr:      prof.DescriptionAr,
			DescriptionEn:      prof.DescriptionEn,
			Image:              prof.Image,
			CoverageImage:      prof.CoverageImage,
		}
	} else if o, err := s.repo.GetOrganizationByID(ctx, orgID); err == nil && o != nil {
		current = ProfileValues{
			NameAr:             o.TradeName.Get("ar"),
			NameEn:             o.TradeName.Get("en"),
			Type:               string(o.Type),
			MinOrderPrice:      o.MinOrderPrice,
			MaxOrderPrice:      o.MaxOrderPrice,
			OrganizationNumber: o.OrganizationNumber,
			TaxNumber:          o.TaxNumber,
		}
	}

	req := &OrganizationChangeRequest{
		OrganizationID: orgID,
		RequestedBy:    &requestedBy,
		Status:         ChangeRequestPending,
		CurrentValues:  current,
		ProposedValues: proposed,
	}

	if err := s.repo.CreateChangeRequest(ctx, req); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "organization change request submitted", "request_id", req.ID, "org_id", orgID, "requested_by", requestedBy)
	return req, nil
}

// GetChangeRequest fetches a change request by ID.
func (s *Service) GetChangeRequest(ctx context.Context, id int64) (*OrganizationChangeRequest, error) {
	return s.repo.GetChangeRequestByID(ctx, id)
}

// GetPendingChangeRequest returns the active pending change request for an organization, if any.
func (s *Service) GetPendingChangeRequest(ctx context.Context, orgID int64) (*OrganizationChangeRequest, error) {
	return s.repo.GetPendingChangeRequestByOrg(ctx, orgID)
}

// ListChangeRequests returns paginated change requests matching the criteria.
func (s *Service) ListChangeRequests(ctx context.Context, orgID *int64, status *ChangeRequestStatus, limit, offset int) ([]*OrganizationChangeRequest, error) {
	return s.repo.ListChangeRequests(ctx, orgID, status, limit, offset)
}

// CountChangeRequests counts total change requests for the specified filters.
func (s *Service) CountChangeRequests(ctx context.Context, orgID *int64, status *ChangeRequestStatus) (int, error) {
	return s.repo.CountChangeRequests(ctx, orgID, status)
}

// ApproveChangeRequest approves the change request and updates the organization profile.
func (s *Service) ApproveChangeRequest(ctx context.Context, reqID int64, adminID int64, adminNotes string) error {
	if reqID <= 0 {
		return apperr.Validation("request_id.invalid", "معرف الطلب غير صالح", nil)
	}
	if err := s.repo.ApproveChangeRequest(ctx, reqID, adminID, adminNotes); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization change request approved", "request_id", reqID, "admin_id", adminID)
	return nil
}

// RejectChangeRequest rejects the change request with a reason.
func (s *Service) RejectChangeRequest(ctx context.Context, reqID int64, adminID int64, rejectionReason string) error {
	if reqID <= 0 {
		return apperr.Validation("request_id.invalid", "معرف الطلب غير صالح", nil)
	}
	if rejectionReason == "" {
		return apperr.Validation("rejection_reason.required", "يرجى توضيح سبب رفض طلب التعديل.", nil)
	}
	if err := s.repo.RejectChangeRequest(ctx, reqID, adminID, rejectionReason); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization change request rejected", "request_id", reqID, "admin_id", adminID, "reason", rejectionReason)
	return nil
}
