package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
)

type scanner interface {
	Scan(dest ...any) error
}

func (q *queries) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	row := q.q.QueryRowContext(ctx, userSelect+` WHERE email = ? COLLATE NOCASE`, strings.TrimSpace(email))
	user, err := scanUser(row)
	return user, translateError("get user by email", err)
}

func (q *queries) GetUser(ctx context.Context, id string) (domain.User, error) {
	user, err := scanUser(q.q.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id))
	return user, translateError("get user", err)
}

const userSelect = `SELECT id, email, display_name, password_hash, role, status, version, created_at, updated_at FROM users`

func scanUser(row scanner) (domain.User, error) {
	var user domain.User
	var role, status, createdAt, updatedAt string
	if err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &role, &status, &user.Version, &createdAt, &updatedAt); err != nil {
		return domain.User{}, err
	}
	var err error
	if user.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.User{}, err
	}
	if user.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.User{}, err
	}
	user.Role = domain.Role(role)
	user.Status = domain.UserStatus(status)
	return user, nil
}

func (q *queries) GetSessionByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error) {
	row := q.q.QueryRowContext(ctx, `SELECT id, user_id, token_hash, expires_at, created_at, revoked_at FROM sessions WHERE token_hash = ?`, tokenHash)
	var session domain.Session
	var expiresAt, createdAt string
	var revokedAt sql.NullString
	if err := row.Scan(&session.ID, &session.UserID, &session.TokenHash, &expiresAt, &createdAt, &revokedAt); err != nil {
		return domain.Session{}, translateError("get session", err)
	}
	var err error
	if session.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return domain.Session{}, err
	}
	if session.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Session{}, err
	}
	if session.RevokedAt, err = parseNullableTime(revokedAt); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (q *queries) GetWorkspace(ctx context.Context, id string) (domain.Workspace, error) {
	row := q.q.QueryRowContext(ctx, `SELECT id, code, name, status, minimum_score_millis, maximum_score_millis,
        max_execution_seconds, review_deadline_seconds, business_timezone, version, created_at, updated_at
        FROM workspaces WHERE id = ?`, id)
	var workspace domain.Workspace
	var status, createdAt, updatedAt string
	var maxExecutionSeconds, reviewDeadlineSeconds int64
	if err := row.Scan(&workspace.ID, &workspace.Code, &workspace.Name, &status, &workspace.Score.Minimum,
		&workspace.Score.Maximum, &maxExecutionSeconds, &reviewDeadlineSeconds, &workspace.BusinessTimezone,
		&workspace.Version, &createdAt, &updatedAt); err != nil {
		return domain.Workspace{}, translateError("get workspace", err)
	}
	workspace.Status = domain.WorkspaceStatus(status)
	workspace.MaxExecution = durationSeconds(maxExecutionSeconds)
	workspace.ReviewDeadline = durationSeconds(reviewDeadlineSeconds)
	var err error
	if workspace.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Workspace{}, err
	}
	if workspace.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Workspace{}, err
	}
	return workspace, nil
}

func (q *queries) GetDataZone(ctx context.Context, id string) (domain.DataZone, error) {
	row := q.q.QueryRowContext(ctx, `SELECT id, code, name, timezone, status, daily_limit, cutoff_hour, version, created_at, updated_at FROM data_zones WHERE id = ?`, id)
	var data_zone domain.DataZone
	var status, createdAt, updatedAt string
	if err := row.Scan(&data_zone.ID, &data_zone.Code, &data_zone.Name, &data_zone.Timezone, &status, &data_zone.DailyLimit, &data_zone.CutoffHour, &data_zone.Version, &createdAt, &updatedAt); err != nil {
		return domain.DataZone{}, translateError("get data_zone", err)
	}
	data_zone.Status = domain.DataZoneStatus(status)
	var err error
	if data_zone.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.DataZone{}, err
	}
	if data_zone.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.DataZone{}, err
	}
	return data_zone, nil
}

func (q *queries) GetDatasetSnapshot(ctx context.Context, id string) (domain.DatasetSnapshot, error) {
	batch, err := scanSnapshot(q.q.QueryRowContext(ctx, snapshotSelect+` WHERE id = ?`, id))
	return batch, translateError("get snapshot batch", err)
}

const snapshotSelect = `SELECT id, workspace_id, source_zone_id, source_revision, schema_family, partition_count, estimated_rows,
    state, expires_at, COALESCE(run_id, ''), quarantine_note, version, created_at, updated_at FROM dataset_snapshots`

func scanSnapshot(row scanner) (domain.DatasetSnapshot, error) {
	var batch domain.DatasetSnapshot
	var state, expiresAt, createdAt, updatedAt string
	if err := row.Scan(&batch.ID, &batch.WorkspaceID, &batch.SourceZoneID, &batch.SourceRevision, &batch.SchemaFamily,
		&batch.PartitionCount, &batch.EstimatedRows, &state, &expiresAt, &batch.InferenceRunID, &batch.QuarantineNote,
		&batch.Version, &createdAt, &updatedAt); err != nil {
		return domain.DatasetSnapshot{}, err
	}
	batch.State = domain.SnapshotState(state)
	var err error
	if batch.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return domain.DatasetSnapshot{}, err
	}
	if batch.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.DatasetSnapshot{}, err
	}
	if batch.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.DatasetSnapshot{}, err
	}
	return batch, nil
}

