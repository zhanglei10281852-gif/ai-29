package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

func testStore(t *testing.T) (*Store, context.Context, time.Time) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "featuremesh.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	return store, ctx, now
}

func seedCatalog(t *testing.T, store *Store, ctx context.Context, now time.Time) (domain.Workspace, domain.DataZone, domain.DataZone, domain.ComputePool, domain.DatasetSnapshot) {
	t.Helper()
	minimum, _ := domain.ScoreFromFloat(0.8)
	maximum, _ := domain.ScoreFromFloat(0.99)
	rangeValue, _ := domain.NewQualityRange(minimum, maximum)
	workspace := domain.Workspace{ID: "workspace_1", Code: "MESH-1", Name: "Ranking workspace", Status: domain.WorkspaceActive, Score: rangeValue, MaxExecution: 24 * time.Hour, ReviewDeadline: 4 * time.Hour, BusinessTimezone: "Asia/Shanghai", Version: 1, CreatedAt: now, UpdatedAt: now}
	origin := domain.DataZone{ID: "data_zone_1", Code: "ZONE-1", Name: "Feature source", Timezone: "Asia/Shanghai", Status: domain.DataZoneActive, DailyLimit: 10, CutoffHour: 6, Version: 1, CreatedAt: now, UpdatedAt: now}
	destination := domain.DataZone{ID: "data_zone_2", Code: "ZONE-2", Name: "Inference target", Timezone: "Asia/Shanghai", Status: domain.DataZoneActive, DailyLimit: 10, CutoffHour: 6, Version: 1, CreatedAt: now, UpdatedAt: now}
	compute_pool := domain.ComputePool{ID: "pool_1", SerialNumber: "GPU-POOL-1", State: domain.ComputePoolAvailable, CapacityRows: 1000, AttestationDueAt: now.Add(48 * time.Hour), LastReconciledAt: now, Version: 1, CreatedAt: now, UpdatedAt: now}
	batch := domain.DatasetSnapshot{ID: "snapshot_1", WorkspaceID: workspace.ID, SourceZoneID: origin.ID, SourceRevision: "REV-1", SchemaFamily: "ranking-features-v2", PartitionCount: 2, EstimatedRows: 100, State: domain.SnapshotValidated, ExpiresAt: now.Add(48 * time.Hour), Version: 1, CreatedAt: now, UpdatedAt: now}
	err := store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertWorkspace(ctx, workspace); err != nil {
			return err
		}
		if err := tx.InsertDataZone(ctx, origin); err != nil {
			return err
		}
		if err := tx.InsertDataZone(ctx, destination); err != nil {
			return err
		}
		if err := tx.InsertComputePool(ctx, compute_pool); err != nil {
			return err
		}
		return tx.InsertDatasetSnapshot(ctx, batch)
	})
	if err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	return workspace, origin, destination, compute_pool, batch
}

func TestOpenAppliesMigrationsAndEnablesForeignKeys(t *testing.T) {
	store, ctx, _ := testStore(t)
	if err := store.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	var tableCount int
	if err := store.Read(ctx, func(reader repository.Reader) error {
		_, err := reader.GetWorkspace(ctx, "missing")
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount < 15 {
		t.Fatalf("table count = %d, want at least 15", tableCount)
	}
	var foreignKeys int
	if err := store.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d", foreignKeys)
	}
}

func TestMemoryStoreKeepsOneDatabaseAcrossConcurrentCalls(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if got := store.db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("memory store max connections = %d, want 1", got)
	}

	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	minimum, _ := domain.ScoreFromFloat(2)
	maximum, _ := domain.ScoreFromFloat(8)
	rangeValue, _ := domain.NewQualityRange(minimum, maximum)
	workspace := domain.Workspace{ID: "workspace_memory", Code: "MEMORY", Name: "Memory store", Status: domain.WorkspaceActive, Score: rangeValue, MaxExecution: time.Hour, ReviewDeadline: time.Hour, BusinessTimezone: "UTC", Version: 1, CreatedAt: now, UpdatedAt: now}

	txReady := make(chan struct{})
	releaseTx := make(chan struct{})
	txDone := make(chan error, 1)
	go func() {
		txDone <- store.WithTx(ctx, func(tx repository.Tx) error {
			if err := tx.InsertWorkspace(ctx, workspace); err != nil {
				return err
			}
			close(txReady)
			<-releaseTx
			return nil
		})
	}()
	<-txReady

	readStarted := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		readDone <- store.Read(ctx, func(reader repository.Reader) error {
			close(readStarted)
			got, err := reader.GetWorkspace(ctx, workspace.ID)
			if err == nil && got.ID != workspace.ID {
				return errors.New("read returned a different workspace")
			}
			return err
		})
	}()
	<-readStarted
	close(releaseTx)
	if err := <-txDone; err != nil {
		t.Fatal(err)
	}
	if err := <-readDone; err != nil {
		t.Fatalf("concurrent read from memory store: %v", err)
	}
}

