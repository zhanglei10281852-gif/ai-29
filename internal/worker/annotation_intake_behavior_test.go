package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/clock"
	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

func TestRunOnceCompletesPlannedRunPayload(t *testing.T) {
	worker, store, ctx, now := workerFixture(t)
	payload, err := json.Marshal(domain.InferenceRun{ID: "run_planned_payload", Reference: "PLANNED-PAYLOAD"})
	if err != nil {
		t.Fatal(err)
	}
	job := domain.OutboxJob{
		ID: "job_planned_payload", Kind: "inference_run_planned", AggregateID: "run_planned_payload",
		Payload: payload, Status: domain.JobPending, MaxAttempts: 5,
		AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.WithTx(ctx, func(tx repository.Tx) error { return tx.InsertJob(ctx, job) }); err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.WithTx(ctx, func(tx repository.Tx) error {
		jobs, err := tx.ClaimJobs(ctx, now.Add(10*time.Second), 10)
		if err != nil {
			return err
		}
		if len(jobs) != 0 {
			t.Fatalf("completed planned event was reclaimed: %+v", jobs)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledPassDoesNotCompleteClaimedJob(t *testing.T) {
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	tx := &cancellationTx{
		claimed: make(chan struct{}),
		release: make(chan struct{}),
	}
	worker := New(&cancellationStore{tx: tx}, clock.NewFixed(now), time.Second, 1, nil)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- worker.RunOnce(ctx)
	}()

	select {
	case <-tx.claimed:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not enter job claim")
	}
	cancel()
	close(tx.release)

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled worker error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled worker did not return")
	}
	if tx.completed {
		t.Fatal("cancelled worker completed the claimed job")
	}
}

type cancellationStore struct {
	repository.Store
	tx *cancellationTx
}

func (s *cancellationStore) WithTx(_ context.Context, fn func(repository.Tx) error) error {
	return fn(s.tx)
}

type cancellationTx struct {
	repository.Tx
	claimed   chan struct{}
	release   chan struct{}
	completed bool
}

func (tx *cancellationTx) ExpireApprovalTasks(context.Context, time.Time, int) ([]domain.ApprovalTask, error) {
	return nil, nil
}

func (tx *cancellationTx) ClaimJobs(context.Context, time.Time, int) ([]domain.OutboxJob, error) {
	close(tx.claimed)
	<-tx.release
	return []domain.OutboxJob{{
		ID: "job_cancelled_pass", Kind: "inference_run_planned", AggregateID: "run_cancelled_pass",
		Payload: []byte(`"run_cancelled_pass"`), Status: domain.JobRunning, Attempts: 1, MaxAttempts: 5,
	}}, nil
}

func (tx *cancellationTx) RetryJob(ctx context.Context, _ string, _ time.Time, _ string, _ bool) error {
	return ctx.Err()
}

func (tx *cancellationTx) CompleteJob(context.Context, string, time.Time) error {
	tx.completed = true
	return nil
}
