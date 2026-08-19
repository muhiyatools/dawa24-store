package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateAutomationRequest writes a new automatic purchase optimization task.
func (r *Repository) CreateAutomationRequest(ctx context.Context, req *workflow.AutomationRequest) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		prioritiesJSON, _ := json.Marshal(req.Priorities)
		fileDataJSON, _ := json.Marshal(req.FileData)
		matchesJSON, _ := json.Marshal(req.VendorMatches)
		resultsJSON, _ := json.Marshal(req.ComparisonResults)

		var budgetVal *string
		if req.BudgetConstraint != nil && req.BudgetConstraint.IsPositive() {
			s := req.BudgetConstraint.String()
			budgetVal = &s
		}

		query := `
			INSERT INTO workflow.automation_requests (
				user_id, organization_id, request_number, original_filename, file_path,
				status, total_products, matched_products, approved_products, total_value,
				priorities, budget_constraint, file_data, vendor_matches, comparison_results,
				notes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			req.UserID, req.OrganizationID, req.RequestNumber, req.OriginalFilename, req.FilePath,
			req.Status, req.TotalProducts, req.MatchedProducts, req.ApprovedProducts, req.TotalValue.String(),
			prioritiesJSON, budgetVal, fileDataJSON, matchesJSON, resultsJSON,
			req.Notes,
		).Scan(&req.ID, &req.PublicID, &req.CreatedAt, &req.UpdatedAt)
	})
}

// GetAutomationRequestByID retrieves an automation request by ID.
func (r *Repository) GetAutomationRequestByID(ctx context.Context, id int64) (*workflow.AutomationRequest, error) {
	var req workflow.AutomationRequest
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, request_number, original_filename,
			       file_path, status, total_products, matched_products, approved_products,
			       total_value, priorities, budget_constraint, file_data, vendor_matches,
			       comparison_results, notes, processed_at, approved_at, approved_by,
			       created_at, updated_at
			FROM workflow.automation_requests
			WHERE id = $1;
		`
		var totalValStr string
		var budgetValStr *string
		var prioritiesJSON, fileDataJSON, matchesJSON, resultsJSON []byte

		err := tx.QueryRow(txCtx, query, id).Scan(
			&req.ID, &req.PublicID, &req.UserID, &req.OrganizationID, &req.RequestNumber, &req.OriginalFilename,
			&req.FilePath, &req.Status, &req.TotalProducts, &req.MatchedProducts, &req.ApprovedProducts,
			&totalValStr, &prioritiesJSON, &budgetValStr, &fileDataJSON, &matchesJSON,
			&resultsJSON, &req.Notes, &req.ProcessedAt, &req.ApprovedAt, &req.ApprovedBy,
			&req.CreatedAt, &req.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("automation_request")
			}
			return err
		}

		req.TotalValue, _ = money.Parse(totalValStr)
		if budgetValStr != nil {
			b, _ := money.Parse(*budgetValStr)
			req.BudgetConstraint = &b
		}
		_ = json.Unmarshal(prioritiesJSON, &req.Priorities)
		_ = json.Unmarshal(fileDataJSON, &req.FileData)
		_ = json.Unmarshal(matchesJSON, &req.VendorMatches)
		_ = json.Unmarshal(resultsJSON, &req.ComparisonResults)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &req, nil
}

// ListAutomationRequestsByUser lists past automation requests for a user.
func (r *Repository) ListAutomationRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*workflow.AutomationRequest, error) {
	var list []*workflow.AutomationRequest

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, request_number, original_filename,
			       file_path, status, total_products, matched_products, approved_products,
			       total_value, priorities, budget_constraint, comparison_results, notes,
			       processed_at, approved_at, approved_by, created_at, updated_at
			FROM workflow.automation_requests
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, userID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var req workflow.AutomationRequest
			var totalValStr string
			var budgetValStr *string
			var prioritiesJSON, resultsJSON []byte

			err := rows.Scan(
				&req.ID, &req.PublicID, &req.UserID, &req.OrganizationID, &req.RequestNumber, &req.OriginalFilename,
				&req.FilePath, &req.Status, &req.TotalProducts, &req.MatchedProducts, &req.ApprovedProducts,
				&totalValStr, &prioritiesJSON, &budgetValStr, &resultsJSON, &req.Notes,
				&req.ProcessedAt, &req.ApprovedAt, &req.ApprovedBy, &req.CreatedAt, &req.UpdatedAt,
			)
			if err != nil {
				return err
			}
			req.TotalValue, _ = money.Parse(totalValStr)
			if budgetValStr != nil {
				b, _ := money.Parse(*budgetValStr)
				req.BudgetConstraint = &b
			}
			_ = json.Unmarshal(prioritiesJSON, &req.Priorities)
			_ = json.Unmarshal(resultsJSON, &req.ComparisonResults)
			list = append(list, &req)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list automation requests: %w", err)
	}
	return list, nil
}

// UpdateAutomationRequestStatus updates status and analysis results.
func (r *Repository) UpdateAutomationRequestStatus(ctx context.Context, id int64, status workflow.AutomationRequestStatus, results map[string]any, totalVal *money.Amount, matchedCount, approvedCount int) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()
		resultsJSON, _ := json.Marshal(results)

		var totalValStr *string
		if totalVal != nil {
			s := totalVal.String()
			totalValStr = &s
		}

		query := `
			UPDATE workflow.automation_requests
			SET status = $2, comparison_results = $3,
			    total_value = COALESCE($4::NUMERIC, total_value),
			    matched_products = $5, approved_products = $6,
			    processed_at = $7, updated_at = $7
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query, id, status, resultsJSON, totalValStr, matchedCount, approvedCount, now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("automation_request")
		}
		return nil
	})
}
