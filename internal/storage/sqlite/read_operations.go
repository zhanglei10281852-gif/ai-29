package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

func (q *queries) GetPendingApprovalTask(ctx context.Context, runID string) (domain.ApprovalTask, error) {
	approval_task, err := scanApprovalTask(q.q.QueryRowContext(ctx, approval_taskSelect+` WHERE run_id = ? AND status = 'pending'`, runID))
	return approval_task, translateError("get pending approval_task", err)
}

func (q *queries) GetApprovalTask(ctx context.Context, id string) (domain.ApprovalTask, error) {
	approval_task, err := scanApprovalTask(q.q.QueryRowContext(ctx, approval_taskSelect+` WHERE id = ?`, id))
	return approval_task, translateError("get approval_task", err)
}

const approval_taskSelect = `SELECT id, run_id, requester_id, reviewer_id, review_queue, status, expires_at,
    resolved_at, resolution_note, version, created_at, updated_at FROM approval_tasks`

func scanApprovalTask(row scanner) (domain.ApprovalTask, error) {
	var approval_task domain.ApprovalTask
	var status, expiresAt, createdAt, updatedAt string
	var resolvedAt sql.NullString
	if err := row.Scan(&approval_task.ID, &approval_task.InferenceRunID, &approval_task.RequesterID, &approval_task.ReviewerID,
		&approval_task.ReviewQueue, &status, &expiresAt, &resolvedAt, &approval_task.ResolutionNote,
		&approval_task.Version, &createdAt, &updatedAt); err != nil {
		return domain.ApprovalTask{}, err
	}
	approval_task.Status = domain.ApprovalTaskStatus(status)
	var err error
	if approval_task.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return domain.ApprovalTask{}, err
	}
	if approval_task.ResolvedAt, err = parseNullableTime(resolvedAt); err != nil {
		return domain.ApprovalTask{}, err
	}
	if approval_task.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.ApprovalTask{}, err
	}
	if approval_task.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.ApprovalTask{}, err
	}
	return approval_task, nil
}

func (q *queries) GetActiveDriftIncident(ctx context.Context, runID string) (domain.DriftIncident, error) {
	drift_incident, err := scanDriftIncident(q.q.QueryRowContext(ctx, drift_incidentSelect+` WHERE run_id = ? AND status IN ('open', 'reviewing')`, runID))
	return drift_incident, translateError("get active drift_incident", err)
}

func (q *queries) GetDriftIncident(ctx context.Context, id string) (domain.DriftIncident, error) {
	drift_incident, err := scanDriftIncident(q.q.QueryRowContext(ctx, drift_incidentSelect+` WHERE id = ?`, id))
	return drift_incident, translateError("get drift_incident", err)
}

const drift_incidentSelect = `SELECT id, run_id, status, first_observation_at, last_observation_at,
    minimum_score_millis, maximum_score_millis, observation_count, review_due_at, version, created_at, updated_at FROM drift_incidents`

func scanDriftIncident(row scanner) (domain.DriftIncident, error) {
	var drift_incident domain.DriftIncident
	var status, firstObservationAt, lastObservationAt, reviewDueAt, createdAt, updatedAt string
	if err := row.Scan(&drift_incident.ID, &drift_incident.InferenceRunID, &status, &firstObservationAt, &lastObservationAt,
		&drift_incident.Minimum, &drift_incident.Maximum, &drift_incident.ObservationCount, &reviewDueAt,
		&drift_incident.Version, &createdAt, &updatedAt); err != nil {
		return domain.DriftIncident{}, err
	}
	drift_incident.Status = domain.DriftIncidentStatus(status)
	var err error
	if drift_incident.FirstObservationAt, err = parseTime(firstObservationAt); err != nil {
		return domain.DriftIncident{}, err
	}
	if drift_incident.LastObservationAt, err = parseTime(lastObservationAt); err != nil {
		return domain.DriftIncident{}, err
	}
	if drift_incident.ReviewDueAt, err = parseTime(reviewDueAt); err != nil {
		return domain.DriftIncident{}, err
	}
	if drift_incident.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.DriftIncident{}, err
	}
	if drift_incident.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.DriftIncident{}, err
	}
	return drift_incident, nil
}

