package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/clock"
	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

type Worker struct {
	store    repository.Store
	clock    clock.Clock
	interval time.Duration
	batch    int
	logger   *slog.Logger
}

func New(store repository.Store, c clock.Clock, interval time.Duration, batch int, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{store: store, clock: c, interval: interval, batch: batch, logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.RunOnce(ctx); err != nil {
		w.logger.Error("worker initial pass failed", "error", err)
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				w.logger.Error("worker pass failed", "error", err)
			}
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := w.clock.Now()
	return w.store.WithTx(ctx, func(tx repository.Tx) error {
		expired, err := tx.ExpireApprovalTasks(ctx, now, w.batch)
		if err != nil {
			return err
		}
		if len(expired) > 0 {
			w.logger.Info("expired approval tasks", "count", len(expired))
		}
		jobs, err := tx.ClaimJobs(ctx, now, w.batch)
		if err != nil {
			return err
		}
		for _, job := range jobs {
			if err := w.processJob(ctx, tx, job); err != nil {
				dead := job.Attempts >= job.MaxAttempts
				backoff := time.Duration(job.Attempts*job.Attempts) * time.Second
				if retryErr := tx.RetryJob(ctx, job.ID, now.Add(backoff), err.Error(), dead); retryErr != nil {
					return fmt.Errorf("retry job %s: %w", job.ID, retryErr)
				}
				w.logger.Warn("outbox job failed", "job_id", job.ID, "attempt", job.Attempts, "dead", dead, "error", err)
				continue
			}
			if err := tx.CompleteJob(ctx, job.ID, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (w *Worker) processJob(ctx context.Context, tx repository.Tx, job domain.OutboxJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch job.Kind {
	case "inference_run_planned", "drift_incident_review":
		if len(job.Payload) == 0 {
			return fmt.Errorf("decode %s job: payload is required", job.Kind)
		}
		var marker string
		if err := json.Unmarshal(job.Payload, &marker); err != nil {
			return fmt.Errorf("decode %s job: %w", job.Kind, err)
		}
		if marker == "" {
			return fmt.Errorf("decode %s job: marker is required", job.Kind)
		}
		return nil
	default:
		return fmt.Errorf("unsupported job kind %q", job.Kind)
	}
}
