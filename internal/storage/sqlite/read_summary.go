package sqlite

import (
	"context"
	"fmt"

	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

func (q *queries) GetPlatformSummary(ctx context.Context) (repository.PlatformSummary, error) {
	var summary repository.PlatformSummary
	queries := []struct {
		name   string
		target *int
		sql    string
	}{
		{"active workspaces", &summary.WorkspacesActive, `SELECT COUNT(*) FROM workspaces WHERE status = 'active'`},
		{"validated dataset snapshots", &summary.SnapshotsValidated, `SELECT COUNT(*) FROM dataset_snapshots WHERE state = 'validated'`},
		{"materializing dataset snapshots", &summary.SnapshotsMaterializing, `SELECT COUNT(*) FROM dataset_snapshots WHERE state = 'materializing'`},
		{"quarantined dataset_snapshots", &summary.SnapshotsQuarantined, `SELECT COUNT(*) FROM dataset_snapshots WHERE state = 'quarantined'`},
		{"available compute_pools", &summary.ComputePoolsAvailable, `SELECT COUNT(*) FROM compute_pools WHERE state = 'available'`},
		{"active inference runs", &summary.InferenceRunsActive, `SELECT COUNT(*) FROM inference_runs WHERE state IN ('queued', 'staged', 'running', 'completed')`},
		{"open drift_incidents", &summary.OpenDriftIncidents, `SELECT COUNT(*) FROM drift_incidents WHERE status IN ('open', 'reviewing')`},
		{"pending approval_tasks", &summary.PendingApprovalTasks, `SELECT COUNT(*) FROM approval_tasks WHERE status = 'pending'`},
		{"failed jobs", &summary.FailedJobs, `SELECT COUNT(*) FROM outbox_jobs WHERE status IN ('failed', 'dead')`},
	}
	for _, item := range queries {
		if err := q.q.QueryRowContext(ctx, item.sql).Scan(item.target); err != nil {
			return repository.PlatformSummary{}, fmt.Errorf("count %s: %w", item.name, err)
		}
	}
	return summary, nil
}
