package domain

import "time"

type AuditEvent struct {
	ID         string
	RequestID  string
	Actor      string
	Action     string
	EntityType string
	EntityID   string
	Outcome    string
	Metadata   map[string]string
	CreatedAt  time.Time
}

func (e AuditEvent) Clone() AuditEvent {
	clone := e
	clone.Metadata = make(map[string]string, len(e.Metadata))
	for key, value := range e.Metadata {
		clone.Metadata[key] = value
	}
	return clone
}