func (q *queries) GetComputePool(ctx context.Context, id string) (domain.ComputePool, error) {
	row := q.q.QueryRowContext(ctx, `SELECT id, serial_number, state, capacity_rows, attestation_due_at, last_reconciled_at,
        COALESCE(reserved_run_id, ''), version, created_at, updated_at FROM compute_pools WHERE id = ?`, id)
	compute_pool, err := scanComputePool(row)
	return compute_pool, translateError("get compute_pool", err)
}

func scanComputePool(row scanner) (domain.ComputePool, error) {
	var compute_pool domain.ComputePool
	var state, attestationDueAt, lastReconciledAt, createdAt, updatedAt string
	if err := row.Scan(&compute_pool.ID, &compute_pool.SerialNumber, &state, &compute_pool.CapacityRows,
		&attestationDueAt, &lastReconciledAt, &compute_pool.ReservedRunID, &compute_pool.Version, &createdAt, &updatedAt); err != nil {
		return domain.ComputePool{}, err
	}
	compute_pool.State = domain.ComputePoolState(state)
	var err error
	if compute_pool.AttestationDueAt, err = parseTime(attestationDueAt); err != nil {
		return domain.ComputePool{}, err
	}
	if compute_pool.LastReconciledAt, err = parseTime(lastReconciledAt); err != nil {
		return domain.ComputePool{}, err
	}
	if compute_pool.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.ComputePool{}, err
	}
	if compute_pool.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.ComputePool{}, err
	}
	return compute_pool, nil
}

func (q *queries) GetInferenceRun(ctx context.Context, id string) (domain.InferenceRun, error) {
	run, err := scanInferenceRun(q.q.QueryRowContext(ctx, runSelect+` WHERE id = ?`, id))
	return run, translateError("get run", err)
}

const runSelect = `SELECT id, workspace_id, source_zone_id, target_zone_id, compute_pool_id, reference, state,
    scheduled_start_at, expected_finish_at, started_at, completed_at, archived_at, total_estimated_rows, version, created_at, updated_at FROM inference_runs`

func scanInferenceRun(row scanner) (domain.InferenceRun, error) {
	var run domain.InferenceRun
	var state, scheduledStartAt, expectedFinishAt, createdAt, updatedAt string
	var startedAt, completedAt, archivedAt sql.NullString
	if err := row.Scan(&run.ID, &run.WorkspaceID, &run.SourceZoneID, &run.TargetZoneID,
		&run.ComputePoolID, &run.Reference, &state, &scheduledStartAt, &expectedFinishAt,
		&startedAt, &completedAt, &archivedAt, &run.TotalEstimatedRows, &run.Version,
		&createdAt, &updatedAt); err != nil {
		return domain.InferenceRun{}, err
	}
	run.State = domain.InferenceRunState(state)
	var err error
	if run.ScheduledStartAt, err = parseTime(scheduledStartAt); err != nil {
		return domain.InferenceRun{}, err
	}
	if run.ExpectedFinishAt, err = parseTime(expectedFinishAt); err != nil {
		return domain.InferenceRun{}, err
	}
	if run.StartedAt, err = parseNullableTime(startedAt); err != nil {
		return domain.InferenceRun{}, err
	}
	if run.CompletedAt, err = parseNullableTime(completedAt); err != nil {
		return domain.InferenceRun{}, err
	}
	if run.ArchivedAt, err = parseNullableTime(archivedAt); err != nil {
		return domain.InferenceRun{}, err
	}
	if run.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.InferenceRun{}, err
	}
	if run.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.InferenceRun{}, err
	}
	return run, nil
}

func (q *queries) ListInferenceRunInputs(ctx context.Context, runID string) ([]domain.DatasetSnapshot, error) {
	rows, err := q.q.QueryContext(ctx, `SELECT dataset_snapshots.id, dataset_snapshots.workspace_id, dataset_snapshots.source_zone_id,
        dataset_snapshots.source_revision, dataset_snapshots.schema_family, dataset_snapshots.partition_count, dataset_snapshots.estimated_rows,
        dataset_snapshots.state, dataset_snapshots.expires_at, COALESCE(dataset_snapshots.run_id, ''), dataset_snapshots.quarantine_note,
        dataset_snapshots.version, dataset_snapshots.created_at, dataset_snapshots.updated_at
		FROM dataset_snapshots JOIN inference_run_inputs ri ON ri.snapshot_id = dataset_snapshots.id
		WHERE ri.run_id = ? ORDER BY ri.added_at, dataset_snapshots.id`, runID)
	if err != nil {
		return nil, translateError("list run items", err)
	}
	defer rows.Close()
	items := make([]domain.DatasetSnapshot, 0)
	for rows.Next() {
		batch, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("scan run item: %w", err)
		}
		items = append(items, batch.Clone())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run items: %w", err)
	}
	return items, nil
}

func decodeMetadata(raw string) (map[string]string, error) {
	metadata := make(map[string]string)
	if raw == "" || raw == "{}" {
		return metadata, nil
	}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("decode audit metadata: %w", err)
	}
	return metadata, nil
}
