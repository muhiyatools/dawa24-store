package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Repository implements chat.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a chat repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateConversation inserts a conversation row.
func (r *Repository) CreateConversation(ctx context.Context, c *chat.Conversation) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO chat.conversations (organization_id, counterparty_org_id, subject, context_type, context_id, status, created_by_user_id)
			VALUES ($1, $2, $3, $4, $5, 'open', $6)
			RETURNING id, public_id, status, created_at, updated_at;
		`
		var status string
		return tx.QueryRow(txCtx, query,
			c.OrganizationID, c.CounterpartyOrgID, c.Subject, string(c.ContextType), c.ContextID, c.CreatedByUserID,
		).Scan(&c.ID, &c.PublicID, &status, &c.CreatedAt, &c.UpdatedAt)
	})
}

// GetConversationByID fetches one conversation.
func (r *Repository) GetConversationByID(ctx context.Context, id int64) (*chat.Conversation, error) {
	var c chat.Conversation
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, organization_id, counterparty_org_id, subject,
			       context_type, context_id, status, last_message_at, COALESCE(created_by_user_id, 0),
			       created_at, updated_at
			FROM chat.conversations WHERE id = $1;
		`
		var ctxType, status string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&c.ID, &c.PublicID, &c.OrganizationID, &c.CounterpartyOrgID, &c.Subject,
			&ctxType, &c.ContextID, &status, &c.LastMessageAt, &c.CreatedByUserID,
			&c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return chat.ErrNoConversation
			}
			return err
		}
		c.ContextType = chat.ContextType(ctxType)
		c.Status = chat.ConversationStatus(status)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListConversationsForOrg returns the org's threads newest-first.
func (r *Repository) ListConversationsForOrg(ctx context.Context, orgID int64, limit, offset int) ([]*chat.Conversation, error) {
	var list []*chat.Conversation
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, organization_id, counterparty_org_id, subject,
			       context_type, context_id, status, last_message_at, COALESCE(created_by_user_id, 0),
			       created_at, updated_at
			FROM chat.conversations
			WHERE organization_id = $1 OR counterparty_org_id = $1
			ORDER BY COALESCE(last_message_at, created_at) DESC
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
			var c chat.Conversation
			var ctxType, status string
			if err := rows.Scan(
				&c.ID, &c.PublicID, &c.OrganizationID, &c.CounterpartyOrgID, &c.Subject,
				&ctxType, &c.ContextID, &status, &c.LastMessageAt, &c.CreatedByUserID,
				&c.CreatedAt, &c.UpdatedAt,
			); err != nil {
				return err
			}
			c.ContextType = chat.ContextType(ctxType)
			c.Status = chat.ConversationStatus(status)
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// AddParticipant registers a user on a conversation.
func (r *Repository) AddParticipant(ctx context.Context, conversationID, userID, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `INSERT INTO chat.participants (conversation_id, user_id, organization_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;`
		_, err := tx.Exec(txCtx, query, conversationID, userID, orgID)
		return err
	})
}

// SendMessage inserts a message and bumps the conversation timestamp.
func (r *Repository) SendMessage(ctx context.Context, m *chat.Message) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO chat.messages (conversation_id, sender_user_id, sender_org_id, body, attachments)
			VALUES ($1, $2, $3, $4, COALESCE($5, '[]'::jsonb))
			RETURNING id, created_at;
		`
		if err := tx.QueryRow(txCtx, query, m.ConversationID, m.SenderUserID, m.SenderOrgID, m.Body, m.Attachments).
			Scan(&m.ID, &m.CreatedAt); err != nil {
			return fmt.Errorf("chat postgres: send message: %w", err)
		}
		const touch = `UPDATE chat.conversations SET last_message_at = now(), updated_at = now() WHERE id = $1;`
		_, err := tx.Exec(txCtx, touch, m.ConversationID)
		return err
	})
}

// ListMessages returns a conversation's messages oldest-first.
func (r *Repository) ListMessages(ctx context.Context, conversationID int64, limit int) ([]*chat.Message, error) {
	var list []*chat.Message
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, conversation_id, sender_user_id, sender_org_id, body, attachments, read_at, created_at
			FROM chat.messages
			WHERE conversation_id = $1
			ORDER BY created_at ASC, id ASC
			LIMIT $2;
		`
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		rows, err := tx.Query(txCtx, query, conversationID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m chat.Message
			var attachments []byte
			if err := rows.Scan(
				&m.ID, &m.ConversationID, &m.SenderUserID, &m.SenderOrgID, &m.Body, &attachments, &m.ReadAt, &m.CreatedAt,
			); err != nil {
				return err
			}
			_ = json.Unmarshal(attachments, &m.Attachments)
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// MarkConversationRead clears unread state for one side of a conversation.
func (r *Repository) MarkConversationRead(ctx context.Context, conversationID, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE chat.messages SET read_at = now()
			WHERE conversation_id = $1 AND sender_org_id IS DISTINCT FROM $2 AND read_at IS NULL;
		`
		_, err := tx.Exec(txCtx, query, conversationID, orgID)
		return err
	})
}

// CountUnread returns the number of conversations with unread incoming messages.
func (r *Repository) CountUnread(ctx context.Context, orgID int64) (int, error) {
	var count int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COUNT(DISTINCT m.conversation_id)
			FROM chat.messages m
			JOIN chat.conversations c ON c.id = m.conversation_id
			WHERE (c.organization_id = $1 OR c.counterparty_org_id = $1)
			  AND m.sender_org_id IS DISTINCT FROM $1
			  AND m.read_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, orgID).Scan(&count)
	})
	return count, err
}
