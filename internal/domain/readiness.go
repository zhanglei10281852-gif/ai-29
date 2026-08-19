package domain

import "time"

type RunReadiness struct {
	InferenceRunID            string            `json:"run_id"`
	InferenceRunState         InferenceRunState `json:"run_state"`
	ExpectedSnapshotCount     int               `json:"expected_snapshot_count"`
	MaterializedSnapshotCount int               `json:"materialized_snapshot_count"`
	ApprovedSnapshotCount     int               `json:"approved_snapshot_count"`
	RejectedSnapshotCount     int               `json:"rejected_snapshot_count"`
	QuarantinedCount          int               `json:"quarantined_count"`
	PendingApprovalTask       bool              `json:"pending_approval_task"`
	OpenDriftIncident         bool              `json:"open_drift_incident"`
	LastObservationAt         *time.Time        `json:"last_observation_at,omitempty"`
	Complete                  bool              `json:"complete"`
	Blockers                  []string          `json:"blockers"`
}

func (r RunReadiness) Clone() RunReadiness {
	clone := r
	clone.Blockers = append([]string(nil), r.Blockers...)
	if r.LastObservationAt != nil {
		value := *r.LastObservationAt
		clone.LastObservationAt = &value
	}
	return clone
}

func (r *RunReadiness) Evaluate() {
	r.Blockers = r.Blockers[:0]
	if r.PendingApprovalTask {
		r.Blockers = append(r.Blockers, "pending approval task")
	}
	if r.OpenDriftIncident {
		r.Blockers = append(r.Blockers, "open quality drift incident")
	}
	if r.QuarantinedCount > 0 {
		r.Blockers = append(r.Blockers, "quarantined snapshots require review")
	}
	if r.ExpectedSnapshotCount == 0 {
		r.Blockers = append(r.Blockers, "run has no dataset snapshots")
	}
	if r.MaterializedSnapshotCount < r.ExpectedSnapshotCount && r.InferenceRunState == InferenceRunCompleted {
		r.Blockers = append(r.Blockers, "not all snapshots are materialized")
	}
	r.Complete = len(r.Blockers) == 0 && (r.InferenceRunState == InferenceRunArchived || r.InferenceRunState == InferenceRunCompleted)
}
