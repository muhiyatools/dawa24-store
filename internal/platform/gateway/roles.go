package gateway

import "os"

// Role names a job the assistant needs done. The mapping to a Gateway model is
// configuration, and lives here because model identifiers must not appear
// outside this package (AGENTS.md R2).
type Role string

const (
	RolePrimary    Role = "assistant.primary"    // conversation, images, video
	RoleAttachment Role = "assistant.attachment" // documents and audio understanding
	RoleTranscribe Role = "assistant.transcribe" // speech to text

	// Capability roles. Before these existed the domain capabilities ran on
	// whatever model the call site happened to pass, which meant an operator
	// could not point them anywhere and a cost surprise had no dial to turn.
	RoleMatching Role = "matching.adjudicate"   // batch product-match adjudication
	RoleColumns  Role = "import.detect_columns" // spreadsheet header detection
	RoleExpand   Role = "search.expand_query"   // search synonym expansion
	RoleClassify Role = "support.classify"      // support ticket triage
)

// defaultRoleModels is the fallback when the operator has not overridden a role.
//
// The assistant runs on gemma-4-31b-it, and that was measured rather than
// guessed. Asked a question that needs data, with one tool offered:
//
//	gemma-4-31b-it   finish_reason=tool_calls   spend_summary({"from":…})
//	qwen3.7-flash    finish_reason=length       no tool call, no answer
//
// The previous default reproduced, on demand, the exact failure the production
// logs showed: a turn that spent its whole output budget and returned nothing.
//
// It also reads images, despite the catalogue publishing supports_vision=false
// for it — asked the colour of a red test image it answered "أحمر". That flag
// is operator-maintained metadata and is simply not filled in on this Gateway,
// which is why nothing in this application treats it as a veto any more.
var defaultRoleModels = map[Role]string{
	RolePrimary: "gemma-4-31b-it",
	// The attachment reader is the same model. A separate one existed to cover
	// modalities the primary lacked; the primary lacks none that were tested,
	// and two models mean two sets of behaviour to keep true.
	RoleAttachment: "gemma-4-31b-it",
	// whisper-large-v3-turbo is active on this Gateway. whisper-1, the previous
	// default, is seeded there as INACTIVE — so voice input answered 404 on
	// every deployment that did not override it.
	RoleTranscribe: "whisper-large-v3-turbo",

	RoleMatching: "qwen3.7-flash",
	RoleColumns:  "qwen3.7-flash",
	RoleExpand:   "qwen3.7-flash",
	RoleClassify: "qwen3.7-flash",
}

// resolveRoleModel returns the concrete Gateway model identifier for a given role.
func resolveRoleModel(role Role) string {
	var envKey string
	switch role {
	case RolePrimary:
		envKey = "GATEWAY_MODEL_ASSISTANT_PRIMARY"
	case RoleAttachment:
		envKey = "GATEWAY_MODEL_ASSISTANT_ATTACHMENT"
	case RoleTranscribe:
		envKey = "GATEWAY_MODEL_ASSISTANT_TRANSCRIBE"
	case RoleMatching:
		envKey = "GATEWAY_MODEL_MATCHING"
	case RoleColumns:
		envKey = "GATEWAY_MODEL_COLUMNS"
	case RoleExpand:
		envKey = "GATEWAY_MODEL_EXPAND"
	case RoleClassify:
		envKey = "GATEWAY_MODEL_CLASSIFY"
	}
	if envKey != "" {
		if override := os.Getenv(envKey); override != "" {
			return override
		}
	}
	if model, ok := defaultRoleModels[role]; ok {
		return model
	}
	return string(role)
}
