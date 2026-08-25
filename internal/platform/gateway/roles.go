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
var defaultRoleModels = map[Role]string{
	RolePrimary:    "qwen3.7-flash",
	RoleAttachment: "voxtral-small-24b-2507",
	RoleTranscribe: "whisper-1",

	// The capability tiers deliberately point at the cheapest published model.
	// Adjudication runs on the long tail of an import, which is exactly where a
	// quality model turns a weekly budget into an afternoon.
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
