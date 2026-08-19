package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

func (q *queries) InsertUser(ctx context.Context, user domain.User) error {
	if err := user.Validate(); err != nil {
		return err
	}
	_, err := q.q.ExecContext(ctx, `INSERT INTO users(id, email, display_name, password_hash, role, status, version, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, user.ID, user.Email, user.DisplayName, user.PasswordHash, user.Role,
		user.Status, user.Version, formatTime(user.CreatedAt), formatTime(user.UpdatedAt))
	return translateError("insert user", err)
}

func (q *queries) InsertSession(ctx context.Context, session domain.Session) error {
	_, err := q.q.ExecContext(ctx, `INSERT INTO sessions(id, user_id, token_hash, expires_at, created_at, revoked_at)
        VALUES(?, ?, ?, ?, ?, NULL)`, session.ID, session.UserID, session.TokenHash, formatTime(session.ExpiresAt), formatTime(session.CreatedAt))
	return translateError("insert session", err)
}

func (q *queries) RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error {
	result, err := q.q.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, formatTime(revokedAt), sessionID)
	if err != nil {
		return translateError("revoke session", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke session rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("revoke session: %w", domain.ErrNotFound)
	}
	return nil
}

func (q *queries) InsertWorkspace(ctx context.Context, workspace domain.Workspace) error {
	if err := workspace.Validate(); err != nil {
		return err
	}
	_, err := q.q.ExecContext(ctx, `INSERT INTO workspaces(id, code, name, status, minimum_score_millis, maximum_score_millis,
        max_execution_seconds, review_deadline_seconds, business_timezone, version, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, workspace.ID, workspace.Code, workspace.Name, workspace.Status,
		workspace.Score.Minimum, workspace.Score.Maximum, int64(workspace.MaxExecution/time.Second),
		int64(workspace.ReviewDeadline/time.Second), workspace.BusinessTimezone, workspace.Version,
		formatTime(workspace.CreatedAt), formatTime(workspace.UpdatedAt))
	return translateError("insert workspace", err)
}

func (q *queries) UpdateWorkspace(ctx context.Context, workspace domain.Workspace, expectedVersion int64) error {
	if err := workspace.Validate(); err != nil {
		return err
	}
	result, err := q.q.ExecContext(ctx, `UPDATE workspaces SET status = ?, version = version + 1, updated_at = ? WHERE id = ? AND version = ?`, workspace.Status, formatTime(workspace.UpdatedAt), workspace.ID, expectedVersion)
	if err != nil {
		return translateError("update workspace", err)
	}
	return expectVersion(result, "update workspace")
}

func (q *queries) InsertDataZone(ctx context.Context, data_zone domain.DataZone) error {
	if err := data_zone.Validate(); err != nil {
		return err
	}
	_, err := q.q.ExecContext(ctx, `INSERT INTO data_zones(id, code, name, timezone, status, daily_limit, cutoff_hour, version, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, data_zone.ID, data_zone.Code, data_zone.Name, data_zone.Timezone, data_zone.Status,
		data_zone.DailyLimit, data_zone.CutoffHour, data_zone.Version, formatTime(data_zone.CreatedAt), formatTime(data_zone.UpdatedAt))
	return translateError("insert data_zone", err)
}

func (q *queries) InsertDatasetSnapshot(ctx context.Context, batch domain.DatasetSnapshot) error {
	if err := batch.Validate(); err != nil {
		return err
	}
	runID := nullableString(batch.InferenceRunID)
	_, err := q.q.ExecContext(ctx, `INSERT INTO dataset_snapshots(id, workspace_id, source_zone_id, source_revision, schema_family,
        partition_count, estimated_rows, state, expires_at, run_id, quarantine_note, version, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, batch.ID, batch.WorkspaceID, batch.SourceZoneID,
		batch.SourceRevision, batch.SchemaFamily, batch.PartitionCount, batch.EstimatedRows, batch.State,
		formatTime(batch.ExpiresAt), runID, batch.QuarantineNote, batch.Version,
		formatTime(batch.CreatedAt), formatTime(batch.UpdatedAt))
	return translateError("insert snapshot batch", err)
}

func (q *queries) UpdateDatasetSnapshot(ctx context.Context, batch domain.DatasetSnapshot, expectedVersion int64) error {
	if err := batch.Validate(); err != nil {
		return err
	}
	result, err := q.q.ExecContext(ctx, `UPDATE dataset_snapshots SET state = ?, run_id = ?, quarantine_note = ?,
        expires_at = ?, version = version + 1, updated_at = ? WHERE id = ? AND version = ?`, batch.State,
		nullableString(batch.InferenceRunID), batch.QuarantineNote, formatTime(batch.ExpiresAt), formatTime(batch.UpdatedAt),
		batch.ID, expectedVersion)
	if err != nil {
		return translateError("update snapshot batch", err)
	}
	return expectVersion(result, "update snapshot batch")
}

func (q *queries) InsertComputePool(ctx context.Context, compute_pool domain.ComputePool) error {
	if err := compute_pool.Validate(); err != nil {
		return err
	}
	_, err := q.q.ExecContext(ctx, `INSERT INTO compute_pools(id, serial_number, state, capacity_rows, attestation_due_at,
        last_reconciled_at, reserved_run_id, version, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		compute_pool.ID, compute_pool.SerialNumber, compute_pool.State, compute_pool.CapacityRows,
		formatTime(compute_pool.AttestationDueAt), formatTime(compute_pool.LastReconciledAt), nullableString(compute_pool.ReservedRunID),
		compute_pool.Version, formatTime(compute_pool.CreatedAt), formatTime(compute_pool.UpdatedAt))
	return translateError("insert compute_pool", err)
}

func (q *queries) UpdateComputePool(ctx context.Context, compute_pool domain.ComputePool, expectedVersion int64) error {
	if err := compute_pool.Validate(); err != nil {
		return err
	}
	result, err := q.q.ExecContext(ctx, `UPDATE compute_pools SET state = ?, capacity_rows = ?, attestation_due_at = ?,
        last_reconciled_at = ?, reserved_run_id = ?, version = version + 1, updated_at = ? WHERE id = ? AND version = ?`,
		compute_pool.State, compute_pool.CapacityRows, formatTime(compute_pool.AttestationDueAt), formatTime(compute_pool.LastReconciledAt),
		nullableString(compute_pool.ReservedRunID), formatTime(compute_pool.UpdatedAt), compute_pool.ID, expectedVersion)
	if err != nil {
		return translateError("update compute_pool", err)
	}
	return expectVersion(result, "update compute_pool")
}

func (q *queries) InsertInferenceRun(ctx context.Context, run domain.InferenceRun) error {
	if err := run.Validate(); err != nil {
		return err
	}
	_, err := q.q.ExecContext(ctx, `INSERT INTO inference_runs(id, workspace_id, source_zone_id, target_zone_id, compute_pool_id,
        reference, state, scheduled_start_at, expected_finish_at, started_at, completed_at, archived_at,
        total_estimated_rows, version, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.WorkspaceID, run.SourceZoneID, run.TargetZoneID, run.ComputePoolID,
		run.Reference, run.State, formatTime(run.ScheduledStartAt), formatTime(run.ExpectedFinishAt),
		nullableTime(run.StartedAt), nullableTime(run.CompletedAt), nullableTime(run.ArchivedAt),
		run.TotalEstimatedRows, run.Version, formatTime(run.CreatedAt), formatTime(run.UpdatedAt))
	return translateError("insert run", err)
}

func (q *queries) UpdateInferenceRun(ctx context.Context, run domain.InferenceRun, expectedVersion int64) error {
	if err := run.Validate(); err != nil {
		return err
	}
	result, err := q.q.ExecContext(ctx, `UPDATE inference_runs SET state = ?, started_at = ?, completed_at = ?, archived_at = ?,
        version = version + 1, updated_at = ? WHERE id = ? AND version = ?`, run.State,
		nullableTime(run.StartedAt), nullableTime(run.CompletedAt), nullableTime(run.ArchivedAt),
		formatTime(run.UpdatedAt), run.ID, expectedVersion)
	if err != nil {
		return translateError("update run", err)
	}
	return expectVersion(result, "update run")
}

func (q *queries) InsertInferenceRunInput(ctx context.Context, item domain.InferenceRunInput) error {
	_, err := q.q.ExecContext(ctx, `INSERT INTO inference_run_inputs(run_id, snapshot_id, added_at) VALUES(?, ?, ?)`,
		item.InferenceRunID, item.SnapshotID, formatTime(item.AddedAt))
	return translateError("insert run item", err)
}

func (q *queries) DeleteInferenceRunInputs(ctx context.Context, runID string) error {
	_, err := q.q.ExecContext(ctx, `DELETE FROM inference_run_inputs WHERE run_id = ?`, runID)
	return translateError("delete run items", err)
}

func (q *queries) InsertApprovalTask(ctx context.Context, approval_task domain.ApprovalTask) error {
	if err := approval_task.Validate(); err != nil {
		return err
	}
	_, err := q.q.ExecContext(ctx, `INSERT INTO approval_tasks(id, run_id, requester_id, reviewer_id,
        review_queue, status, expires_at, resolved_at, resolution_note, version, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, approval_task.ID, approval_task.InferenceRunID, approval_task.RequesterID,
		approval_task.ReviewerID, approval_task.ReviewQueue, approval_task.Status, formatTime(approval_task.ExpiresAt), nullableTime(approval_task.ResolvedAt),
		approval_task.ResolutionNote, approval_task.Version, formatTime(approval_task.CreatedAt), formatTime(approval_task.UpdatedAt))
	return translateError("insert approval_task", err)
}

func (q *queries) UpdateApprovalTask(ctx context.Context, approval_task domain.ApprovalTask, expectedVersion int64) error {
	if err := approval_task.Validate(); err != nil {
		return err
	}
	result, err := q.q.ExecContext(ctx, `UPDATE approval_tasks SET status = ?, resolved_at = ?, resolution_note = ?,
        version = version + 1, updated_at = ? WHERE id = ? AND version = ?`, approval_task.Status,
		nullableTime(approval_task.ResolvedAt), approval_task.ResolutionNote, formatTime(approval_task.UpdatedAt), approval_task.ID, expectedVersion)
	if err != nil {
		return translateError("update approval_task", err)
	}
	return expectVersion(result, "update approval_task")
}

func (q *queries) InsertObservation(ctx context.Context, observation domain.QualityObservation) error {
	if err := observation.Validate(); err != nil {
		return err
	}
	_, err := q.q.ExecContext(ctx, `INSERT INTO quality_observations(id, run_id, metric_key, sequence,
        score_millis, recorded_at, received_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, observation.ID,
		observation.InferenceRunID, observation.MetricKey, observation.Sequence, observation.Score,
		formatTime(observation.RecordedAt), formatTime(observation.ReceivedAt))
	return translateError("insert score observation", err)
}

func (q *queries) InsertDriftIncident(ctx context.Context, drift_incident domain.DriftIncident) error {
	_, err := q.q.ExecContext(ctx, `INSERT INTO drift_incidents(id, run_id, status, first_observation_at, last_observation_at,
        minimum_score_millis, maximum_score_millis, observation_count, review_due_at, version, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, drift_incident.ID, drift_incident.InferenceRunID, drift_incident.Status,
		formatTime(drift_incident.FirstObservationAt), formatTime(drift_incident.LastObservationAt), drift_incident.Minimum, drift_incident.Maximum,
		drift_incident.ObservationCount, formatTime(drift_incident.ReviewDueAt), drift_incident.Version,
		formatTime(drift_incident.CreatedAt), formatTime(drift_incident.UpdatedAt))
	return translateError("insert drift_incident", err)
}

func (q *queries) UpdateDriftIncident(ctx context.Context, drift_incident domain.DriftIncident, expectedVersion int64) error {
	result, err := q.q.ExecContext(ctx, `UPDATE drift_incidents SET status = ?, first_observation_at = ?, last_observation_at = ?,
        minimum_score_millis = ?, maximum_score_millis = ?, observation_count = ?, review_due_at = ?,
        version = version + 1, updated_at = ? WHERE id = ? AND version = ?`, drift_incident.Status,
		formatTime(drift_incident.FirstObservationAt), formatTime(drift_incident.LastObservationAt), drift_incident.Minimum, drift_incident.Maximum,
		drift_incident.ObservationCount, formatTime(drift_incident.ReviewDueAt), formatTime(drift_incident.UpdatedAt), drift_incident.ID, expectedVersion)
	if err != nil {
		return translateError("update drift_incident", err)
	}
	return expectVersion(result, "update drift_incident")
}

func (q *queries) InsertRiskDecision(ctx context.Context, decision domain.RiskDecision) error {
	_, err := q.q.ExecContext(ctx, `INSERT INTO review_decisions(id, drift_incident_id, risk_reviewer, decision, rationale, created_at)
        VALUES(?, ?, ?, ?, ?, ?)`, decision.ID, decision.DriftIncidentID, decision.Reviewer, decision.Decision,
		decision.Rationale, formatTime(decision.CreatedAt))
	return translateError("insert review decision", err)
}

func (q *queries) InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	_, err = q.q.ExecContext(ctx, `INSERT INTO audit_events(id, request_id, actor, action, entity_type, entity_id,
        outcome, metadata_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.RequestID,
		event.Actor, event.Action, event.EntityType, event.EntityID, event.Outcome, string(metadata), formatTime(event.CreatedAt))
	return translateError("insert audit event", err)
}

func (q *queries) PutIdempotency(ctx context.Context, record repository.IdempotencyRecord) error {
	result, err := q.q.ExecContext(ctx, `INSERT INTO idempotency_records(scope, idempotency_key, request_hash,
		response_code, response_body, expires_at, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope, idempotency_key) DO UPDATE SET request_hash = excluded.request_hash,
		response_code = excluded.response_code, response_body = excluded.response_body,
		expires_at = excluded.expires_at, created_at = excluded.created_at
		WHERE idempotency_records.expires_at <= excluded.created_at`, record.Scope,
		record.Key, record.RequestHash, record.ResponseCode, append([]byte(nil), record.ResponseBody...),
		formatTime(record.ExpiresAt), formatTime(record.CreatedAt))
	if err != nil {
		return translateError("put idempotency record", err)
	}
	return expectVersion(result, "put idempotency record")
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}