func (q *queries) GetIdempotency(ctx context.Context, scope, key string) (repository.IdempotencyRecord, error) {
	row := q.q.QueryRowContext(ctx, `SELECT scope, idempotency_key, request_hash, response_code, response_body, expires_at, created_at
        FROM idempotency_records WHERE scope = ? AND idempotency_key = ?`, scope, key)
	var record repository.IdempotencyRecord
	var expiresAt, createdAt string
	if err := row.Scan(&record.Scope, &record.Key, &record.RequestHash, &record.ResponseCode, &record.ResponseBody, &expiresAt, &createdAt); err != nil {
		return repository.IdempotencyRecord{}, translateError("get idempotency record", err)
	}
	var err error
	if record.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return repository.IdempotencyRecord{}, err
	}
	if record.CreatedAt, err = parseTime(createdAt); err != nil {
		return repository.IdempotencyRecord{}, err
	}
	record.ResponseBody = append([]byte(nil), record.ResponseBody...)
	return record, nil
}

func (q *queries) CountDataZoneInferenceRunsForBusinessDay(ctx context.Context, data_zoneID string, startAt, endAt time.Time) (int, error) {
	var count int
	err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM inference_runs
		WHERE source_zone_id = ? AND scheduled_start_at >= ? AND scheduled_start_at < ? AND state != 'cancelled'`,
		data_zoneID, formatTime(startAt), formatTime(endAt)).Scan(&count)
	if err != nil {
		return 0, translateError("count data_zone inference_runs", err)
	}
	return count, nil
}

func (q *queries) ListInferenceRuns(ctx context.Context, filter repository.InferenceRunFilter) (repository.InferenceRunPage, error) {
	page := filter.Page.Normalize(200)
	where, args := buildInferenceRunWhere(filter)
	var total int
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM inference_runs`+where, args...).Scan(&total); err != nil {
		return repository.InferenceRunPage{}, translateError("count inference_runs", err)
	}
	sortColumn := runSortColumn(page.Sort)
	direction := " ASC"
	if page.Desc {
		direction = " DESC"
	}
	query := runSelect + where + ` ORDER BY ` + sortColumn + direction + `, id ASC LIMIT ? OFFSET ?`
	rows, err := q.q.QueryContext(ctx, query, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return repository.InferenceRunPage{}, translateError("list inference_runs", err)
	}
	defer rows.Close()
	items := make([]domain.InferenceRun, 0, page.Limit)
	for rows.Next() {
		run, err := scanInferenceRun(rows)
		if err != nil {
			return repository.InferenceRunPage{}, fmt.Errorf("scan run: %w", err)
		}
		items = append(items, run)
	}
	if err := rows.Err(); err != nil {
		return repository.InferenceRunPage{}, fmt.Errorf("iterate inference_runs: %w", err)
	}
	return repository.InferenceRunPage{Items: items, Total: total}, nil
}

