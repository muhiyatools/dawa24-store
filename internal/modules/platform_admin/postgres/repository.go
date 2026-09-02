package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
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

// ListGovernorates returns active governorates for a country.
func (r *Repository) ListGovernorates(ctx context.Context, countryID int64) ([]*platformadmin.Governorate, error) {
	var list []*platformadmin.Governorate
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, country_id, name, COALESCE(latitude, 0.0), COALESCE(longitude, 0.0), is_active
			FROM platform_admin.governorates
			WHERE (country_id = $1 OR $1 = 0) AND is_active = true
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, query, countryID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var g platformadmin.Governorate
			if err := rows.Scan(&g.ID, &g.CountryID, &g.Name, &g.Latitude, &g.Longitude, &g.IsActive); err != nil {
				return err
			}
			list = append(list, &g)
		}
		return rows.Err()
	})
	return list, err
}

// ListAllGovernorates returns all governorates with city counts for admin management.
func (r *Repository) ListAllGovernorates(ctx context.Context, countryID int64) ([]*platformadmin.Governorate, error) {
	var list []*platformadmin.Governorate
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT g.id, g.country_id, g.name, COALESCE(g.latitude, 0.0), COALESCE(g.longitude, 0.0), g.is_active, COUNT(c.id)
			FROM platform_admin.governorates g
			LEFT JOIN platform_admin.cities c ON c.governorate_id = g.id
			WHERE (g.country_id = $1 OR $1 = 0)
			GROUP BY g.id, g.country_id, g.name, g.latitude, g.longitude, g.is_active
			ORDER BY g.id ASC;
		`
		rows, err := tx.Query(txCtx, query, countryID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var g platformadmin.Governorate
			if err := rows.Scan(&g.ID, &g.CountryID, &g.Name, &g.Latitude, &g.Longitude, &g.IsActive, &g.CityCount); err != nil {
				return err
			}
			list = append(list, &g)
		}
		return rows.Err()
	})
	return list, err
}

// GetGovernorate returns a single governorate by ID.
func (r *Repository) GetGovernorate(ctx context.Context, id int64) (*platformadmin.Governorate, error) {
	var g platformadmin.Governorate
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, country_id, name, COALESCE(latitude, 0.0), COALESCE(longitude, 0.0), is_active
			FROM platform_admin.governorates
			WHERE id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(&g.ID, &g.CountryID, &g.Name, &g.Latitude, &g.Longitude, &g.IsActive)
	})
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// CreateGovernorate inserts a new governorate.
func (r *Repository) CreateGovernorate(ctx context.Context, g *platformadmin.Governorate) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if g.CountryID == 0 {
			g.CountryID = 1
		}
		query := `
			INSERT INTO platform_admin.governorates (country_id, name, latitude, longitude, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id;
		`
		return tx.QueryRow(txCtx, query, g.CountryID, g.Name, g.Latitude, g.Longitude, g.IsActive).Scan(&g.ID)
	})
}

// UpdateGovernorate updates an existing governorate.
func (r *Repository) UpdateGovernorate(ctx context.Context, g *platformadmin.Governorate) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE platform_admin.governorates
			SET name = $2, latitude = $3, longitude = $4, is_active = $5, updated_at = now()
			WHERE id = $1;
		`
		_, err := tx.Exec(txCtx, query, g.ID, g.Name, g.Latitude, g.Longitude, g.IsActive)
		return err
	})
}

