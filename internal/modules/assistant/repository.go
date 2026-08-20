package assistant

import "context"

// Repository defines data access for assistant conversations and messages.
type Repository interface {
	CreateConversation(ctx context.Context, c *Conversation) error
	GetConversation(ctx context.Context, id int64) (*Conversation, error)
	ListConversations(ctx context.Context, orgID, userID int64, limit, offset int) ([]*Conversation, error)
	SaveMessage(ctx context.Context, m *Message) error
	ListMessages(ctx context.Context, convID int64, limit int) ([]*Message, error)
}
