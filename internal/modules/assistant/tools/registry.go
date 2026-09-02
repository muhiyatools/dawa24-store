// Package tools is the only path from something a model said to something the
// database reads.
//
// The whole security posture of the assistant rests on one property of this
// package: a tool call is data, not an instruction. What arrives from the model
// is a name and a blob of JSON, both of which a prompt injection, a poisoned
// product description, or a user simply asking can control. Nothing in that
// blob decides who the caller is, which organisation is read, or whether the
// call is allowed. Those come from the request's own authenticated session,
// every time, after the model has spoken.
//
// Concretely, Dispatch always runs the same sequence and there is no other way
// in:
//
//  1. the caller is read from the request context   (never from arguments)
//  2. the assistant gate permission is re-checked   (owner's per-role switch)
//  3. the tool is looked up by exact name           (no fuzzy matching)
//  4. the tool's dashboard scope must match         (pharmacy ≠ vendor ≠ admin)
//  5. the caller must hold the tool's permission    (the same key the screen needs)
//  6. arguments are decoded strictly                (unknown fields are refused)
//  7. handle arguments are verified and bound       (forgery and enumeration die here)
//  8. the call runs under its own timeout
//  9. results are truncated to a row and byte ceiling
//  10. the decision is written to the audit log, allowed or denied
//
// A tool added tomorrow inherits all ten by existing. That is the point of the
// registry: there is no per-tool place to forget a check.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Decision is the outcome recorded for one tool call.
type Decision string

const (
	DecisionAllowed    Decision = "allowed"
	DecisionGate       Decision = "denied_gate"
	DecisionScope      Decision = "denied_scope"
	DecisionPermission Decision = "denied_permission"
	DecisionHandle     Decision = "denied_handle"
	DecisionInvalid    Decision = "invalid_args"
	DecisionUnknown    Decision = "unknown_tool"
	DecisionFailed     Decision = "failed"
)

// maxResultBytes caps what one tool call may add to the conversation.
//
// Every byte here is re-sent on every later turn, so an unbounded result is not
// just one large prompt but a permanently larger one. Six kilobytes is roughly
// a page of dense tabular data — enough to answer, small enough to carry.
const maxResultBytes = 6 << 10

// Result is what a tool hands back to the model.
type Result struct {
	// Data is marshalled to JSON and becomes the tool message content.
	Data any
	// Note is a short sentence for the model when there is nothing to return,
	// or when a refusal needs explaining. It is model-facing, never shown raw.
	Note string
	// Rows is how many records Data holds, for the audit log.
	Rows int
}

// Handler runs one tool. It receives the live actor; it must never take an
// identity from args.
type Handler func(ctx context.Context, actor authctx.Actor, args json.RawMessage) (Result, error)

// Tool is one read-only capability offered to a model.
type Tool struct {
	Name        string
	Description string
	// Params is the JSON Schema shown to the model. Build it with objectSchema
	// so additionalProperties:false is never forgotten.
	Params map[string]any
	// Scopes are the dashboards this tool belongs to. A tool with no scope is
	// unreachable, which is the correct default for a mistake.
	Scopes []rbac.Scope
	// Permissions are the dashboard permission keys, any one of which admits
	// the caller. These are the SAME keys the corresponding screen is gated on,
	// so the assistant can never show an employee something they could not
	// already open themselves.
	Permissions []string
	Timeout     time.Duration
	Handler     Handler
}

// AuditSink records tool decisions. Failures to record are logged and dropped:
// the audit trail must not be able to fail a read.
type AuditSink interface {
	RecordToolCall(ctx context.Context, entry assistant.ToolAudit)
}

var _ assistant.ToolRunner = (*Registry)(nil)

// Registry holds every tool and performs dispatch.
type Registry struct {
	byName map[string]Tool
	order  []string
	signer *handles.Signer
	reader assistant.Reader
	audit  AuditSink
	log    *slog.Logger
}

// NewRegistry builds the registry and declares every tool.
func NewRegistry(reader assistant.Reader, signer *handles.Signer, audit AuditSink, log *slog.Logger) *Registry {
	if log == nil {
		log = slog.Default()
	}
	r := &Registry{
		byName: make(map[string]Tool),
		signer: signer,
		reader: reader,
		audit:  audit,
		log:    log.With("component", "capsule_tools"),
	}
	r.declare(sharedTools(r)...)
	r.declare(pharmacyTools(r)...)
	r.declare(vendorTools(r)...)
	r.declare(adminTools(r)...)
	return r
}

func (r *Registry) declare(tools ...Tool) {
	for _, t := range tools {
		if _, exists := r.byName[t.Name]; exists {
			panic("assistant tools: duplicate tool name " + t.Name)
		}
		if t.Timeout == 0 {
			t.Timeout = 12 * time.Second
		}
		r.byName[t.Name] = t
		r.order = append(r.order, t.Name)
	}
}

// Names returns every declared tool name, in declaration order. Used by tests
// that assert schema hygiene across the whole registry.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Lookup returns a declared tool. Reported for tests and diagnostics.
func (r *Registry) Lookup(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// Schemas returns the tools this caller may use.
//
// Filtering here is not the security boundary — Dispatch re-checks everything —
// but it matters for two other reasons. A model cannot be talked into calling a
// tool it was never shown, and a prompt that lists twenty irrelevant tools
// produces worse answers than one that lists the six that apply.
func (r *Registry) Schemas(actor authctx.Actor) []gateway.ToolSpec {
	scope := actor.DashboardScope()
	if scope == "" {
		return nil
	}
	if _, ok := assistant.Allowed(actor); !ok {
		return nil
	}
	out := make([]gateway.ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		t := r.byName[name]
		if !scopeAllows(t.Scopes, scope) || !actor.CanAny(t.Permissions...) {
			continue
		}
		out = append(out, gateway.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Params,
		})
	}
	return out
}

