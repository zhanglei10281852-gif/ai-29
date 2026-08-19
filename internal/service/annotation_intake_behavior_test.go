package service

import (
	"errors"
	"testing"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

func TestDailyLimitSeparatesAdjacentLocalBusinessDays(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	source, err := f.services.Catalog.CreateDataZone(ctx, domain.DataZone{
		Code: "DST-SOURCE", Name: "DST source", Timezone: "America/New_York", DailyLimit: 1, CutoffHour: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := f.services.Catalog.CreateComputePool(ctx, domain.ComputePool{
		SerialNumber: "DST-POOL", CapacityRows: 1000,
		AttestationDueAt: time.Date(2027, 3, 16, 0, 0, 0, 0, time.UTC), LastReconciledAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.services.Catalog.RegisterSnapshot(ctx, domain.DatasetSnapshot{
		WorkspaceID: f.workspace.ID, SourceZoneID: source.ID, SourceRevision: "DST-REV",
		SchemaFamily: "regional-policy", PartitionCount: 1, EstimatedRows: 100,
		ExpiresAt: time.Date(2027, 3, 16, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = f.services.Catalog.ValidateSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	nextLocalDay := time.Date(2027, 3, 15, 4, 30, 0, 0, time.UTC)
	existing := domain.InferenceRun{
		ID: "run_dst_next_day", WorkspaceID: f.workspace.ID, SourceZoneID: source.ID,
		TargetZoneID: f.destination.ID, ComputePoolID: pool.ID, Reference: "DST-NEXT-DAY",
		State: domain.InferenceRunQueued, ScheduledStartAt: nextLocalDay,
		ExpectedFinishAt: nextLocalDay.Add(time.Hour), TotalEstimatedRows: 10,
		Version: 1, CreatedAt: f.clock.Now(), UpdatedAt: f.clock.Now(),
	}
	if err := f.store.WithTx(ctx, func(tx repository.Tx) error { return tx.InsertInferenceRun(ctx, existing) }); err != nil {
		t.Fatal(err)
	}
	currentLocalDay := time.Date(2027, 3, 14, 16, 0, 0, 0, time.UTC)
	created, err := f.services.Inference.PlanInferenceRun(ctx, PlanInferenceRunInput{
		WorkspaceID: f.workspace.ID, SourceZoneID: source.ID, TargetZoneID: f.destination.ID,
		ComputePoolID: pool.ID, Reference: "DST-CURRENT-DAY", SnapshotIDs: []string{snapshot.ID},
		ScheduledStartAt: currentLocalDay, ExpectedFinishAt: currentLocalDay.Add(time.Hour),
		IdempotencyKey: "dst-current-day-key",
	})
	if err != nil {
		t.Fatalf("plan on preceding local business day: %v", err)
	}
	if created.ID == "" {
		t.Fatal("plan returned an empty run")
	}
}

func TestPlanningUsesTotalSnapshotRowsForCapacity(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	pool, err := f.services.Catalog.CreateComputePool(ctx, domain.ComputePool{
		SerialNumber: "TOTAL-ROWS-POOL", CapacityRows: 500,
		AttestationDueAt: f.clock.Now().Add(48 * time.Hour), LastReconciledAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.services.Catalog.RegisterSnapshot(ctx, domain.DatasetSnapshot{
		WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, SourceRevision: "TOTAL-ROWS-REV",
		SchemaFamily: "partitioned-policy", PartitionCount: 6, EstimatedRows: 600,
		ExpiresAt: f.clock.Now().Add(48 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = f.services.Catalog.ValidateSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.services.Inference.PlanInferenceRun(ctx, PlanInferenceRunInput{
		WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID,
		ComputePoolID: pool.ID, Reference: "TOTAL-ROWS-RUN", SnapshotIDs: []string{snapshot.ID},
		ScheduledStartAt: f.clock.Now().Add(time.Hour), ExpectedFinishAt: f.clock.Now().Add(2 * time.Hour),
		IdempotencyKey: "total-rows-run-key",
	})
	if !errors.Is(err, domain.ErrCapacityExceeded) {
		t.Fatalf("planning error = %v, want capacity exceeded", err)
	}
}

func TestIdempotencyKeyIncludesExpectedFinishAt(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	input := PlanInferenceRunInput{
		WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID,
		ComputePoolID: f.compute_pool.ID, Reference: "RANK-V42", SnapshotIDs: []string{f.batch.ID},
		ScheduledStartAt: f.clock.Now().Add(time.Hour), ExpectedFinishAt: f.clock.Now().Add(2 * time.Hour),
		IdempotencyKey: "finish-time-key",
	}
	first, err := f.services.Inference.PlanInferenceRun(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedFinishAt = input.ExpectedFinishAt.Add(time.Hour)
	second, err := f.services.Inference.PlanInferenceRun(ctx, input)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed finish time error = %v, want conflict (run=%+v)", err, second)
	}
	if err := f.store.Read(ctx, func(reader repository.Reader) error {
		page, err := reader.ListInferenceRuns(ctx, repository.InferenceRunFilter{
			Page: repository.PageRequest{Limit: 10}, WorkspaceID: f.workspace.ID,
		})
		if err != nil {
			return err
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != first.ID || !page.Items[0].ExpectedFinishAt.Equal(input.ExpectedFinishAt.Add(-time.Hour)) {
			t.Fatalf("runs after changed idempotency request = %+v", page)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDriftIncidentKeepsTelemetryEventTimes(t *testing.T) {
	f := newServiceFixture(t)
	run := f.planAndStart(t, "RUN-EVENT-TIMES")
	firstRecordedAt := f.clock.Now().Add(-2 * time.Hour)
	_, incident, err := f.services.Metrics.RecordObservation(f.as(f.data_engineer), RecordObservationInput{
		InferenceRunID: run.ID, MetricKey: "delayed-policy", Sequence: 1,
		Score: 12000, RecordedAt: firstRecordedAt,
	})
	if err != nil || incident == nil {
		t.Fatalf("first observation incident=%+v error=%v", incident, err)
	}
	f.clock.Advance(5 * time.Minute)
	lastRecordedAt := firstRecordedAt.Add(45 * time.Minute)
	_, incident, err = f.services.Metrics.RecordObservation(f.as(f.data_engineer), RecordObservationInput{
		InferenceRunID: run.ID, MetricKey: "delayed-policy", Sequence: 2,
		Score: 13000, RecordedAt: lastRecordedAt,
	})
	if err != nil || incident == nil {
		t.Fatalf("second observation incident=%+v error=%v", incident, err)
	}
	if !incident.FirstObservationAt.Equal(firstRecordedAt) || !incident.LastObservationAt.Equal(lastRecordedAt) {
		t.Fatalf("incident event window = %s..%s, want %s..%s",
			incident.FirstObservationAt, incident.LastObservationAt, firstRecordedAt, lastRecordedAt)
	}
}

func TestExecutionWindowRejectsRunBeyondConfiguredLimit(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	workspace, err := f.services.Catalog.CreateWorkspace(ctx, domain.Workspace{
		Code: "SHORT-RUNS", Name: "Short execution workspace", Score: f.workspace.Score,
		MaxExecution: time.Hour, ReviewDeadline: f.workspace.ReviewDeadline, BusinessTimezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = f.services.Catalog.ActivateWorkspace(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.services.Catalog.RegisterSnapshot(ctx, domain.DatasetSnapshot{
		WorkspaceID: workspace.ID, SourceZoneID: f.origin.ID, SourceRevision: "SHORT-RUN-INPUT",
		SchemaFamily: "agent-policy", PartitionCount: 1, EstimatedRows: 100,
		ExpiresAt: f.clock.Now().Add(6 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = f.services.Catalog.ValidateSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := f.clock.Now().Add(time.Hour)
	_, err = f.services.Inference.PlanInferenceRun(ctx, PlanInferenceRunInput{
		WorkspaceID: workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID,
		ComputePoolID: f.compute_pool.ID, Reference: "RUN-TOO-LONG", SnapshotIDs: []string{snapshot.ID},
		ScheduledStartAt: start, ExpectedFinishAt: start.Add(time.Hour + 59*time.Minute),
		IdempotencyKey: "run-too-long-key",
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("plan error = %v, want conflict", err)
	}
	if err := f.store.Read(ctx, func(reader repository.Reader) error {
		runs, err := reader.ListInferenceRuns(ctx, repository.InferenceRunFilter{
			Page: repository.PageRequest{Limit: 10}, WorkspaceID: workspace.ID,
		})
		if err != nil {
			return err
		}
		if runs.Total != 0 || len(runs.Items) != 0 {
			t.Fatalf("rejected plan persisted runs: %+v", runs)
		}
		storedSnapshot, err := reader.GetDatasetSnapshot(ctx, snapshot.ID)
		if err != nil {
			return err
		}
		if storedSnapshot.State != domain.SnapshotValidated || storedSnapshot.InferenceRunID != "" {
			t.Fatalf("rejected plan reserved snapshot: %+v", storedSnapshot)
		}
		pool, err := reader.GetComputePool(ctx, f.compute_pool.ID)
		if err != nil {
			return err
		}
		if pool.State != domain.ComputePoolAvailable || pool.ReservedRunID != "" {
			t.Fatalf("rejected plan reserved compute pool: %+v", pool)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
