package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
)

func (q *queries) GetRunReadiness(ctx context.Context, runID string) (domain.RunReadiness, error) {
	run, err := q.GetInferenceRun(ctx, runID)
	if err != nil {
		return domain.RunReadiness{}, err
	}
	var report domain.RunReadiness
	report.InferenceRunID = run.ID
	report.InferenceRunState = run.State
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM inference_run_inputs WHERE run_id = ?`, runID).Scan(&report.ExpectedSnapshotCount); err != nil {
		return domain.RunReadiness{}, translateError("count run snapshots", err)
	}
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM inference_run_inputs ri JOIN dataset_snapshots s ON s.id = ri.snapshot_id
        WHERE ri.run_id = ? AND s.state IN ('materialized', 'approved', 'rejected', 'quarantined')`, runID).Scan(&report.MaterializedSnapshotCount); err != nil {
		return domain.RunReadiness{}, translateError("count materialized snapshots", err)
	}
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM inference_run_inputs ri JOIN dataset_snapshots s ON s.id = ri.snapshot_id
		WHERE ri.run_id = ? AND s.state = 'approved'`, runID).Scan(&report.ApprovedSnapshotCount); err != nil {
		return domain.RunReadiness{}, translateError("count approved snapshots", err)
	}
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM inference_run_inputs ri JOIN dataset_snapshots s ON s.id = ri.snapshot_id
		WHERE ri.run_id = ? AND s.state = 'rejected'`, runID).Scan(&report.RejectedSnapshotCount); err != nil {
		return domain.RunReadiness{}, translateError("count rejected snapshots", err)
	}
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM inference_run_inputs ri JOIN dataset_snapshots s ON s.id = ri.snapshot_id
		WHERE ri.run_id = ? AND s.state = 'quarantined'`, runID).Scan(&report.QuarantinedCount); err != nil {
		return domain.RunReadiness{}, translateError("count quarantined snapshots", err)
	}
	var pending int
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_tasks WHERE run_id = ? AND status = 'pending'`, runID).Scan(&pending); err != nil {
		return domain.RunReadiness{}, translateError("count pending approval_tasks", err)
	}
	report.PendingApprovalTask = pending > 0
	var open int
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM drift_incidents WHERE run_id = ? AND status IN ('open', 'reviewing')`, runID).Scan(&open); err != nil {
		return domain.RunReadiness{}, translateError("count open drift_incidents", err)
	}
	report.OpenDriftIncident = open > 0
	var lastObservation sql.NullString
	if err := q.q.QueryRowContext(ctx, `SELECT MAX(recorded_at) FROM quality_observations WHERE run_id = ?`, runID).Scan(&lastObservation); err != nil {
		return domain.RunReadiness{}, translateError("get last observation", err)
	}
	if lastObservation.Valid {
		parsed, err := parseTime(lastObservation.String)
		if err != nil {
			return domain.RunReadiness{}, err
		}
		report.LastObservationAt = &parsed
	}
	report.Evaluate()
	return report.Clone(), nil
}

func (q *queries) latestObservationAt(ctx context.Context, runID string) (time.Time, error) {
	var raw string
	if err := q.q.QueryRowContext(ctx, `SELECT recorded_at FROM quality_observations WHERE run_id = ? ORDER BY recorded_at DESC LIMIT 1`, runID).Scan(&raw); err != nil {
		return time.Time{}, translateError("get latest observation", err)
	}
	parsed, err := parseTime(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse latest observation: %w", err)
	}
	return parsed, nil
}
