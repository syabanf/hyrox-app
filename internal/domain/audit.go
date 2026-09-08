package domain

import "time"

// AuditEvent records who changed what. Every financial and access-control
// action writes one, and the rows are append-only: the audit trail is
// worthless if it can be edited.
type AuditEvent struct {
	ID         string `json:"id"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	Action     string `json:"action"`
	// PreviousValue and NewValue are rendered strings rather than structured
	// diffs, so the trail stays readable even after a schema change.
	PreviousValue *string   `json:"previousValue"`
	NewValue      *string   `json:"newValue"`
	ActorID       string    `json:"actorId"`
	ActorName     string    `json:"actorName"`
	Reason        *string   `json:"reason"`
	CreatedAt     time.Time `json:"createdAt"`
}
