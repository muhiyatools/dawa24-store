package gateway

import (
	"errors"
)

// PartKind categorises multimodal content parts.
type PartKind string

const (
	PartText  PartKind = "text"
	PartImage PartKind = "image"
	PartAudio PartKind = "audio"
	PartVideo PartKind = "video"
	PartFile  PartKind = "file"
)

// ContentPart is one piece of multimodal content in the Gateway's OpenAI dialect.
type ContentPart struct {
	Kind     PartKind
	Text     string
	MIMEType string
	DataURL  string
	Filename string
}

// ChatMessage is one turn in a conversation.
type ChatMessage struct {
	Role  string
	Text  string
	Parts []ContentPart
}

// ToolSpec reserves space for future tool definitions.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Usage reports token counts from a completion turn.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatRequest carries the parameters for a streaming completion.
type ChatRequest struct {
	Role        Role
	Messages    []ChatMessage
	MaxTokens   int
	Temperature float64
	Tools       []ToolSpec // ALWAYS EMPTY IN THIS PHASE — asserted in tests
	OrgID       int64
	UserID      int64
	VirtualKey  string // Tenant virtual key
	// Feature names the screen that asked, for the usage ledger. See
	// Request.Feature.
	Feature string
}

// StreamEvent represents one decoded SSE chunk or lifecycle event.
type StreamEvent struct {
	Delta     string
	Reasoning string
	Done      bool
	Err       error
	Usage     *Usage
}

// ErrToolsNotSupported is returned when a caller provides tools in this phase.
var ErrToolsNotSupported = errors.New("gateway: tools not supported in this phase")

func buildWireMessages(messages []ChatMessage) []wireChatMessage {
	wireMsgs := make([]wireChatMessage, 0, len(messages))
	for _, m := range messages {
		wm := wireChatMessage{Role: m.Role}
		if len(m.Parts) == 0 {
			wm.Content = m.Text
		} else {
			parts := make([]wireContentPart, 0, len(m.Parts))
			for _, p := range m.Parts {
				switch p.Kind {
				case PartText:
					parts = append(parts, wireContentPart{
						Type: "text",
						Text: p.Text,
					})
				case PartImage:
					parts = append(parts, wireContentPart{
						Type:     "image_url",
						ImageURL: &wireURLPart{URL: p.DataURL},
					})
				case PartVideo:
					parts = append(parts, wireContentPart{
						Type:     "video_url",
						VideoURL: &wireURLPart{URL: p.DataURL},
					})
				case PartAudio:
					format := "wav"
					if p.MIMEType == "audio/mp3" || p.MIMEType == "audio/mpeg" {
						format = "mp3"
					}
					parts = append(parts, wireContentPart{
						Type: "input_audio",
						InputAudio: &wireAudioPart{
							Data:   p.DataURL,
							Format: format,
						},
					})
				case PartFile:
					fileMap := map[string]any{
						"url": p.DataURL,
					}
					if p.Filename != "" {
						fileMap["name"] = p.Filename
					}
					if p.MIMEType != "" {
						fileMap["mime_type"] = p.MIMEType
					}
					parts = append(parts, wireContentPart{
						Type: "file",
						File: fileMap,
					})
				default:
					parts = append(parts, wireContentPart{
						Type: "text",
						Text: p.Text,
					})
				}
			}
			wm.Content = parts
		}
		wireMsgs = append(wireMsgs, wm)
	}
	return wireMsgs
}

type wireChatRequest struct {
	Model       string            `json:"model"`
	Messages    []wireChatMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	Stream      bool              `json:"stream"`
}

type wireChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type wireContentPart struct {
	Type       string         `json:"type"`
	Text       string         `json:"text,omitempty"`
	ImageURL   *wireURLPart   `json:"image_url,omitempty"`
	VideoURL   *wireURLPart   `json:"video_url,omitempty"`
	InputAudio *wireAudioPart `json:"input_audio,omitempty"`
	File       map[string]any `json:"file,omitempty"`
}

type wireURLPart struct {
	URL string `json:"url"`
}

type wireAudioPart struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}
