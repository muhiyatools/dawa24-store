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
