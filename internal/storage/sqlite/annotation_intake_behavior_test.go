package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

func TestRunningJobCannotBeClaimedTwice(t *testing.T) {
	store, ctx, now := testStore(t)
	job := domain.OutboxJob{
		ID: "job_single_owner", Kind: "inference_run_planned", AggregateID: "run_single_owner",
		Payload: []byte(`"run_single_owner"`), Status: domain.JobPending, MaxAttempts: 5,
		AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.WithTx(ctx, func(tx repository.Tx) error { return tx.InsertJob(ctx, job) }); err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan claimResult, 1)
	firstCommitted := make(chan struct{})
	secondDone := make(chan claimResult, 1)
	go func() {
		firstDone <- claimOnce(ctx, store, now)
		close(firstCommitted)
	}()
	go func() {
		<-firstCommitted
		secondDone <- claimOnce(ctx, store, now.Add(time.Second))
	}()

	first := <-firstDone
	if first.err != nil {
		t.Fatal(first.err)
	}
	if len(first.jobs) != 1 || first.jobs[0].ID != job.ID {
		t.Fatalf("first worker claimed jobs = %+v", first.jobs)
	}
	if first.jobs[0].Status != domain.JobRunning || first.jobs[0].Attempts != 1 {
		t.Fatalf("first claim state = %+v", first.jobs[0])
	}

	second := <-secondDone
	if second.err != nil {
		t.Fatal(second.err)
	}
	if len(second.jobs) != 0 {
		t.Fatalf("second worker reclaimed running job: %+v", second.jobs)
	}
}

type claimResult struct {
	jobs []domain.OutboxJob
	err  error
}

func claimOnce(ctx context.Context, store *Store, now time.Time) claimResult {
	var result claimResult
	result.err = store.WithTx(ctx, func(tx repository.Tx) error {
		var err error
		result.jobs, err = tx.ClaimJobs(ctx, now, 1)
		return err
	})
	return result
}
