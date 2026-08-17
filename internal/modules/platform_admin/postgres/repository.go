package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements platformadmin.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a platform admin PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// GetSetting retrieves a setting by key.
func (r *Repository) GetSetting(ctx context.Context, key string) (*platformadmin.SystemSetting, error) {
	var s platformadmin.SystemSetting
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT key, value, description, is_public, updated_at FROM platform_admin.system_settings WHERE key = $1;`
		var valJSON []byte
		var desc *string
		err := tx.QueryRow(txCtx, query, key).Scan(&s.Key, &valJSON, &desc, &s.IsPublic, &s.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("system_setting")
			}
			return err
		}
		if desc != nil {
			s.Description = *desc
		}
		return json.Unmarshal(valJSON, &s.Value)
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// SetSetting writes or updates a system setting.
func (r *Repository) SetSetting(ctx context.Context, s *platformadmin.SystemSetting) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		valJSON, err := json.Marshal(s.Value)
		if err != nil {
			return err
		}
		query := `
			INSERT INTO platform_admin.system_settings (key, value, description, is_public, updated_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (key) DO UPDATE SET
				value = EXCLUDED.value,
				description = EXCLUDED.description,
				is_public = EXCLUDED.is_public,
				updated_at = now();
		`
		_, err = tx.Exec(txCtx, query, s.Key, valJSON, s.Description, s.IsPublic)
		return err
	})
}

// ListPublicSettings returns all public settings.
func (r *Repository) ListPublicSettings(ctx context.Context) ([]*platformadmin.SystemSetting, error) {
	var list []*platformadmin.SystemSetting
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT key, value, description, is_public, updated_at FROM platform_admin.system_settings WHERE is_public = true;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s platformadmin.SystemSetting
			var valJSON []byte
			var desc *string
			if err := rows.Scan(&s.Key, &valJSON, &desc, &s.IsPublic, &s.UpdatedAt); err != nil {
				return err
			}
			if desc != nil {
				s.Description = *desc
			}
			_ = json.Unmarshal(valJSON, &s.Value)
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// ListCountries returns supported countries.
func (r *Repository) ListCountries(ctx context.Context) ([]*platformadmin.Country, error) {
	var list []*platformadmin.Country
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, code, name, phone_code, currency, is_active FROM platform_admin.countries WHERE is_active = true ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c platformadmin.Country
			if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.PhoneCode, &c.Currency, &c.IsActive); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// ListCities returns cities for a country.
func (r *Repository) ListCities(ctx context.Context, countryID int64) ([]*platformadmin.City, error) {
	var list []*platformadmin.City
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, country_id, name, is_active FROM platform_admin.cities WHERE country_id = $1 AND is_active = true ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query, countryID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c platformadmin.City
			if err := rows.Scan(&c.ID, &c.CountryID, &c.Name, &c.IsActive); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// ListCurrencies returns supported currencies.
func (r *Repository) ListCurrencies(ctx context.Context) ([]*platformadmin.Currency, error) {
	var list []*platformadmin.Currency
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, code, name, symbol, exchange_rate_egp, is_active, is_default FROM platform_admin.currencies WHERE is_active = true ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var cur platformadmin.Currency
			if err := rows.Scan(&cur.ID, &cur.Code, &cur.Name, &cur.Symbol, &cur.ExchangeRateEGP, &cur.IsActive, &cur.IsDefault); err != nil {
				return err
			}
			list = append(list, &cur)
		}
		return rows.Err()
	})
	return list, err
}

// ListLanguages returns supported languages.
func (r *Repository) ListLanguages(ctx context.Context) ([]*platformadmin.Language, error) {
	var list []*platformadmin.Language
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, code, name, dir, is_active, is_default FROM platform_admin.languages WHERE is_active = true ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var l platformadmin.Language
			if err := rows.Scan(&l.ID, &l.Code, &l.Name, &l.Dir, &l.IsActive, &l.IsDefault); err != nil {
				return err
			}
			list = append(list, &l)
		}
		return rows.Err()
	})
	return list, err
}

// CreateContactMessage persists a contact inquiry.
func (r *Repository) CreateContactMessage(ctx context.Context, m *platformadmin.ContactMessage) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.contact_messages (name, email, phone, subject, message, status)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, public_id, created_at;
		`
		return tx.QueryRow(txCtx, query, m.Name, m.Email, m.Phone, m.Subject, m.Message, m.Status).
			Scan(&m.ID, &m.PublicID, &m.CreatedAt)
	})
}