func scopeAllows(scopes []rbac.Scope, want rbac.Scope) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

// Dispatch runs one tool call from the model.
//
// It never returns an error: a refusal is an answer the model must be able to
// read and work around ("you cannot see that"), not a transport failure. What
// it does return is always safe to put in a prompt — no SQL, no stack, no
// column names, no other tenant's anything.
func (r *Registry) Dispatch(ctx context.Context, actor authctx.Actor, turnID int64, call gateway.ToolCall) assistant.ToolOutcome {
	started := time.Now()
	name := strings.TrimSpace(call.Name)

	record := func(d Decision, permission string, rows int) {
		if r.audit == nil {
			return
		}
		r.audit.RecordToolCall(ctx, assistant.ToolAudit{
			TurnID:         turnID,
			OrganizationID: actor.OrgID,
			UserID:         actor.UserID,
			AgentRole:      string(actor.DashboardScope()),
			ToolName:       name,
			Decision:       string(d),
			Permission:     permission,
			LatencyMS:      int(time.Since(started).Milliseconds()),
			RowCount:       rows,
		})
	}

	deny := func(d Decision, permission, note string) assistant.ToolOutcome {
		record(d, permission, 0)
		if d != DecisionInvalid {
			r.log.WarnContext(ctx, "assistant tool refused",
				"tool", name, "decision", d, "user_id", actor.UserID,
				"org_id", actor.OrgID, "scope", actor.DashboardScope())
		}
		return assistant.ToolOutcome{
			CallID: call.ID, Name: name, Decision: string(d), Content: encodeNote(note),
		}
	}

	// 1–2. Who is calling, and may they use the assistant at all.
	cfg, allowed := assistant.Allowed(actor)
	if !allowed {
		return deny(DecisionGate, "",
			"لا يملك المستخدم صلاحية استخدام المساعد الذكي.")
	}

	// 3. Exact name. Never guess a near match: a model that misremembers a
	// tool must be corrected, not helpfully redirected to a different one.
	tool, ok := r.byName[name]
	if !ok {
		return deny(DecisionUnknown, "",
			"لا توجد أداة بهذا الاسم. استخدم فقط الأدوات المتاحة لك.")
	}

	// 4. Dashboard scope.
	if !scopeAllows(tool.Scopes, cfg.Role) {
		return deny(DecisionScope, "",
			"هذه الأداة ليست ضمن لوحة تحكم هذا المستخدم.")
	}

	// 5. The dashboard permission the equivalent screen requires.
	if !actor.CanAny(tool.Permissions...) {
		return deny(DecisionPermission, strings.Join(tool.Permissions, ","),
			"هذه البيانات خارج صلاحيات المستخدم الحالي.")
	}

	// 6. Arguments. An empty argument string is a valid no-argument call.
	args := json.RawMessage(strings.TrimSpace(call.Arguments))
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	if !json.Valid(args) {
		return deny(DecisionInvalid, "", "صيغة المعطيات غير صالحة.")
	}

	// 7–8. Run, bounded.
	runCtx, cancel := context.WithTimeout(ctx, tool.Timeout)
	defer cancel()

	res, err := tool.Handler(runCtx, actor, args)
	if err != nil {
		switch {
		case errors.Is(err, errBadArgs):
			return deny(DecisionInvalid, "", err.Error())
		case errors.Is(err, errBadHandle):
			return deny(DecisionHandle, "",
				"المرجع المستخدم غير صالح أو لا يخص هذا الحساب.")
		default:
			// The underlying error may name a table or a column. It goes to the
			// log; the model gets a sentence.
			r.log.ErrorContext(ctx, "assistant tool failed",
				"tool", name, "user_id", actor.UserID, "error", err)
			return deny(DecisionFailed, "", "تعذّر قراءة البيانات المطلوبة.")
		}
	}

	// 9. Shape.
	content := encodeResult(res)
	record(DecisionAllowed, strings.Join(tool.Permissions, ","), res.Rows)
	return assistant.ToolOutcome{
		CallID:   call.ID,
		Name:     name,
		Decision: string(DecisionAllowed),
		Rows:     res.Rows,
		Content:  content,
	}
}

func encodeNote(note string) string {
	b, err := json.Marshal(map[string]string{"error": note})
	if err != nil {
		return `{"error":"refused"}`
	}
	return string(b)
}

func encodeResult(res Result) string {
	payload := map[string]any{}
	if res.Data != nil {
		payload["data"] = res.Data
	}
	if res.Note != "" {
		payload["note"] = res.Note
	}
	if len(payload) == 0 {
		payload["note"] = "لا توجد نتائج مطابقة."
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return `{"error":"تعذّر تجهيز النتيجة."}`
	}
	out := strings.TrimSpace(buf.String())
	if len(out) > maxResultBytes {
		// Truncating JSON would produce something the model reads as corrupt,
		// so say what happened instead and let it narrow the question.
		return fmt.Sprintf(
			`{"note":"النتيجة أكبر من الحد المسموح (%d سطر). ضيّق نطاق البحث أو حدّد فترة أقصر."}`,
			res.Rows)
	}
	return out
}
