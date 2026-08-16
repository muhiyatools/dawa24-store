package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// AuditEntry is one row of platform.audit_log.
//
// Before and After hold the entity state either side of the change. They are
// what makes an audit trail answer "what did this actually do", rather than
// only "something happened".
type AuditEntry struct {
	OrganizationID *int64
	ActorUserID    int64
	Action         string
	EntityType     string
	EntityID       string
	Before         any
	After          any
	RequestID      string
}

// WriteAudit records an administrative change.
//
// It takes the transaction rather than opening its own, deliberately: the audit
// row has to commit or roll back with the change it describes. Written in a
// separate transaction it can survive a rolled-back change and claim something
// happened that did not, or be lost while the change lands and leave a
// privileged mutation with no record at all.
func WriteAudit(ctx context.Context, tx pgx.Tx, e AuditEntry) error {
	beforeJSON, err := marshalAudit(e.Before)
	if err != nil {
		return fmt.Errorf("audit before: %w", err)
	}
	afterJSON, err := marshalAudit(e.After)
	if err != nil {
		return fmt.Errorf("audit after: %w", err)
	}

	const query = `
		INSERT INTO platform.audit_log (
			organization_id, actor_user_id, action, entity_type, entity_id,
			before, after, request_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''));
	`
	if _, err := tx.Exec(ctx, query,
		e.OrganizationID, e.ActorUserID, e.Action, e.EntityType, e.EntityID,
		beforeJSON, afterJSON, e.RequestID,
	); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

// marshalAudit renders a value as JSONB, or NULL when there is nothing to say —
// a creation has no before state and a deletion has no after state.
func marshalAudit(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
