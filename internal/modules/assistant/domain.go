package assistant

import (
	"time"

	"github.com/google/uuid"
)

// DigestMaxChars caps the maximum length of an attachment summary.
const DigestMaxChars = 6000

// Conversation represents a multi-turn assistant session.
type Conversation struct {
	ID             int64      `json:"id"`
	PublicID       uuid.UUID  `json:"public_id"`
	OrganizationID int64      `json:"organization_id"`
	UserID         int64      `json:"user_id"`
	Title          string     `json:"title"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// ConversationSummary holds enriched metadata about an assistant conversation for administration and auditing.
type ConversationSummary struct {
	ID                int64     `json:"id"`
	PublicID          uuid.UUID `json:"public_id"`
	OrganizationID    int64     `json:"organization_id"`
	OrganizationName  string    `json:"organization_name"`
	OrganizationType  string    `json:"organization_type,omitempty"`
	UserID            int64     `json:"user_id"`
	UserName          string    `json:"user_name"`
	UserEmail         string    `json:"user_email"`
	UserPhone         string    `json:"user_phone"`
	UserRole          string    `json:"user_role,omitempty"`
	Title             string    `json:"title"`
	MessageCount      int       `json:"message_count"`
	TotalInputTokens  int       `json:"total_input_tokens"`
	TotalOutputTokens int       `json:"total_output_tokens"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// AssistantStats provides aggregated metrics for the AI Assistant platform overview.
type AssistantStats struct {
	TotalConversations int `json:"total_conversations"`
	TotalMessages      int `json:"total_messages"`
	TotalInputTokens   int `json:"total_input_tokens"`
	TotalOutputTokens  int `json:"total_output_tokens"`
	ActiveUsers        int `json:"active_users"`
}

// Message represents one turn in a conversation.
type Message struct {
	ID               int64        `json:"id"`
	ConversationID   int64        `json:"conversation_id"`
	OrganizationID   int64        `json:"organization_id"`
	Role             string       `json:"role"` // system | user | assistant | tool
	Content          string       `json:"content"`
	Attachments      []Attachment `json:"attachments"`
	PromptVersion    string       `json:"prompt_version"`
	ModelRole        string       `json:"model_role"`
	InputTokens      int          `json:"input_tokens"`
	OutputTokens     int          `json:"output_tokens"`
	GatewayRequestID string       `json:"gateway_request_id"`
	CreatedAt        time.Time    `json:"created_at"`
}

// Attachment holds uploaded file metadata and content references.
type Attachment struct {
	Handle      string  `json:"handle"`
	Filename    string  `json:"filename"`
	MIMEType    string  `json:"mime_type"`
	SizeMB      float64 `json:"size_mb"`
	DataURL     string  `json:"data_url,omitempty"`
	ContentHash string  `json:"content_hash"`
	UserID      int64   `json:"user_id"`
	OrgID       int64   `json:"org_id"`
}

// Digest is the attachment model's structured understanding of one file.
type Digest struct {
	Filename  string   `json:"filename"`
	Kind      string   `json:"kind"`
	Summary   string   `json:"summary"`
	KeyFacts  []string `json:"key_facts"`
	Verbatim  string   `json:"verbatim,omitempty"`
	Truncated bool     `json:"truncated"`
}

// RenderBlock formats a digest into a delimited Markdown block for the primary model context.
func (d Digest) RenderBlock() string {
	var s string
	s += "[مرفق: " + d.Filename + " — تحليل]\n"
	if d.Summary != "" {
		s += d.Summary + "\n"
	}
	if len(d.KeyFacts) > 0 {
		s += "النقاط الرئيسية:\n"
		for _, k := range d.KeyFacts {
			s += "- " + k + "\n"
		}
	}
	if d.Verbatim != "" {
		s += "النص المفرغ:\n" + d.Verbatim + "\n"
	}
	if d.Truncated {
		s += "(تنبيه: تم اقتصاص التحليل لتجاوز الحد الأقصى للسياق)\n"
	}
	s += "[/مرفق]"
	return s
}