// ToggleGovernorateStatus toggles the active state of a governorate.
func (r *Repository) ToggleGovernorateStatus(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE platform_admin.governorates SET is_active = NOT is_active, updated_at = now() WHERE id = $1;`, id)
		return err
	})
}

// ListCities returns active cities for a country.
func (r *Repository) ListCities(ctx context.Context, countryID int64) ([]*platformadmin.City, error) {
	var list []*platformadmin.City
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT c.id, c.country_id, c.governorate_id, g.name, c.name, COALESCE(c.latitude, 0.0), COALESCE(c.longitude, 0.0), c.is_active, COALESCE(c.is_capital, false), COALESCE(c.coverage_radius_meters, 3000)
			FROM platform_admin.cities c
			LEFT JOIN platform_admin.governorates g ON g.id = c.governorate_id
			WHERE (c.country_id = $1 OR $1 = 0) AND c.is_active = true
			ORDER BY c.governorate_id ASC, c.id ASC;
		`
		rows, err := tx.Query(txCtx, query, countryID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c platformadmin.City
			var govName *i18n.Text
			if err := rows.Scan(&c.ID, &c.CountryID, &c.GovernorateID, &govName, &c.Name, &c.Latitude, &c.Longitude, &c.IsActive, &c.IsCapital, &c.CoverageRadiusMeters); err != nil {
				return err
			}
			c.GovernorateName = govName
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// ListAllCities returns all cities for admin management.
func (r *Repository) ListAllCities(ctx context.Context, countryID int64) ([]*platformadmin.City, error) {
	var list []*platformadmin.City
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT c.id, c.country_id, c.governorate_id, g.name, c.name, COALESCE(c.latitude, 0.0), COALESCE(c.longitude, 0.0), c.is_active, COALESCE(c.is_capital, false), COALESCE(c.coverage_radius_meters, 3000)
			FROM platform_admin.cities c
			LEFT JOIN platform_admin.governorates g ON g.id = c.governorate_id
			WHERE (c.country_id = $1 OR $1 = 0)
			ORDER BY c.governorate_id ASC, c.id ASC;
		`
		rows, err := tx.Query(txCtx, query, countryID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c platformadmin.City
			var govName *i18n.Text
			if err := rows.Scan(&c.ID, &c.CountryID, &c.GovernorateID, &govName, &c.Name, &c.Latitude, &c.Longitude, &c.IsActive, &c.IsCapital, &c.CoverageRadiusMeters); err != nil {
				return err
			}
			c.GovernorateName = govName
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// ListCitiesByGovernorate returns all cities belonging to a specific governorate.
func (r *Repository) ListCitiesByGovernorate(ctx context.Context, governorateID int64) ([]*platformadmin.City, error) {
	var list []*platformadmin.City
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT c.id, c.country_id, c.governorate_id, g.name, c.name, COALESCE(c.latitude, 0.0), COALESCE(c.longitude, 0.0), c.is_active, COALESCE(c.is_capital, false), COALESCE(c.coverage_radius_meters, 3000)
			FROM platform_admin.cities c
			LEFT JOIN platform_admin.governorates g ON g.id = c.governorate_id
			WHERE c.governorate_id = $1
			ORDER BY c.id ASC;
		`
		rows, err := tx.Query(txCtx, query, governorateID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c platformadmin.City
			var govName *i18n.Text
			if err := rows.Scan(&c.ID, &c.CountryID, &c.GovernorateID, &govName, &c.Name, &c.Latitude, &c.Longitude, &c.IsActive, &c.IsCapital, &c.CoverageRadiusMeters); err != nil {
				return err
			}
			c.GovernorateName = govName
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// GetCity returns a single city by ID with its governorate details.
func (r *Repository) GetCity(ctx context.Context, id int64) (*platformadmin.City, error) {
	var c platformadmin.City
	var govName *i18n.Text
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT c.id, c.country_id, c.governorate_id, g.name, c.name, COALESCE(c.latitude, 0.0), COALESCE(c.longitude, 0.0), c.is_active, COALESCE(c.is_capital, false), COALESCE(c.coverage_radius_meters, 3000)
			FROM platform_admin.cities c
			LEFT JOIN platform_admin.governorates g ON g.id = c.governorate_id
			WHERE c.id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(&c.ID, &c.CountryID, &c.GovernorateID, &govName, &c.Name, &c.Latitude, &c.Longitude, &c.IsActive, &c.IsCapital, &c.CoverageRadiusMeters)
	})
	if err != nil {
		return nil, err
	}
	c.GovernorateName = govName
	return &c, nil
}
