package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateInvoice persists a B2B invoice and its line items.
func (r *Repository) CreateInvoice(ctx context.Context, inv *billing.Invoice) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO billing.invoices (
				organization_id, customer_org_id, order_id, invoice_number,
				issue_date, due_date, subtotal, tax_amount, discount_amount,
				total_amount, status, payment_method, notes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			RETURNING id, public_id, created_at, updated_at;
		`
		if inv.IssueDate.IsZero() {
			inv.IssueDate = time.Now().UTC()
		}
		if inv.DueDate.IsZero() {
			inv.DueDate = inv.IssueDate.AddDate(0, 0, 30)
		}

		err := tx.QueryRow(txCtx, query,
			inv.OrganizationID, inv.CustomerOrgID, inv.OrderID, inv.InvoiceNumber,
			inv.IssueDate, inv.DueDate, inv.Subtotal, inv.TaxAmount, inv.DiscountAmount,
			inv.TotalAmount, string(inv.Status), inv.PaymentMethod, inv.Notes,
		).Scan(&inv.ID, &inv.PublicID, &inv.CreatedAt, &inv.UpdatedAt)
		if err != nil {
			return err
		}

		for i := range inv.Lines {
			line := &inv.Lines[i]
			lineQuery := `
				INSERT INTO billing.invoice_lines (invoice_id, product_id, description, quantity, unit_price, total_price)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id;
			`
			if err := tx.QueryRow(txCtx, lineQuery,
				inv.ID, line.ProductID, line.Description, line.Quantity, line.UnitPrice, line.TotalPrice,
			).Scan(&line.ID); err != nil {
				return err
			}
			line.InvoiceID = inv.ID
		}
		return nil
	})
}

// GetInvoiceByID retrieves an invoice and its lines.
func (r *Repository) GetInvoiceByID(ctx context.Context, id int64) (*billing.Invoice, error) {
	var inv billing.Invoice
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, customer_org_id, order_id, invoice_number,
			       issue_date, due_date, subtotal, tax_amount, discount_amount, total_amount,
			       status, payment_method, notes, created_at, updated_at
			FROM billing.invoices WHERE id = $1;
		`
		var statusStr string
		var notes *string
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&inv.ID, &inv.PublicID, &inv.OrganizationID, &inv.CustomerOrgID, &inv.OrderID, &inv.InvoiceNumber,
			&inv.IssueDate, &inv.DueDate, &inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount,
			&statusStr, &inv.PaymentMethod, &notes, &inv.CreatedAt, &inv.UpdatedAt,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("invoice")
			}
			return err
		}
		inv.Status = billing.InvoiceStatus(statusStr)
		if notes != nil {
			inv.Notes = *notes
		}

		linesQuery := `SELECT id, invoice_id, product_id, description, quantity, unit_price, total_price FROM billing.invoice_lines WHERE invoice_id = $1;`
		rows, err := tx.Query(txCtx, linesQuery, id)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var line billing.InvoiceLine
			if err := rows.Scan(&line.ID, &line.InvoiceID, &line.ProductID, &line.Description, &line.Quantity, &line.UnitPrice, &line.TotalPrice); err != nil {
				return err
			}
			inv.Lines = append(inv.Lines, line)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// UpdateInvoiceStatus changes invoice status.
func (r *Repository) UpdateInvoiceStatus(ctx context.Context, id int64, status billing.InvoiceStatus) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE billing.invoices SET status = $1, updated_at = now() WHERE id = $2;`
		tag, err := tx.Exec(txCtx, query, string(status), id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("invoice")
		}
		return nil
	})
}

// ListInvoicesByOrg returns invoices for an organization.
func (r *Repository) ListInvoicesByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, error) {
	var list []*billing.Invoice
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, customer_org_id, order_id, invoice_number,
			       issue_date, due_date, subtotal, tax_amount, discount_amount, total_amount,
			       status, payment_method, notes, created_at, updated_at
			FROM billing.invoices
			WHERE organization_id = $1 OR customer_org_id = $1
			ORDER BY issue_date DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var inv billing.Invoice
			var statusStr string
			var notes *string
			if err := rows.Scan(
				&inv.ID, &inv.PublicID, &inv.OrganizationID, &inv.CustomerOrgID, &inv.OrderID, &inv.InvoiceNumber,
				&inv.IssueDate, &inv.DueDate, &inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount,
				&statusStr, &inv.PaymentMethod, &notes, &inv.CreatedAt, &inv.UpdatedAt,
			); err != nil {
				return err
			}
			inv.Status = billing.InvoiceStatus(statusStr)
			if notes != nil {
				inv.Notes = *notes
			}
			list = append(list, &inv)
		}
		return rows.Err()
	})
	return list, err
}

// AddPaymentMethod stores a user payment method.
func (r *Repository) AddPaymentMethod(ctx context.Context, pm *billing.UserPaymentMethod) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO billing.user_payment_methods (user_id, provider, account_identifier, is_default)
			VALUES ($1, $2, $3, $4)
			RETURNING id, public_id, created_at;
		`
		return tx.QueryRow(txCtx, query, pm.UserID, pm.Provider, pm.AccountIdentifier, pm.IsDefault).
			Scan(&pm.ID, &pm.PublicID, &pm.CreatedAt)
	})
}

// ListPaymentMethods returns saved payment methods for a user.
func (r *Repository) ListPaymentMethods(ctx context.Context, userID int64) ([]*billing.UserPaymentMethod, error) {
	var list []*billing.UserPaymentMethod
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, user_id, provider, account_identifier, is_default, created_at FROM billing.user_payment_methods WHERE user_id = $1 ORDER BY is_default DESC, id DESC;`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pm billing.UserPaymentMethod
			if err := rows.Scan(&pm.ID, &pm.PublicID, &pm.UserID, &pm.Provider, &pm.AccountIdentifier, &pm.IsDefault, &pm.CreatedAt); err != nil {
				return err
			}
			list = append(list, &pm)
		}
		return rows.Err()
	})
	return list, err
}

// DeletePaymentMethod removes a payment method belonging to the given user.
//
// userID is part of the predicate, not just a lookup argument. The previous
// signature took the id alone and deleted on it, and billing.user_payment_methods
// carries no row-level security - so once any handler reached this, one user
// could delete another's saved card by id. Requiring the owner here means a
// caller cannot express the unsafe version.
func (r *Repository) DeletePaymentMethod(ctx context.Context, userID, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM billing.user_payment_methods WHERE id = $1 AND user_id = $2;`
		tag, err := tx.Exec(txCtx, query, id, userID)
		if err != nil {
			return err
		}
		// A row that exists but belongs to someone else is reported as absent
		// rather than forbidden, so the endpoint cannot be used to probe which
		// payment-method ids are real.
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("payment_method")
		}
		return nil
	})
}
