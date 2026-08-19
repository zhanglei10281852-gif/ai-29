package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/clock"
	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
	"github.com/zhanglei10281852-gif/ai/internal/requestmeta"
	"github.com/zhanglei10281852-gif/ai/internal/service"
	"github.com/zhanglei10281852-gif/ai/internal/storage/sqlite"
)

func workerFixture(t *testing.T) (*Worker, *sqlite.Store, context.Context, time.Time) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	fixed := clock.NewFixed(now)
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "worker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	return New(store, fixed, time.Second, 20, logger), store, ctx, now
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestRunOnceExpiresApprovalTasksAndCompletesJobs(t *testing.T) {
	worker, store, ctx, now := workerFixture(t)
	minimum, _ := domain.ScoreFromFloat(2)
	maximum, _ := domain.ScoreFromFloat(8)
	rangeValue, _ := domain.NewQualityRange(minimum, maximum)
	workspace := domain.Workspace{ID: "workspace_worker", Code: "WORKER", Name: "Worker", Status: domain.WorkspaceActive, Score: rangeValue, MaxExecution: time.Hour, ReviewDeadline: time.Hour, BusinessTimezone: "UTC", Version: 1, CreatedAt: now, UpdatedAt: now}
	origin := domain.DataZone{ID: "origin_worker", Code: "ORIGIN", Name: "Origin", Timezone: "UTC", Status: domain.DataZoneActive, DailyLimit: 10, CutoffHour: 0, Version: 1, CreatedAt: now, UpdatedAt: now}
	destination := domain.DataZone{ID: "dest_worker", Code: "DEST", Name: "Destination", Timezone: "UTC", Status: domain.DataZoneActive, DailyLimit: 10, CutoffHour: 0, Version: 1, CreatedAt: now, UpdatedAt: now}
	compute_pool := domain.ComputePool{ID: "box_worker", SerialNumber: "BOX-W", State: domain.ComputePoolAllocated, CapacityRows: 1000, AttestationDueAt: now.Add(time.Hour), LastReconciledAt: now, ReservedRunID: "ship_worker", Version: 1, CreatedAt: now, UpdatedAt: now}
	run := domain.InferenceRun{ID: "ship_worker", WorkspaceID: workspace.ID, SourceZoneID: origin.ID, TargetZoneID: destination.ID, ComputePoolID: compute_pool.ID, Reference: "SHIP-W", State: domain.InferenceRunRunning, ScheduledStartAt: now, ExpectedFinishAt: now.Add(time.Hour), TotalEstimatedRows: 1, Version: 1, CreatedAt: now, UpdatedAt: now}
	approval_task := domain.ApprovalTask{ID: "approval_task_worker", InferenceRunID: run.ID, RequesterID: "from", ReviewerID: "to", ReviewQueue: "dock", Status: domain.ApprovalTaskPending, ExpiresAt: now.Add(-time.Minute), Version: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)}
	job := domain.OutboxJob{ID: "job_worker", Kind: "inference_run_planned", AggregateID: run.ID, Payload: []byte(`{"id":"ship_worker"}`), Status: domain.JobPending, MaxAttempts: 3, AvailableAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)}
	if err := store.WithTx(ctx, func(tx repository.Tx) error {
		for _, entity := range []any{workspace, origin, destination, compute_pool, run, approval_task, job} {
			switch value := entity.(type) {
			case domain.Workspace:
				if err := tx.InsertWorkspace(ctx, value); err != nil {
					return err
				}
			case domain.DataZone:
				if err := tx.InsertDataZone(ctx, value); err != nil {
					return err
				}
			case domain.ComputePool:
				if err := tx.InsertComputePool(ctx, value); err != nil {
					return err
				}
			case domain.InferenceRun:
				if err := tx.InsertInferenceRun(ctx, value); err != nil {
					return err
				}
			case domain.ApprovalTask:
				if err := tx.InsertApprovalTask(ctx, value); err != nil {
					return err
				}
			case domain.OutboxJob:
				if err := tx.InsertJob(ctx, value); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Read(ctx, func(reader repository.Reader) error {
		approval_task, err := reader.GetApprovalTask(ctx, approval_task.ID)
		if err != nil {
			return err
		}
		if approval_task.Status != domain.ApprovalTaskExpired {
			t.Fatalf("approval_task = %+v", approval_task)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDriftReviewJobProducerConsumerContract(t *testing.T) {
	worker, store, ctx, now := workerFixture(t)
	minimum, _ := domain.ScoreFromFloat(2)
	maximum, _ := domain.ScoreFromFloat(8)
	rangeValue, _ := domain.NewQualityRange(minimum, maximum)
	workspace := domain.Workspace{ID: "workspace_contract", Code: "CONTRACT", Name: "Contract", Status: domain.WorkspaceActive, Score: rangeValue, MaxExecution: time.Hour, ReviewDeadline: time.Hour, BusinessTimezone: "UTC", Version: 1, CreatedAt: now, UpdatedAt: now}
	origin := domain.DataZone{ID: "origin_contract", Code: "ORIGIN-C", Name: "Origin", Timezone: "UTC", Status: domain.DataZoneActive, DailyLimit: 10, CutoffHour: 0, Version: 1, CreatedAt: now, UpdatedAt: now}
	destination := domain.DataZone{ID: "dest_contract", Code: "DEST-C", Name: "Destination", Timezone: "UTC", Status: domain.DataZoneActive, DailyLimit: 10, CutoffHour: 0, Version: 1, CreatedAt: now, UpdatedAt: now}
	computePool := domain.ComputePool{ID: "pool_contract", SerialNumber: "POOL-C", State: domain.ComputePoolAllocated, CapacityRows: 1000, AttestationDueAt: now.Add(time.Hour), LastReconciledAt: now, ReservedRunID: "run_contract", Version: 1, CreatedAt: now, UpdatedAt: now}
	run := domain.InferenceRun{ID: "run_contract", WorkspaceID: workspace.ID, SourceZoneID: origin.ID, TargetZoneID: destination.ID, ComputePoolID: computePool.ID, Reference: "RUN-C", State: domain.InferenceRunRunning, ScheduledStartAt: now.Add(-time.Hour), ExpectedFinishAt: now.Add(time.Hour), TotalEstimatedRows: 100, Version: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now}
	snapshot := domain.DatasetSnapshot{ID: "snapshot_contract", WorkspaceID: workspace.ID, SourceZoneID: origin.ID, SourceRevision: "REV-C", SchemaFamily: "features-v1", PartitionCount: 1, EstimatedRows: 100, State: domain.SnapshotMaterializing, ExpiresAt: now.Add(time.Hour), InferenceRunID: run.ID, Version: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now}

	if err := store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertWorkspace(ctx, workspace); err != nil {
			return err
		}
		if err := tx.InsertDataZone(ctx, origin); err != nil {
			return err
		}
		if err := tx.InsertDataZone(ctx, destination); err != nil {
			return err
		}
		if err := tx.InsertComputePool(ctx, computePool); err != nil {
			return err
		}
		if err := tx.InsertDatasetSnapshot(ctx, snapshot); err != nil {
			return err
		}
		if err := tx.InsertInferenceRun(ctx, run); err != nil {
			return err
		}
		return tx.InsertInferenceRunInput(ctx, domain.InferenceRunInput{InferenceRunID: run.ID, SnapshotID: snapshot.ID, AddedAt: now})
	}); err != nil {
		t.Fatal(err)
	}

	services := service.New(store, worker.clock, 4*time.Hour, 30*time.Minute)
	principalCtx := requestmeta.WithPrincipal(ctx, domain.Principal{UserID: "data_contract", Role: domain.RoleDataEngineer})
	_, incident, err := services.Metrics.RecordObservation(principalCtx, service.RecordObservationInput{InferenceRunID: run.ID, MetricKey: "quality-score", Sequence: 1, Score: 12000, RecordedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if incident == nil {
		t.Fatal("out-of-range observation did not open a drift incident")
	}

	if err := store.WithTx(ctx, func(tx repository.Tx) error {
		jobs, err := tx.ClaimJobs(ctx, now, 10)
		if err != nil {
			return err
		}
		if len(jobs) != 1 {
			return fmt.Errorf("claimed jobs = %d, want 1", len(jobs))
		}
		job := jobs[0]
		var marker string
		if err := json.Unmarshal(job.Payload, &marker); err != nil {
			return fmt.Errorf("decode producer payload as worker string: %w", err)
		}
		if marker != incident.ID {
			return fmt.Errorf("payload marker = %q, want %q", marker, incident.ID)
		}
		if err := worker.processJob(ctx, tx, job); err != nil {
			return err
		}
		return tx.CompleteJob(ctx, job.ID, now)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRunOnceHonorsCancellation(t *testing.T) {
	worker, _, _, _ := workerFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := worker.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
}
