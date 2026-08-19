package domain

import (
	"strings"
	"time"
)

type SnapshotState string

const (
	SnapshotRegistered    SnapshotState = "registered"
	SnapshotValidated     SnapshotState = "validated"
	SnapshotReserved      SnapshotState = "reserved"
	SnapshotMaterializing SnapshotState = "materializing"
	SnapshotMaterialized  SnapshotState = "materialized"
	SnapshotQuarantined   SnapshotState = "quarantined"
	SnapshotApproved      SnapshotState = "approved"
	SnapshotRejected      SnapshotState = "rejected"
)

type DatasetSnapshot struct {
	ID             string        `json:"id"`
	WorkspaceID    string        `json:"workspace_id"`
	SourceZoneID   string        `json:"source_zone_id"`
	SourceRevision string        `json:"source_revision"`
	SchemaFamily   string        `json:"schema_family"`
	PartitionCount int           `json:"partition_count"`
	EstimatedRows  int           `json:"estimated_rows"`
	State          SnapshotState `json:"state"`
	ExpiresAt      time.Time     `json:"expires_at"`
	InferenceRunID string        `json:"run_id,omitempty"`
	QuarantineNote string        `json:"quarantine_note,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	Version        int64         `json:"version"`
}

func (b DatasetSnapshot) Validate() error {
	if strings.TrimSpace(b.WorkspaceID) == "" || strings.TrimSpace(b.SourceZoneID) == "" {
		return FieldError{Field: "dataset_snapshot", Message: "workspace and source zone are required"}
	}
	if strings.TrimSpace(b.SourceRevision) == "" || strings.TrimSpace(b.SchemaFamily) == "" {
		return FieldError{Field: "dataset_snapshot", Message: "source revision and schema family are required"}
	}
	if b.PartitionCount < 1 || b.EstimatedRows < 1 {
		return FieldError{Field: "dataset_snapshot", Message: "partition count and estimated rows must be positive"}
	}
	if b.ExpiresAt.IsZero() {
		return FieldError{Field: "expires_at", Message: "is required"}
	}
	return validateSnapshotState(b.State)
}

func validateSnapshotState(state SnapshotState) error {
	switch state {
	case SnapshotRegistered, SnapshotValidated, SnapshotReserved, SnapshotMaterializing, SnapshotMaterialized, SnapshotQuarantined, SnapshotApproved, SnapshotRejected:
		return nil
	default:
		return FieldError{Field: "snapshot_state", Message: "is invalid"}
	}
}

func (s SnapshotState) IsTerminal() bool {
	return s == SnapshotApproved || s == SnapshotRejected
}

func (b *DatasetSnapshot) Transition(to SnapshotState, now time.Time) error {
	allowed := map[SnapshotState]map[SnapshotState]bool{
		SnapshotRegistered:    {SnapshotValidated: true, SnapshotRejected: true},
		SnapshotValidated:     {SnapshotReserved: true, SnapshotRejected: true},
		SnapshotReserved:      {SnapshotValidated: true, SnapshotMaterializing: true},
		SnapshotMaterializing: {SnapshotMaterialized: true, SnapshotQuarantined: true},
		SnapshotMaterialized:  {SnapshotApproved: true, SnapshotQuarantined: true},
		SnapshotQuarantined:   {SnapshotApproved: true, SnapshotRejected: true},
	}
	if !allowed[b.State][to] {
		return TransitionError{Entity: "dataset_snapshot", From: string(b.State), To: string(to)}
	}
	if !b.IsUsableAt(now) && to != SnapshotRejected && to != SnapshotQuarantined {
		return ConflictError{Resource: "dataset_snapshot", Reason: "expired snapshot cannot advance"}
	}
	b.State = to
	b.UpdatedAt = now.UTC()
	return nil
}

func (b DatasetSnapshot) Clone() DatasetSnapshot { return b }

func (b DatasetSnapshot) IsUsableAt(at time.Time) bool {
	return b.ExpiresAt.After(at) && b.State != SnapshotRejected
}