func buildInferenceRunWhere(filter repository.InferenceRunFilter) (string, []any) {
	clauses := make([]string, 0, 6)
	args := make([]any, 0, 6)
	appendStringFilter := func(column, value string) {
		if value != "" {
			clauses = append(clauses, column+" = ?")
			args = append(args, value)
		}
	}
	appendStringFilter("workspace_id", filter.WorkspaceID)
	appendStringFilter("source_zone_id", filter.SourceZoneID)
	appendStringFilter("target_zone_id", filter.TargetZoneID)
	appendStringFilter("state", string(filter.State))
	if filter.From != nil {
		clauses = append(clauses, "scheduled_start_at >= ?")
		args = append(args, formatTime(*filter.From))
	}
	if filter.To != nil {
		clauses = append(clauses, "scheduled_start_at < ?")
		args = append(args, formatTime(*filter.To))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func runSortColumn(value string) string {
	switch value {
	case "expected_finish_at":
		return "expected_finish_at"
	case "updated_at":
		return "updated_at"
	case "reference":
		return "reference"
	default:
		return "scheduled_start_at"
	}
}

func (q *queries) ListSnapshots(ctx context.Context, filter repository.SnapshotFilter) (repository.SnapshotPage, error) {
	page := filter.Page.Normalize(200)
	where, args := buildSnapshotWhere(filter)
	var total int
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM dataset_snapshots`+where, args...).Scan(&total); err != nil {
		return repository.SnapshotPage{}, translateError("count dataset_snapshots", err)
	}
	query := snapshotSelect + where + ` ORDER BY expires_at ASC, id ASC LIMIT ? OFFSET ?`
	rows, err := q.q.QueryContext(ctx, query, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return repository.SnapshotPage{}, translateError("list dataset_snapshots", err)
	}
	defer rows.Close()
	items := make([]domain.DatasetSnapshot, 0, page.Limit)
	for rows.Next() {
		batch, err := scanSnapshot(rows)
		if err != nil {
			return repository.SnapshotPage{}, fmt.Errorf("scan snapshot: %w", err)
		}
		items = append(items, batch.Clone())
	}
	if err := rows.Err(); err != nil {
		return repository.SnapshotPage{}, fmt.Errorf("iterate dataset_snapshots: %w", err)
	}
	return repository.SnapshotPage{Items: items, Total: total}, nil
}

func buildSnapshotWhere(filter repository.SnapshotFilter) (string, []any) {
	clauses := make([]string, 0, 5)
	args := make([]any, 0, 5)
	values := []struct{ column, value string }{
		{"workspace_id", filter.WorkspaceID}, {"source_zone_id", filter.DataZoneID}, {"run_id", filter.InferenceRunID}, {"state", string(filter.State)},
	}
	for _, item := range values {
		if item.value != "" {
			clauses = append(clauses, item.column+" = ?")
			args = append(args, item.value)
		}
	}
	if filter.ExpiresBy != nil {
		clauses = append(clauses, "expires_at <= ?")
		args = append(args, formatTime(*filter.ExpiresBy))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (q *queries) ListDriftIncidents(ctx context.Context, filter repository.DriftIncidentFilter) (repository.DriftIncidentPage, error) {
	page := filter.Page.Normalize(200)
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if filter.InferenceRunID != "" {
		clauses = append(clauses, "run_id = ?")
		args = append(args, filter.InferenceRunID)
	}
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.DueBefore != nil {
		clauses = append(clauses, "review_due_at <= ?")
		args = append(args, formatTime(*filter.DueBefore))
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	var total int
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM drift_incidents`+where, args...).Scan(&total); err != nil {
		return repository.DriftIncidentPage{}, translateError("count drift_incidents", err)
	}
	rows, err := q.q.QueryContext(ctx, drift_incidentSelect+where+` ORDER BY review_due_at ASC, id ASC LIMIT ? OFFSET ?`, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return repository.DriftIncidentPage{}, translateError("list drift_incidents", err)
	}
	defer rows.Close()
	items := make([]domain.DriftIncident, 0, page.Limit)
	for rows.Next() {
		drift_incident, err := scanDriftIncident(rows)
		if err != nil {
			return repository.DriftIncidentPage{}, fmt.Errorf("scan drift_incident: %w", err)
		}
		items = append(items, drift_incident)
	}
	if err := rows.Err(); err != nil {
		return repository.DriftIncidentPage{}, fmt.Errorf("iterate drift_incidents: %w", err)
	}
	return repository.DriftIncidentPage{Items: items, Total: total}, nil
}

func (q *queries) ListAuditEvents(ctx context.Context, filter repository.AuditFilter) (repository.AuditPage, error) {
	page := filter.Page.Normalize(500)
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	values := []struct{ column, value string }{
		{"entity_type", filter.EntityType}, {"entity_id", filter.EntityID}, {"actor", filter.Actor}, {"request_id", filter.RequestID},
	}
	for _, item := range values {
		if item.value != "" {
			clauses = append(clauses, item.column+" = ?")
			args = append(args, item.value)
		}
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	var total int
	if err := q.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`+where, args...).Scan(&total); err != nil {
		return repository.AuditPage{}, translateError("count audit events", err)
	}
	rows, err := q.q.QueryContext(ctx, `SELECT id, request_id, actor, action, entity_type, entity_id, outcome, metadata_json, created_at
        FROM audit_events`+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return repository.AuditPage{}, translateError("list audit events", err)
	}
	defer rows.Close()
	items := make([]domain.AuditEvent, 0, page.Limit)
	for rows.Next() {
		var event domain.AuditEvent
		var metadataJSON, createdAt string
		if err := rows.Scan(&event.ID, &event.RequestID, &event.Actor, &event.Action, &event.EntityType,
			&event.EntityID, &event.Outcome, &metadataJSON, &createdAt); err != nil {
			return repository.AuditPage{}, fmt.Errorf("scan audit event: %w", err)
		}
		metadata, err := decodeMetadata(metadataJSON)
		if err != nil {
			return repository.AuditPage{}, err
		}
		event.Metadata = metadata
		if event.CreatedAt, err = parseTime(createdAt); err != nil {
			return repository.AuditPage{}, err
		}
		items = append(items, event.Clone())
	}
	if err := rows.Err(); err != nil {
		return repository.AuditPage{}, fmt.Errorf("iterate audit events: %w", err)
	}
	return repository.AuditPage{Items: items, Total: total}, nil
}

func beginningOfUTCDate(day string) (time.Time, error) {
	return time.Parse("2006-01-02", day)
}
