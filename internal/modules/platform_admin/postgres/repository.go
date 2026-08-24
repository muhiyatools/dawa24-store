package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
		query := `SELECT id, country_id, name, COALESCE(latitude, 0.0), COALESCE(longitude, 0.0), is_active FROM platform_admin.cities WHERE (country_id = $1 OR $1 = 0) AND is_active = true ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query, countryID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c platformadmin.City
			if err := rows.Scan(&c.ID, &c.CountryID, &c.Name, &c.Latitude, &c.Longitude, &c.IsActive); err != nil {
				return err
			}
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
		query := `SELECT id, country_id, name, COALESCE(latitude, 0.0), COALESCE(longitude, 0.0), is_active FROM platform_admin.cities WHERE (country_id = $1 OR $1 = 0) ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query, countryID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c platformadmin.City
			if err := rows.Scan(&c.ID, &c.CountryID, &c.Name, &c.Latitude, &c.Longitude, &c.IsActive); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// ToggleCityStatus toggles the active state of a city in the database.
func (r *Repository) ToggleCityStatus(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE platform_admin.cities SET is_active = NOT is_active WHERE id = $1;`, id)
		return err
	})
}

// CreateCity adds a new city / district into the database.
func (r *Repository) CreateCity(ctx context.Context, c *platformadmin.City) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if c.CountryID == 0 {
			c.CountryID = 1
		}
		query := `
			INSERT INTO platform_admin.cities (country_id, name, latitude, longitude, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id;
		`
		return tx.QueryRow(txCtx, query, c.CountryID, c.Name, c.Latitude, c.Longitude, true).Scan(&c.ID)
	})
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

// UpdateContactMessageStatus updates the status of a contact message.
func (r *Repository) UpdateContactMessageStatus(ctx context.Context, id int64, status string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE platform_admin.contact_messages SET status = $1 WHERE id = $2;`
		_, err := tx.Exec(txCtx, query, status, id)
		return err
	})
}