func TestTransactionRollsBackAllEntities(t *testing.T) {
	store, ctx, now := testStore(t)
	minimum, _ := domain.ScoreFromFloat(2)
	maximum, _ := domain.ScoreFromFloat(8)
	rangeValue, _ := domain.NewQualityRange(minimum, maximum)
	workspace := domain.Workspace{ID: "workspace_roll", Code: "ROLL", Name: "Rollback", Status: domain.WorkspaceActive, Score: rangeValue, MaxExecution: time.Hour, ReviewDeadline: time.Hour, BusinessTimezone: "UTC", Version: 1, CreatedAt: now, UpdatedAt: now}
	err := store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertWorkspace(ctx, workspace); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("rollback transaction returned nil")
	}
	if err := store.Read(ctx, func(reader repository.Reader) error {
		_, err := reader.GetWorkspace(ctx, workspace.ID)
		return err
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("workspace after rollback error = %v", err)
	}
}

func TestRepositoryReadsAndDeepCopiesSnapshot(t *testing.T) {
	store, ctx, now := testStore(t)
	_, origin, _, _, batch := seedCatalog(t, store, ctx, now)
	got, err := storeReadSnapshot(store, ctx, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceZoneID != origin.ID || got.State != domain.SnapshotValidated {
		t.Fatalf("snapshot = %+v", got)
	}
	got.QuarantineNote = "local mutation"
	again, err := storeReadSnapshot(store, ctx, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.QuarantineNote != "" {
		t.Fatalf("stored snapshot was mutated: %+v", again)
	}
}

func storeReadSnapshot(store *Store, ctx context.Context, id string) (domain.DatasetSnapshot, error) {
	var result domain.DatasetSnapshot
	err := store.Read(ctx, func(reader repository.Reader) error {
		var err error
		result, err = reader.GetDatasetSnapshot(ctx, id)
		return err
	})
	return result, err
}

func TestOptimisticVersionRejectsStaleUpdate(t *testing.T) {
	store, ctx, now := testStore(t)
	_, _, _, _, batch := seedCatalog(t, store, ctx, now)
	first, err := storeReadSnapshot(store, ctx, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	first.State = domain.SnapshotReserved
	first.UpdatedAt = now.Add(time.Minute)
	if err := store.WithTx(ctx, func(tx repository.Tx) error { return tx.UpdateDatasetSnapshot(ctx, first, first.Version) }); err != nil {
		t.Fatal(err)
	}
	second.State = domain.SnapshotReserved
	second.UpdatedAt = now.Add(2 * time.Minute)
	err = store.WithTx(ctx, func(tx repository.Tx) error { return tx.UpdateDatasetSnapshot(ctx, second, second.Version) })
	if !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale update error = %v", err)
	}
}

func TestInferenceRunFilterPaginationAndOrdering(t *testing.T) {
	store, ctx, now := testStore(t)
	workspace, origin, destination, compute_pool, batch := seedCatalog(t, store, ctx, now)
	secondSnapshot := batch
	secondSnapshot.ID = "snapshot_2"
	secondSnapshot.SourceRevision = "EXT-2"
	if err := store.WithTx(ctx, func(tx repository.Tx) error { return tx.InsertDatasetSnapshot(ctx, secondSnapshot) }); err != nil {
		t.Fatal(err)
	}
	inference_runs := []domain.InferenceRun{
		{ID: "ship_1", WorkspaceID: workspace.ID, SourceZoneID: origin.ID, TargetZoneID: destination.ID, ComputePoolID: compute_pool.ID, Reference: "REF-1", State: domain.InferenceRunQueued, ScheduledStartAt: now.Add(time.Hour), ExpectedFinishAt: now.Add(2 * time.Hour), TotalEstimatedRows: 100, Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "ship_2", WorkspaceID: workspace.ID, SourceZoneID: origin.ID, TargetZoneID: destination.ID, ComputePoolID: compute_pool.ID, Reference: "REF-2", State: domain.InferenceRunStaged, ScheduledStartAt: now.Add(2 * time.Hour), ExpectedFinishAt: now.Add(3 * time.Hour), TotalEstimatedRows: 100, Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	if err := store.WithTx(ctx, func(tx repository.Tx) error {
		for _, run := range inference_runs {
			if err := tx.InsertInferenceRun(ctx, run); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var page repository.InferenceRunPage
	err := store.Read(ctx, func(reader repository.Reader) error {
		var err error
		page, err = reader.ListInferenceRuns(ctx, repository.InferenceRunFilter{Page: repository.PageRequest{Limit: 1, Sort: "scheduled_start_at"}, WorkspaceID: workspace.ID})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != "ship_1" {
		t.Fatalf("page = %+v", page)
	}
}

func TestIdempotencyRecordCopiesResponse(t *testing.T) {
	store, ctx, now := testStore(t)
	record := repository.IdempotencyRecord{Scope: "scope", Key: "key", RequestHash: "hash", ResponseCode: 201, ResponseBody: []byte("body"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if err := store.WithTx(ctx, func(tx repository.Tx) error { return tx.PutIdempotency(ctx, record) }); err != nil {
		t.Fatal(err)
	}
	var got repository.IdempotencyRecord
	if err := store.Read(ctx, func(reader repository.Reader) error {
		var err error
		got, err = reader.GetIdempotency(ctx, record.Scope, record.Key)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got.ResponseBody[0] = 'B'
	var again repository.IdempotencyRecord
	if err := store.Read(ctx, func(reader repository.Reader) error {
		var err error
		again, err = reader.GetIdempotency(ctx, record.Scope, record.Key)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if string(again.ResponseBody) != "body" {
		t.Fatalf("response body = %q", again.ResponseBody)
	}
}

func TestOutboxClaimRetryAndCompletion(t *testing.T) {
	store, ctx, now := testStore(t)
	job := domain.OutboxJob{ID: "job_1", Kind: "inference_run_planned", AggregateID: "ship_1", Payload: []byte("{}"), Status: domain.JobPending, MaxAttempts: 2, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.WithTx(ctx, func(tx repository.Tx) error { return tx.InsertJob(ctx, job) }); err != nil {
		t.Fatal(err)
	}
	var claimed []domain.OutboxJob
	if err := store.WithTx(ctx, func(tx repository.Tx) error {
		var err error
		claimed, err = tx.ClaimJobs(ctx, now, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].Attempts != 1 || claimed[0].Status != domain.JobRunning {
		t.Fatalf("claimed = %+v", claimed)
	}
	if err := store.WithTx(ctx, func(tx repository.Tx) error {
		return tx.RetryJob(ctx, job.ID, now.Add(time.Minute), "temporary", false)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.WithTx(ctx, func(tx repository.Tx) error {
		jobs, err := tx.ClaimJobs(ctx, now.Add(2*time.Minute), 10)
		if err != nil || len(jobs) != 1 {
			return errors.New("job was not re-claimed")
		}
		return tx.CompleteJob(ctx, jobs[0].ID, now.Add(2*time.Minute))
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRestartRecoversPersistedRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "restart.db")
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, batch := seedCatalog(t, store, ctx, now)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := storeReadSnapshot(reopened, ctx, batch.ID)
	if err != nil || got.ID != batch.ID {
		t.Fatalf("recovered snapshot = %+v, error = %v", got, err)
	}
}

func TestForeignKeyRejectsUnknownWorkspace(t *testing.T) {
	store, ctx, now := testStore(t)
	batch := domain.DatasetSnapshot{ID: "orphan", WorkspaceID: "missing", SourceZoneID: "missing", SourceRevision: "EXT", SchemaFamily: "plasma", PartitionCount: 1, EstimatedRows: 1, State: domain.SnapshotRegistered, ExpiresAt: now.Add(time.Hour), Version: 1, CreatedAt: now, UpdatedAt: now}
	err := store.WithTx(ctx, func(tx repository.Tx) error { return tx.InsertDatasetSnapshot(ctx, batch) })
	if err == nil {
		t.Fatal("orphan insert succeeded")
	}
}

func TestReadinessCountsRelatedRows(t *testing.T) {
	store, ctx, now := testStore(t)
	workspace, origin, destination, compute_pool, batch := seedCatalog(t, store, ctx, now)
	run := domain.InferenceRun{ID: "ship_report", WorkspaceID: workspace.ID, SourceZoneID: origin.ID, TargetZoneID: destination.ID, ComputePoolID: compute_pool.ID, Reference: "REPORT-1", State: domain.InferenceRunCompleted, ScheduledStartAt: now, ExpectedFinishAt: now.Add(time.Hour), TotalEstimatedRows: batch.EstimatedRows, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertInferenceRun(ctx, run); err != nil {
			return err
		}
		batch.State = domain.SnapshotMaterialized
		batch.InferenceRunID = run.ID
		if err := tx.UpdateDatasetSnapshot(ctx, batch, batch.Version); err != nil {
			return err
		}
		return tx.InsertInferenceRunInput(ctx, domain.InferenceRunInput{InferenceRunID: run.ID, SnapshotID: batch.ID, AddedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	var report domain.RunReadiness
	if err := store.Read(ctx, func(reader repository.Reader) error {
		var err error
		report, err = reader.GetRunReadiness(ctx, run.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if report.ExpectedSnapshotCount != 1 || report.MaterializedSnapshotCount != 1 || !report.Complete {
		t.Fatalf("report = %+v", report)
	}
}
