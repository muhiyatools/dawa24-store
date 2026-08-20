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
)

// defaultRoleModels is the fallback when the operator has not overridden a role.
var defaultRoleModels = map[Role]string{
	RolePrimary:    "qwen3.7-flash",
	RoleAttachment: "voxtral-small-24b-2507",
	RoleTranscribe: "whisper-1",
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
