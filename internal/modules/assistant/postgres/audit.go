package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
)

// ListAllConversations returns all assistant sessions across organizations for administrative audit.
func (r *Repository) ListAllConversations(ctx context.Context, search string, limit, offset int) ([]*assistant.ConversationSummary, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	whereClause := "WHERE c.deleted_at IS NULL"
	args := []any{}
	argIdx := 1

	if search != "" {
		whereClause += fmt.Sprintf(` AND (
			c.title ILIKE $%d OR
			COALESCE(u.name->>'ar', u.name->>'en', '') ILIKE $%d OR
			u.email ILIKE $%d OR
			u.phone ILIKE $%d OR
			COALESCE(o.name->>'ar', o.name->>'en', '') ILIKE $%d
		)`, argIdx, argIdx, argIdx, argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	var total int
	var convs []*assistant.ConversationSummary

	err := r.db.InReadTx(systemCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := fmt.Sprintf(`
			SELECT COUNT(DISTINCT c.id)
			FROM assistant.conversations c
			LEFT JOIN org.organizations o ON o.id = c.organization_id
			LEFT JOIN identity.users u ON u.id = c.user_id
			%s
		`, whereClause)

		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("assistant: count conversations: %w", err)
		}

		query := fmt.Sprintf(`
			SELECT 
				c.id, c.public_id, c.organization_id, COALESCE(o.name->>'ar', o.name->>'en', 'منشأة #' || COALESCE(c.organization_id, 0)) AS org_name,
				COALESCE(o.type, '') AS org_type,
				c.user_id, COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم #' || c.user_id) AS user_name,
				COALESCE(u.email, '') AS user_email, COALESCE(u.phone, '') AS user_phone,
				COALESCE(u.role, '') AS user_role,
				c.title, c.created_at, c.updated_at,
				COUNT(m.id) AS message_count,
				COALESCE(SUM(m.input_tokens), 0) AS total_input_tokens,
				COALESCE(SUM(m.output_tokens), 0) AS total_output_tokens
			FROM assistant.conversations c
			LEFT JOIN org.organizations o ON o.id = c.organization_id
			LEFT JOIN identity.users u ON u.id = c.user_id
			LEFT JOIN assistant.messages m ON m.conversation_id = c.id
			%s
			GROUP BY c.id, c.public_id, c.organization_id, o.name, o.type, c.user_id, u.name, u.email, u.phone, u.role, c.title, c.created_at, c.updated_at
			ORDER BY c.updated_at DESC
			LIMIT $%d OFFSET $%d
		`, whereClause, argIdx, argIdx+1)

		pageArgs := append([]any{}, args...)
		pageArgs = append(pageArgs, limit, offset)
		rows, err := tx.Query(txCtx, query, pageArgs...)
		if err != nil {
			return fmt.Errorf("assistant: list all conversations: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var s assistant.ConversationSummary
			var orgID *int64
			if err := rows.Scan(
				&s.ID, &s.PublicID, &orgID, &s.OrganizationName, &s.OrganizationType,
				&s.UserID, &s.UserName, &s.UserEmail, &s.UserPhone, &s.UserRole,
				&s.Title, &s.CreatedAt, &s.UpdatedAt,
				&s.MessageCount, &s.TotalInputTokens, &s.TotalOutputTokens,
			); err != nil {
				return fmt.Errorf("assistant: scan conversation summary: %w", err)
			}
			if orgID != nil {
				s.OrganizationID = *orgID
			}
			convs = append(convs, &s)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}
	return convs, total, nil
}

// GetAssistantStats aggregates platform-wide assistant usage metrics.
func (r *Repository) GetAssistantStats(ctx context.Context) (*assistant.AssistantStats, error) {
	var stats assistant.AssistantStats
	err := r.db.InReadTx(systemCtx(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 
				(SELECT COUNT(*) FROM assistant.conversations WHERE deleted_at IS NULL),
				(SELECT COUNT(*) FROM assistant.messages),
				(SELECT COALESCE(SUM(input_tokens), 0) FROM assistant.messages),
				(SELECT COALESCE(SUM(output_tokens), 0) FROM assistant.messages),
				(SELECT COUNT(DISTINCT user_id) FROM assistant.conversations WHERE deleted_at IS NULL)
		`
		return tx.QueryRow(txCtx, query).Scan(
			&stats.TotalConversations,
			&stats.TotalMessages,
			&stats.TotalInputTokens,
			&stats.TotalOutputTokens,
			&stats.ActiveUsers,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: get stats: %w", err)
	}
	return &stats, nil
}
