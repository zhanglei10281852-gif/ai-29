package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
)

func (q *queries) InsertJob(ctx context.Context, job domain.OutboxJob) error {
	_, err := q.q.ExecContext(ctx, `INSERT INTO outbox_jobs(id, kind, aggregate_id, payload, status, attempts,
        max_attempts, available_at, locked_at, last_error, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, job.Kind, job.AggregateID, append([]byte(nil), job.Payload...),
		job.Status, job.Attempts, job.MaxAttempts, formatTime(job.AvailableAt), nullableTime(job.LockedAt),
		job.LastError, formatTime(job.CreatedAt), formatTime(job.UpdatedAt))
	return translateError("insert outbox job", err)
}

func (q *queries) ClaimJobs(ctx context.Context, now time.Time, limit int) ([]domain.OutboxJob, error) {
	rows, err := q.q.QueryContext(ctx, `SELECT id, kind, aggregate_id, payload, status, attempts, max_attempts,
        available_at, locked_at, last_error, created_at, updated_at FROM outbox_jobs
        WHERE status IN ('pending', 'failed') AND available_at <= ? ORDER BY created_at ASC LIMIT ?`, formatTime(now), limit)
	if err != nil {
		return nil, translateError("select outbox jobs", err)
	}
	defer rows.Close()
	jobs := make([]domain.OutboxJob, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan outbox job: %w", err)
		}
		jobs = append(jobs, job.Clone())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox jobs: %w", err)
	}
	claimed := make([]domain.OutboxJob, 0, len(jobs))
	for _, job := range jobs {
		result, err := q.q.ExecContext(ctx, `UPDATE outbox_jobs SET status = 'running', attempts = attempts + 1,
            locked_at = ?, updated_at = ? WHERE id = ? AND status IN ('pending', 'failed')`, formatTime(now), formatTime(now), job.ID)
		if err != nil {
			return nil, translateError("claim outbox job", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("claim outbox job rows affected: %w", err)
		}
		if rowsAffected == 1 {
			job.Status = domain.JobRunning
			job.Attempts++
			lockedAt := now.UTC()
			job.LockedAt = &lockedAt
			job.UpdatedAt = now.UTC()
			claimed = append(claimed, job.Clone())
		}
	}
	return claimed, nil
}

func scanJob(row scanner) (domain.OutboxJob, error) {
	var job domain.OutboxJob
	var status, availableAt, createdAt, updatedAt string
	var lockedAt sql.NullString
	if err := row.Scan(&job.ID, &job.Kind, &job.AggregateID, &job.Payload, &status, &job.Attempts,
		&job.MaxAttempts, &availableAt, &lockedAt, &job.LastError, &createdAt, &updatedAt); err != nil {
		return domain.OutboxJob{}, err
	}
	job.Status = domain.JobStatus(status)
	var err error
	if job.AvailableAt, err = parseTime(availableAt); err != nil {
		return domain.OutboxJob{}, err
	}
	if job.LockedAt, err = parseNullableTime(lockedAt); err != nil {
		return domain.OutboxJob{}, err
	}
	if job.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.OutboxJob{}, err
	}
	if job.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.OutboxJob{}, err
	}
	job.Payload = append([]byte(nil), job.Payload...)
	return job, nil
}

func (q *queries) CompleteJob(ctx context.Context, id string, now time.Time) error {
	result, err := q.q.ExecContext(ctx, `UPDATE outbox_jobs SET status = 'succeeded', locked_at = NULL,
        updated_at = ? WHERE id = ? AND status = 'running'`, formatTime(now), id)
	if err != nil {
		return translateError("complete outbox job", err)
	}
	return expectVersion(result, "complete outbox job")
}

func (q *queries) RetryJob(ctx context.Context, id string, availableAt time.Time, lastError string, dead bool) error {
	status := domain.JobFailed
	if dead {
		status = domain.JobDead
	}
	result, err := q.q.ExecContext(ctx, `UPDATE outbox_jobs SET status = ?, available_at = ?, locked_at = NULL,
        last_error = ?, updated_at = ? WHERE id = ? AND status = 'running'`, status, formatTime(availableAt),
		lastError, formatTime(time.Now().UTC()), id)
	if err != nil {
		return translateError("retry outbox job", err)
	}
	return expectVersion(result, "retry outbox job")
}

func (q *queries) ExpireApprovalTasks(ctx context.Context, now time.Time, limit int) ([]domain.ApprovalTask, error) {
	rows, err := q.q.QueryContext(ctx, approval_taskSelect+` WHERE status = 'pending' AND expires_at <= ? ORDER BY expires_at ASC LIMIT ?`, formatTime(now), limit)
	if err != nil {
		return nil, translateError("list expired approval_tasks", err)
	}
	defer rows.Close()
	approval_tasks := make([]domain.ApprovalTask, 0, limit)
	for rows.Next() {
		approval_task, err := scanApprovalTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan expired approval_task: %w", err)
		}
		approval_tasks = append(approval_tasks, approval_task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired approval_tasks: %w", err)
	}
	expired := make([]domain.ApprovalTask, 0, len(approval_tasks))
	for _, approval_task := range approval_tasks {
		expected := approval_task.Version
		if err := approval_task.Resolve(domain.ApprovalTaskExpired, "approval_task expired before acceptance", now); err != nil {
			return nil, err
		}
		result, err := q.q.ExecContext(ctx, `UPDATE approval_tasks SET status = 'expired', resolved_at = ?,
            resolution_note = ?, version = version + 1, updated_at = ? WHERE id = ? AND version = ? AND status = 'pending'`,
			formatTime(now), approval_task.ResolutionNote, formatTime(now), approval_task.ID, expected)
		if err != nil {
			return nil, translateError("expire approval_task", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("expire approval_task rows affected: %w", err)
		}
		if rowsAffected == 1 {
			expired = append(expired, approval_task)
		}
	}
	return expired, nil
}
