package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateChangeRequest stores a new organization change request in the database.
func (r *Repository) CreateChangeRequest(ctx context.Context, req *org.OrganizationChangeRequest) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Cancel any existing pending change requests for this organization
		_, err := tx.Exec(txCtx, `
			UPDATE org.organization_change_requests
			SET status = 'cancelled', updated_at = now()
			WHERE organization_id = $1 AND status = 'pending';
		`, req.OrganizationID)
		if err != nil {
			return err
		}

		curBytes, err := json.Marshal(req.CurrentValues)
		if err != nil {
			return err
		}
		propBytes, err := json.Marshal(req.ProposedValues)
		if err != nil {
			return err
		}

		query := `
			INSERT INTO org.organization_change_requests (
				organization_id, requested_by, status, current_values, proposed_values
			) VALUES (
				$1, $2, 'pending', $3::jsonb, $4::jsonb
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			req.OrganizationID, req.RequestedBy, curBytes, propBytes,
		).Scan(&req.ID, &req.PublicID, &req.CreatedAt, &req.UpdatedAt)
	})
}

// GetChangeRequestByID retrieves a single change request by its ID.
func (r *Repository) GetChangeRequestByID(ctx context.Context, id int64) (*org.OrganizationChangeRequest, error) {
	var item org.OrganizationChangeRequest
	var curBytes, propBytes []byte
	var statusStr string

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 
				ocr.id, ocr.public_id, ocr.organization_id,
				COALESCE(o.legal_name, o.name->>'ar', o.name->>'en', ''),
				COALESCE(o.type, 'vendor'),
				ocr.requested_by,
				COALESCE(u.name->>'ar', u.name->>'en', u.email, ''),
				COALESCE(u.email, ''),
				ocr.status, ocr.current_values, ocr.proposed_values,
				ocr.reviewed_by,
				COALESCE(rev.name->>'ar', rev.name->>'en', rev.email, ''),
				ocr.reviewed_at,
				COALESCE(ocr.admin_notes, ''),
				COALESCE(ocr.rejection_reason, ''),
				ocr.created_at, ocr.updated_at
			FROM org.organization_change_requests ocr
			JOIN org.organizations o ON o.id = ocr.organization_id
			LEFT JOIN identity.users u ON u.id = ocr.requested_by
			LEFT JOIN identity.users rev ON rev.id = ocr.reviewed_by
			WHERE ocr.id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&item.ID, &item.PublicID, &item.OrganizationID,
			&item.OrgName, &item.OrgType,
			&item.RequestedBy, &item.RequesterName, &item.RequesterEmail,
			&statusStr, &curBytes, &propBytes,
			&item.ReviewedBy, &item.ReviewerName, &item.ReviewedAt,
			&item.AdminNotes, &item.RejectionReason,
			&item.CreatedAt, &item.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("change_request")
		}
		return nil, err
	}

	item.Status = org.ChangeRequestStatus(statusStr)
	_ = json.Unmarshal(curBytes, &item.CurrentValues)
	_ = json.Unmarshal(propBytes, &item.ProposedValues)
	return &item, nil
}

// GetPendingChangeRequestByOrg finds the current pending request for an organization, if any.
func (r *Repository) GetPendingChangeRequestByOrg(ctx context.Context, orgID int64) (*org.OrganizationChangeRequest, error) {
	var item org.OrganizationChangeRequest
	var curBytes, propBytes []byte
	var statusStr string

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 
				ocr.id, ocr.public_id, ocr.organization_id,
				COALESCE(o.legal_name, o.name->>'ar', o.name->>'en', ''),
				COALESCE(o.type, 'vendor'),
				ocr.requested_by,
				COALESCE(u.name->>'ar', u.name->>'en', u.email, ''),
				COALESCE(u.email, ''),
				ocr.status, ocr.current_values, ocr.proposed_values,
				ocr.reviewed_by,
				COALESCE(rev.name->>'ar', rev.name->>'en', rev.email, ''),
				ocr.reviewed_at,
				COALESCE(ocr.admin_notes, ''),
				COALESCE(ocr.rejection_reason, ''),
				ocr.created_at, ocr.updated_at
			FROM org.organization_change_requests ocr
			JOIN org.organizations o ON o.id = ocr.organization_id
			LEFT JOIN identity.users u ON u.id = ocr.requested_by
			LEFT JOIN identity.users rev ON rev.id = ocr.reviewed_by
			WHERE ocr.organization_id = $1 AND ocr.status = 'pending'
			ORDER BY ocr.id DESC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, orgID).Scan(
			&item.ID, &item.PublicID, &item.OrganizationID,
			&item.OrgName, &item.OrgType,
			&item.RequestedBy, &item.RequesterName, &item.RequesterEmail,
			&statusStr, &curBytes, &propBytes,
			&item.ReviewedBy, &item.ReviewerName, &item.ReviewedAt,
			&item.AdminNotes, &item.RejectionReason,
			&item.CreatedAt, &item.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	item.Status = org.ChangeRequestStatus(statusStr)
	_ = json.Unmarshal(curBytes, &item.CurrentValues)
	_ = json.Unmarshal(propBytes, &item.ProposedValues)
	return &item, nil
}

// ListChangeRequests returns paginated change requests matching the criteria.
func (r *Repository) ListChangeRequests(ctx context.Context, orgID *int64, status *org.ChangeRequestStatus, limit, offset int) ([]*org.OrganizationChangeRequest, error) {
	var list []*org.OrganizationChangeRequest
	if limit <= 0 {
		limit = 50
	}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 
				ocr.id, ocr.public_id, ocr.organization_id,
				COALESCE(o.legal_name, o.name->>'ar', o.name->>'en', ''),
				COALESCE(o.type, 'vendor'),
				ocr.requested_by,
				COALESCE(u.name->>'ar', u.name->>'en', u.email, ''),
				COALESCE(u.email, ''),
				ocr.status, ocr.current_values, ocr.proposed_values,
				ocr.reviewed_by,
				COALESCE(rev.name->>'ar', rev.name->>'en', rev.email, ''),
				ocr.reviewed_at,
				COALESCE(ocr.admin_notes, ''),
				COALESCE(ocr.rejection_reason, ''),
				ocr.created_at, ocr.updated_at
			FROM org.organization_change_requests ocr
			JOIN org.organizations o ON o.id = ocr.organization_id
			LEFT JOIN identity.users u ON u.id = ocr.requested_by
			LEFT JOIN identity.users rev ON rev.id = ocr.reviewed_by
			WHERE ($1::bigint IS NULL OR ocr.organization_id = $1)
			  AND ($2::text IS NULL OR ocr.status = $2)
			ORDER BY ocr.id DESC
			LIMIT $3 OFFSET $4;
		`
		var statusStr *string
		if status != nil {
			s := string(*status)
			statusStr = &s
		}

		rows, err := tx.Query(txCtx, query, orgID, statusStr, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item org.OrganizationChangeRequest
			var curBytes, propBytes []byte
			var st string

			if err := rows.Scan(
				&item.ID, &item.PublicID, &item.OrganizationID,
				&item.OrgName, &item.OrgType,
				&item.RequestedBy, &item.RequesterName, &item.RequesterEmail,
				&st, &curBytes, &propBytes,
				&item.ReviewedBy, &item.ReviewerName, &item.ReviewedAt,
				&item.AdminNotes, &item.RejectionReason,
				&item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}

			item.Status = org.ChangeRequestStatus(st)
			_ = json.Unmarshal(curBytes, &item.CurrentValues)
			_ = json.Unmarshal(propBytes, &item.ProposedValues)
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

// CountChangeRequests counts total records matching filters.
func (r *Repository) CountChangeRequests(ctx context.Context, orgID *int64, status *org.ChangeRequestStatus) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(*)
			FROM org.organization_change_requests
			WHERE ($1::bigint IS NULL OR organization_id = $1)
			  AND ($2::text IS NULL OR status = $2);
		`
		var statusStr *string
		if status != nil {
			s := string(*status)
			statusStr = &s
		}
		return tx.QueryRow(txCtx, query, orgID, statusStr).Scan(&total)
	})
	return total, err
}

// ApproveChangeRequest applies the proposed changes to the organization and marks the request approved.
func (r *Repository) ApproveChangeRequest(ctx context.Context, reqID int64, adminID int64, adminNotes string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var orgID int64
		var propBytes []byte
		var currentStatus string

		queryReq := `
			SELECT organization_id, proposed_values, status
			FROM org.organization_change_requests
			WHERE id = $1
			FOR UPDATE;
		`
		if err := tx.QueryRow(txCtx, queryReq, reqID).Scan(&orgID, &propBytes, &currentStatus); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("change_request")
			}
			return err
		}

		if currentStatus != string(org.ChangeRequestPending) {
			return apperr.Conflict("change_request.already_decided", "هذا الطلب تمت مراجعته مسبقاً.")
		}

		var prop org.ProfileValues
		if err := json.Unmarshal(propBytes, &prop); err != nil {
			return err
		}

		// Update org.organizations
		updateOrgQuery := `
			UPDATE org.organizations
			SET 
				name = jsonb_build_object('ar', $1::text, 'en', $2::text),
				type = $3,
				min_order_price = $4,
				max_order_price = $5,
				organization_number = $6,
				email = $7,
				phone = $8,
				tax_number = $9,
				address = $10,
				description = jsonb_build_object('ar', $11::text, 'en', $12::text),
				image = CASE WHEN $13::text <> '' THEN $13::text ELSE image END,
				coverage_image = CASE WHEN $14::text <> '' THEN $14::text ELSE coverage_image END,
				updated_at = now()
			WHERE id = $15 AND deleted_at IS NULL;
		`
		tag, err := tx.Exec(txCtx, updateOrgQuery,
			prop.NameAr, prop.NameEn, prop.Type,
			prop.MinOrderPrice, prop.MaxOrderPrice,
			prop.OrganizationNumber, prop.Email, prop.Phone, prop.TaxNumber, prop.Address,
			prop.DescriptionAr, prop.DescriptionEn,
			prop.Image, prop.CoverageImage,
			orgID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}

		// Mark request as approved
		updateReqQuery := `
			UPDATE org.organization_change_requests
			SET status = 'approved',
			    reviewed_by = $2,
			    reviewed_at = now(),
			    admin_notes = $3,
			    updated_at = now()
			WHERE id = $1;
		`
		_, err = tx.Exec(txCtx, updateReqQuery, reqID, adminID, adminNotes)
		return err
	})
}

// RejectChangeRequest marks a change request as rejected with a mandatory reason.
func (r *Repository) RejectChangeRequest(ctx context.Context, reqID int64, adminID int64, rejectionReason string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.organization_change_requests
			SET status = 'rejected',
			    reviewed_by = $2,
			    reviewed_at = now(),
			    rejection_reason = $3,
			    updated_at = now()
			WHERE id = $1 AND status = 'pending';
		`
		tag, err := tx.Exec(txCtx, query, reqID, adminID, rejectionReason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("change_request")
		}
		return nil
	})
}
