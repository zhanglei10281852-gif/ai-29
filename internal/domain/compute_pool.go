package domain

import (
	"strings"
	"time"
)

type ComputePoolState string

const (
	ComputePoolAvailable   ComputePoolState = "available"
	ComputePoolReserved    ComputePoolState = "reserved"
	ComputePoolAllocated   ComputePoolState = "allocated"
	ComputePoolReconciling ComputePoolState = "reconciling"
	ComputePoolRetired     ComputePoolState = "retired"
)

type ComputePool struct {
	ID               string           `json:"id"`
	SerialNumber     string           `json:"serial_number"`
	State            ComputePoolState `json:"state"`
	CapacityRows     int              `json:"capacity_rows"`
	AttestationDueAt time.Time        `json:"attestation_due_at"`
	LastReconciledAt time.Time        `json:"last_reconciled_at"`
	ReservedRunID    string           `json:"reserved_run_id,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	Version          int64            `json:"version"`
}

func (c ComputePool) Validate() error {
	if strings.TrimSpace(c.SerialNumber) == "" {
		return FieldError{Field: "serial_number", Message: "is required"}
	}
	if c.CapacityRows < 100 || c.CapacityRows > 1000000 {
		return FieldError{Field: "capacity_rows", Message: "outside supported range"}
	}
	if c.AttestationDueAt.IsZero() || c.LastReconciledAt.IsZero() {
		return FieldError{Field: "compute_pool", Message: "attestation and reconciliation timestamps are required"}
	}
	switch c.State {
	case ComputePoolAvailable, ComputePoolReserved, ComputePoolAllocated, ComputePoolReconciling, ComputePoolRetired:
		return nil
	default:
		return FieldError{Field: "compute_pool_state", Message: "is invalid"}
	}
}

func (c ComputePool) EligibleFor(plannedStart time.Time, volume int) error {
	if c.State != ComputePoolAvailable {
		return ConflictError{Resource: "compute_pool", Reason: "not available"}
	}
	if !c.IsAttestedAt(plannedStart) {
		return ConflictError{Resource: "compute_pool", Reason: "attestation expires before scheduled start"}
	}
	if c.CapacityRows < volume {
		return ErrCapacityExceeded
	}
	return nil
}

func (c ComputePool) IsAttestedAt(at time.Time) bool {
	return c.AttestationDueAt.After(at) && !c.LastReconciledAt.After(at)
}

func (c ComputePool) NeedsReconciliation(at time.Time) bool {
	return c.LastReconciledAt.IsZero() || c.LastReconciledAt.Before(at.Add(-72*time.Hour))
}

func (c *ComputePool) StartReconciliation(now time.Time) error {
	if c.State != ComputePoolAvailable {
		return TransitionError{Entity: "compute_pool", From: string(c.State), To: string(ComputePoolReconciling)}
	}
	c.State = ComputePoolReconciling
	c.UpdatedAt = now.UTC()
	return nil
}

func (c *ComputePool) CompleteReconciliation(now time.Time) error {
	if c.State != ComputePoolReconciling {
		return TransitionError{Entity: "compute_pool", From: string(c.State), To: string(ComputePoolAvailable)}
	}
	c.State = ComputePoolAvailable
	c.LastReconciledAt = now.UTC()
	c.UpdatedAt = now.UTC()
	return nil
}

func (c *ComputePool) Retire(now time.Time) error {
	if c.State != ComputePoolAvailable && c.State != ComputePoolReconciling {
		return ConflictError{Resource: "compute_pool", Reason: "active reservation must be completed before retirement"}
	}
	c.State = ComputePoolRetired
	c.ReservedRunID = ""
	c.UpdatedAt = now.UTC()
	return nil
}