// ListContactMessages returns contact inquiries.
func (r *Repository) ListContactMessages(ctx context.Context, status string, limit, offset int) ([]*platformadmin.ContactMessage, error) {
	var list []*platformadmin.ContactMessage
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, name, email, phone, subject, message, status, created_at
			FROM platform_admin.contact_messages
			WHERE ($1 = '' OR status = $1)
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m platformadmin.ContactMessage
			var phone *string
			if err := rows.Scan(&m.ID, &m.PublicID, &m.Name, &m.Email, &phone, &m.Subject, &m.Message, &m.Status, &m.CreatedAt); err != nil {
				return err
			}
			if phone != nil {
				m.Phone = *phone
			}
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// ListPolicyVersions returns all versions for a policy key.
func (r *Repository) ListPolicyVersions(ctx context.Context, policyKey string) ([]*platformadmin.Policy, error) {
	var list []*platformadmin.Policy
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, policy_key, version, title, content, summary, is_published, published_at, created_by, created_at, updated_at
			FROM platform_admin.policies
			WHERE ($1 = '' OR policy_key = $1)
			ORDER BY policy_key ASC, version DESC, created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, policyKey)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p platformadmin.Policy
			if err := rows.Scan(
				&p.ID, &p.PolicyKey, &p.Version, &p.Title, &p.Content, &p.Summary,
				&p.IsPublished, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	return list, err
}

// GetPolicyVersion gets a specific version of a policy.
func (r *Repository) GetPolicyVersion(ctx context.Context, policyKey, version string) (*platformadmin.Policy, error) {
	var p platformadmin.Policy
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, policy_key, version, title, content, summary, is_published, published_at, created_by, created_at, updated_at
			FROM platform_admin.policies
			WHERE policy_key = $1 AND version = $2;
		`
		return tx.QueryRow(txCtx, query, policyKey, version).Scan(
			&p.ID, &p.PolicyKey, &p.Version, &p.Title, &p.Content, &p.Summary,
			&p.IsPublished, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("policy")
		}
		return nil, err
	}
	return &p, nil
}

// GetActivePolicy gets the currently published version of a policy.
func (r *Repository) GetActivePolicy(ctx context.Context, policyKey string) (*platformadmin.Policy, error) {
	var p platformadmin.Policy
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, policy_key, version, title, content, summary, is_published, published_at, created_by, created_at, updated_at
			FROM platform_admin.policies
			WHERE policy_key = $1 AND is_published = true
			ORDER BY published_at DESC NULLS LAST, created_at DESC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, policyKey).Scan(
			&p.ID, &p.PolicyKey, &p.Version, &p.Title, &p.Content, &p.Summary,
			&p.IsPublished, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("policy")
		}
		return nil, err
	}
	return &p, nil
}

// CreatePolicyVersion inserts a new draft policy version.
func (r *Repository) CreatePolicyVersion(ctx context.Context, p *platformadmin.Policy) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.policies (
				policy_key, version, title, content, summary, is_published, published_at, created_by
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			p.PolicyKey, p.Version, p.Title, p.Content, p.Summary, p.IsPublished, p.PublishedAt, p.CreatedBy,
		).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	})
}

// PublishPolicyVersion sets this version as published and unpublishes earlier versions.
func (r *Repository) PublishPolicyVersion(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var policyKey string
		err := tx.QueryRow(txCtx, `SELECT policy_key FROM platform_admin.policies WHERE id = $1;`, id).Scan(&policyKey)
		if err != nil {
			return err
		}

		// Unpublish previous versions
		_, err = tx.Exec(txCtx, `UPDATE platform_admin.policies SET is_published = false WHERE policy_key = $1;`, policyKey)
		if err != nil {
			return err
		}

		// Publish current version
		_, err = tx.Exec(txCtx, `
			UPDATE platform_admin.policies 
			SET is_published = true, published_at = NOW(), updated_at = NOW() 
			WHERE id = $1;
		`, id)
		return err
	})
}