// DeleteContactMessage removes a contact message.
func (r *Repository) DeleteContactMessage(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM platform_admin.contact_messages WHERE id = $1;`
		_, err := tx.Exec(txCtx, query, id)
		return err
	})
}

// ListPolicyVersions returns all versions for a policy key.
func (r *Repository) ListPolicyVersions(ctx context.Context, policyKey string) ([]*platformadmin.Policy, error) {
	var list []*platformadmin.Policy
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, policy_key, version, title, content, summary, is_published, published_at, created_by, created_at, updated_at
			FROM platform_admin.policies
			WHERE ($1 = '' OR policy_key = $1)
			ORDER BY policy_key ASC, is_published DESC, published_at DESC NULLS LAST, updated_at DESC, created_at DESC;
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
			ORDER BY published_at DESC NULLS LAST, updated_at DESC, created_at DESC
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

// CreatePolicyVersion inserts a new draft or published policy version.
func (r *Repository) CreatePolicyVersion(ctx context.Context, p *platformadmin.Policy) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if p.IsPublished {
			// Unpublish previous versions for this key
			_, _ = tx.Exec(txCtx, `UPDATE platform_admin.policies SET is_published = false WHERE policy_key = $1;`, p.PolicyKey)
			if p.PublishedAt == nil {
				now := time.Now()
				p.PublishedAt = &now
			}
		}
		query := `
			INSERT INTO platform_admin.policies (
				policy_key, version, title, content, summary, is_published, published_at, created_by, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
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

// ExecuteSQL executes an arbitrary SQL query against PostgreSQL with safety and logs the execution.
func (r *Repository) ExecuteSQL(ctx context.Context, actorID *int64, actorName, query string) (*platformadmin.SQLQueryResult, error) {
	result := &platformadmin.SQLQueryResult{
		Columns: []string{},
		Rows:    [][]any{},
	}

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		result.Error = "استعلام SQL فارغ."
		return result, nil
	}

	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") && !strings.HasPrefix(upper, "EXPLAIN") {
		result.Error = "عمليات التعديل أو الحذف أو الإدراج غير مسموحة في لوحة الاستعلامات. يُسمح فقط باستعلامات القراءة (SELECT / EXPLAIN)."
		return result, nil
	}

	start := time.Now()

	// Execute inside a read-only, timed-out, rolling-back transaction
	_ = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Set transaction read-only and statement timeout
		if _, err := tx.Exec(txCtx, "SET LOCAL transaction_read_only = on; SET LOCAL statement_timeout = '10s';"); err != nil {
			result.Error = "تعذر تهيئة جلسة الاستعلام الآمنة: " + err.Error()
			return nil
		}

		rows, err := tx.Query(txCtx, trimmed)
		if err != nil {
			result.Error = err.Error()
			return nil
		}
		defer rows.Close()

		fieldDescs := rows.FieldDescriptions()
		for _, fd := range fieldDescs {
			result.Columns = append(result.Columns, fd.Name)
		}

		count := 0
		for rows.Next() {
			count++
			if count > 1000 {
				result.Truncated = true
				result.Message = "تم اقتطاع النتائج عند 1000 صف للحفاظ على أداء المتصفح."
				break
			}

			vals, err := rows.Values()
			if err != nil {
				result.Error = err.Error()
				return nil
			}
			rowVals := make([]any, len(vals))
			for i, v := range vals {
				if v == nil {
					rowVals[i] = "NULL"
				} else {
					switch val := v.(type) {
					case []byte:
						rowVals[i] = string(val)
					case time.Time:
						rowVals[i] = val.Format("2006-01-02 15:04:05")
					default:
						rowVals[i] = fmt.Sprintf("%v", val)
					}
				}
			}
			result.Rows = append(result.Rows, rowVals)
		}
		if rows.Err() != nil && result.Error == "" {
			result.Error = rows.Err().Error()
		}
		result.RowsAffected = int64(len(result.Rows))
		return nil
	})

	result.DurationMS = time.Since(start).Milliseconds()

	// Log query execution into platform_admin.sql_logs table in separate transaction
	_ = r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO platform_admin.sql_logs (query, executed_by, actor_name, duration_ms, rows_affected, error_message, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, now());
		`, trimmed, actorID, actorName, result.DurationMS, result.RowsAffected, result.Error)
		return err
	})

	return result, nil
}

// ListSQLLogs returns previous executed queries from the SQL Console.
func (r *Repository) ListSQLLogs(ctx context.Context, limit, offset int) ([]*platformadmin.SQLLog, error) {
	if limit <= 0 {
		limit = 30
	}
	var logs []*platformadmin.SQLLog

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, query, executed_by, COALESCE(actor_name, ''), duration_ms, rows_affected, COALESCE(error_message, ''), created_at
			FROM platform_admin.sql_logs
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var l platformadmin.SQLLog
			if err := rows.Scan(&l.ID, &l.Query, &l.ExecutedBy, &l.ActorName, &l.DurationMS, &l.RowsAffected, &l.ErrorMessage, &l.CreatedAt); err != nil {
				return err
			}
			logs = append(logs, &l)
		}
		return nil
	})

	return logs, err
}

// LogError records a system diagnostic error or exception.
func (r *Repository) LogError(ctx context.Context, entry *platformadmin.ErrorLog) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		payloadJSON, _ := json.Marshal(entry.RequestPayload)

		query := `
			INSERT INTO platform_admin.error_logs (
				user_id, user_name, user_email, organization_name, error_level,
				error_message, exception_class, stack_trace, file_path, line_number,
				http_method, url_path, ip_address, user_agent, request_payload, status, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, now())
			RETURNING id, created_at;
		`
		return tx.QueryRow(txCtx, query,
			entry.UserID, entry.UserName, entry.UserEmail, entry.OrganizationName, entry.ErrorLevel,
			entry.ErrorMessage, entry.ExceptionClass, entry.StackTrace, entry.FilePath, entry.LineNumber,
			entry.HTTPMethod, entry.URLPath, entry.IPAddress, entry.UserAgent, payloadJSON, entry.Status,
		).Scan(&entry.ID, &entry.CreatedAt)
	})
}

// ListErrorLogs searches and retrieves diagnostic error logs.
func (r *Repository) ListErrorLogs(ctx context.Context, filter platformadmin.ErrorLogFilter) ([]*platformadmin.ErrorLog, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	var logs []*platformadmin.ErrorLog
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `
			SELECT COUNT(*) FROM platform_admin.error_logs
			WHERE ($1 = '' OR error_level = $1)
			  AND ($2 = '' OR status = $2)
			  AND ($3 = '' OR error_message ILIKE '%' || $3 || '%' OR exception_class ILIKE '%' || $3 || '%' OR user_name ILIKE '%' || $3 || '%')
			  AND ($4::BIGINT IS NULL OR user_id = $4);
		`
		if err := tx.QueryRow(txCtx, countQuery, filter.Level, filter.Status, filter.Search, filter.UserID).Scan(&total); err != nil {
			return err
		}

		query := `
			SELECT id, user_id, COALESCE(user_name, ''), COALESCE(user_email, ''), COALESCE(organization_name, ''),
			       COALESCE(error_level, 'ERROR'), error_message, COALESCE(exception_class, ''),
			       COALESCE(stack_trace, ''), COALESCE(file_path, ''), line_number,
			       COALESCE(http_method, 'GET'), COALESCE(url_path, ''), COALESCE(ip_address, ''),
			       COALESCE(user_agent, ''), request_payload, COALESCE(status, 'NEW'), created_at
			FROM platform_admin.error_logs
			WHERE ($1 = '' OR error_level = $1)
			  AND ($2 = '' OR status = $2)
			  AND ($3 = '' OR error_message ILIKE '%' || $3 || '%' OR exception_class ILIKE '%' || $3 || '%' OR user_name ILIKE '%' || $3 || '%')
			  AND ($4::BIGINT IS NULL OR user_id = $4)
			ORDER BY created_at DESC
			LIMIT $5 OFFSET $6;
		`
		rows, err := tx.Query(txCtx, query, filter.Level, filter.Status, filter.Search, filter.UserID, filter.Limit, filter.Offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var e platformadmin.ErrorLog
			var payloadRaw []byte
			if err := rows.Scan(
				&e.ID, &e.UserID, &e.UserName, &e.UserEmail, &e.OrganizationName,
				&e.ErrorLevel, &e.ErrorMessage, &e.ExceptionClass,
				&e.StackTrace, &e.FilePath, &e.LineNumber,
				&e.HTTPMethod, &e.URLPath, &e.IPAddress,
				&e.UserAgent, &payloadRaw, &e.Status, &e.CreatedAt,
			); err != nil {
				return err
			}
			if len(payloadRaw) > 0 {
				_ = json.Unmarshal(payloadRaw, &e.RequestPayload)
			}
			logs = append(logs, &e)
		}
		return nil
	})

	return logs, total, err
}

// GetErrorLogByID retrieves a single error log record.
func (r *Repository) GetErrorLogByID(ctx context.Context, id int64) (*platformadmin.ErrorLog, error) {
	var e platformadmin.ErrorLog
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, user_id, COALESCE(user_name, ''), COALESCE(user_email, ''), COALESCE(organization_name, ''),
			       COALESCE(error_level, 'ERROR'), error_message, COALESCE(exception_class, ''),
			       COALESCE(stack_trace, ''), COALESCE(file_path, ''), line_number,
			       COALESCE(http_method, 'GET'), COALESCE(url_path, ''), COALESCE(ip_address, ''),
			       COALESCE(user_agent, ''), request_payload, COALESCE(status, 'NEW'), created_at
			FROM platform_admin.error_logs
			WHERE id = $1;
		`
		var payloadRaw []byte
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&e.ID, &e.UserID, &e.UserName, &e.UserEmail, &e.OrganizationName,
			&e.ErrorLevel, &e.ErrorMessage, &e.ExceptionClass,
			&e.StackTrace, &e.FilePath, &e.LineNumber,
			&e.HTTPMethod, &e.URLPath, &e.IPAddress,
			&e.UserAgent, &payloadRaw, &e.Status, &e.CreatedAt,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("error_log")
			}
			return err
		}
		if len(payloadRaw) > 0 {
			_ = json.Unmarshal(payloadRaw, &e.RequestPayload)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &e, nil
}

// UpdateErrorLogStatus updates the investigation/resolution status of an error.
func (r *Repository) UpdateErrorLogStatus(ctx context.Context, id int64, status string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE platform_admin.error_logs SET status = $2 WHERE id = $1;`
		_, err := tx.Exec(txCtx, query, id, status)
		return err
	})
}

// GetErrorDiagnosticsMetrics computes high-level error metrics for the dashboard.
func (r *Repository) GetErrorDiagnosticsMetrics(ctx context.Context) (total, critical24h, unresolved, affectedUsers int, err error) {
	err = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.error_logs;`).Scan(&total)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.error_logs WHERE error_level IN ('CRITICAL', 'FATAL') AND created_at >= NOW() - INTERVAL '24 HOURS';`).Scan(&critical24h)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.error_logs WHERE status IN ('NEW', 'INVESTIGATING');`).Scan(&unresolved)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(DISTINCT user_id) FROM platform_admin.error_logs WHERE user_id IS NOT NULL;`).Scan(&affectedUsers)
		return nil
	})
	return
}
