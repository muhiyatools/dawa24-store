package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Repository provides PostgreSQL persistence for the assistant module.
type Repository struct {
	db *database.DB
}

// NewRepository constructs a PostgreSQL assistant repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

var _ assistant.Repository = (*Repository)(nil)

// CreateConversation inserts a new conversation row.
func (r *Repository) CreateConversation(ctx context.Context, c *assistant.Conversation) error {
	pool := r.db.Pool()

	query := `
		INSERT INTO assistant.conversations (
			organization_id, user_id, title
		) VALUES ($1, $2, $3)
		RETURNING id, public_id, created_at, updated_at
	`
	err := pool.QueryRow(ctx, query, c.OrganizationID, c.UserID, c.Title).
		Scan(&c.ID, &c.PublicID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("assistant: create conversation: %w", err)
	}
	return nil
}

// GetConversation fetches one conversation by primary key.
func (r *Repository) GetConversation(ctx context.Context, id int64) (*assistant.Conversation, error) {
	pool := r.db.Pool()

	query := `
		SELECT id, public_id, organization_id, user_id, title, created_at, updated_at, deleted_at
		FROM assistant.conversations
		WHERE id = $1 AND deleted_at IS NULL
	`
	var c assistant.Conversation
	err := pool.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.PublicID, &c.OrganizationID, &c.UserID, &c.Title,
		&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("assistant: get conversation: %w", err)
	}
	return &c, nil
}

// ListConversations returns recent active conversations for a user.
func (r *Repository) ListConversations(ctx context.Context, orgID, userID int64, limit, offset int) ([]*assistant.Conversation, error) {
	pool := r.db.Pool()

	if limit <= 0 || limit > 100 {
		limit = 30
	}

	query := `
		SELECT id, public_id, organization_id, user_id, title, created_at, updated_at, deleted_at
		FROM assistant.conversations
		WHERE organization_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY updated_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := pool.Query(ctx, query, orgID, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("assistant: list conversations: %w", err)
	}
	defer rows.Close()

	var convs []*assistant.Conversation
	for rows.Next() {
		var c assistant.Conversation
		if err := rows.Scan(
			&c.ID, &c.PublicID, &c.OrganizationID, &c.UserID, &c.Title,
			&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("assistant: scan conversation: %w", err)
		}
		convs = append(convs, &c)
	}
	return convs, rows.Err()
}

// SaveMessage records a turn in a conversation.
func (r *Repository) SaveMessage(ctx context.Context, m *assistant.Message) error {
	pool := r.db.Pool()

	attsJSON, err := json.Marshal(m.Attachments)
	if err != nil {
		attsJSON = []byte("[]")
	}

	query := `
		INSERT INTO assistant.messages (
			conversation_id, organization_id, role, content,
			attachments, prompt_version, model_role, input_tokens,
			output_tokens, gateway_request_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`
	err = pool.QueryRow(
		ctx, query,
		m.ConversationID, m.OrganizationID, m.Role, m.Content,
		attsJSON, m.PromptVersion, m.ModelRole, m.InputTokens,
		m.OutputTokens, m.GatewayRequestID,
	).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return fmt.Errorf("assistant: save message: %w", err)
	}

	// Update conversation updated_at
	_, _ = pool.Exec(ctx, `UPDATE assistant.conversations SET updated_at = now() WHERE id = $1`, m.ConversationID)
	return nil
}

// ListMessages returns turn history for a conversation in chronological order.
func (r *Repository) ListMessages(ctx context.Context, convID int64, limit int) ([]*assistant.Message, error) {
	pool := r.db.Pool()

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `
		SELECT id, conversation_id, organization_id, role, content,
		       attachments, prompt_version, model_role, input_tokens,
		       output_tokens, gateway_request_id, created_at
		FROM assistant.messages
		WHERE conversation_id = $1
		ORDER BY id ASC
		LIMIT $2
	`
	rows, err := pool.Query(ctx, query, convID, limit)
	if err != nil {
		return nil, fmt.Errorf("assistant: list messages: %w", err)
	}
	defer rows.Close()

	var msgs []*assistant.Message
	for rows.Next() {
		var m assistant.Message
		var attsRaw []byte
		if err := rows.Scan(
			&m.ID, &m.ConversationID, &m.OrganizationID, &m.Role, &m.Content,
			&attsRaw, &m.PromptVersion, &m.ModelRole, &m.InputTokens,
			&m.OutputTokens, &m.GatewayRequestID, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("assistant: scan message: %w", err)
		}
		if len(attsRaw) > 0 {
			_ = json.Unmarshal(attsRaw, &m.Attachments)
		}
		msgs = append(msgs, &m)
	}
	return msgs, rows.Err()
}
