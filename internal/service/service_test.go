package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/clock"
	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
	"github.com/zhanglei10281852-gif/ai/internal/requestmeta"
	"github.com/zhanglei10281852-gif/ai/internal/storage/sqlite"
)

type serviceFixture struct {
	t             *testing.T
	ctx           context.Context
	store         *sqlite.Store
	services      *Services
	clock         *clock.Fixed
	ml_engineer   domain.Principal
	data_engineer domain.Principal
	risk_reviewer domain.Principal
	workspace     domain.Workspace
	origin        domain.DataZone
	destination   domain.DataZone
	compute_pool  domain.ComputePool
	batch         domain.DatasetSnapshot
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	fixed := clock.NewFixed(now)
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	services := New(store, fixed, 4*time.Hour, 30*time.Minute)
	users := []struct {
		email string
		name  string
		role  domain.Role
	}{
		{"ops@example.test", "Ops", domain.RoleMLEngineer},
		{"data_engineer@example.test", "Data Engineer", domain.RoleDataEngineer},
		{"risk_reviewer@example.test", "Reviewer", domain.RoleRiskReviewer},
	}
	principals := make([]domain.Principal, 0, len(users))
	adminCtx := requestmeta.WithPrincipal(ctx, domain.Principal{UserID: "bootstrap-admin", Role: domain.RoleMLEngineer})
	for _, user := range users {
		created, err := services.Auth.CreateUser(adminCtx, user.email, user.name, "very-secure-password", user.role)
		if err != nil {
			t.Fatalf("create user %s: %v", user.email, err)
		}
		login, err := services.Auth.Login(ctx, LoginInput{Email: user.email, Password: "very-secure-password"})
		if err != nil {
			t.Fatalf("login %s: %v", user.email, err)
		}
		if login.Principal.UserID != created.ID {
			t.Fatalf("principal user = %s, created = %s", login.Principal.UserID, created.ID)
		}
		principals = append(principals, login.Principal)
	}
	minimum, _ := domain.ScoreFromFloat(2)
	maximum, _ := domain.ScoreFromFloat(8)
	rangeValue, _ := domain.NewQualityRange(minimum, maximum)
	opsCtx := requestmeta.WithPrincipal(ctx, principals[0])
	workspace, err := services.Catalog.CreateWorkspace(opsCtx, domain.Workspace{Code: "STUDY-1", Name: "Cold workspace", Score: rangeValue, MaxExecution: 24 * time.Hour, ReviewDeadline: 4 * time.Hour, BusinessTimezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = services.Catalog.ActivateWorkspace(opsCtx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := services.Catalog.CreateDataZone(opsCtx, domain.DataZone{Code: "SITE-1", Name: "Origin", Timezone: "Asia/Shanghai", DailyLimit: 10, CutoffHour: 6})
	if err != nil {
		t.Fatal(err)
	}
	destination, err := services.Catalog.CreateDataZone(opsCtx, domain.DataZone{Code: "SITE-2", Name: "Destination", Timezone: "Asia/Shanghai", DailyLimit: 10, CutoffHour: 6})
	if err != nil {
		t.Fatal(err)
	}
	now = fixed.Now()
	compute_pool, err := services.Catalog.CreateComputePool(opsCtx, domain.ComputePool{SerialNumber: "BOX-1", CapacityRows: 1000, AttestationDueAt: now.Add(48 * time.Hour), LastReconciledAt: now})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := services.Catalog.RegisterSnapshot(opsCtx, domain.DatasetSnapshot{WorkspaceID: workspace.ID, SourceZoneID: origin.ID, SourceRevision: "EXT-1", SchemaFamily: "plasma", PartitionCount: 2, EstimatedRows: 100, ExpiresAt: now.Add(48 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	batch, err = services.Catalog.ValidateSnapshot(opsCtx, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &serviceFixture{t: t, ctx: ctx, store: store, services: services, clock: fixed, ml_engineer: principals[0], data_engineer: principals[1], risk_reviewer: principals[2], workspace: workspace, origin: origin, destination: destination, compute_pool: compute_pool, batch: batch}
}

func TestCreateUserRequiresMLEngineerAtServiceBoundary(t *testing.T) {
	f := newServiceFixture(t)
	for name, ctx := range map[string]context.Context{
		"unauthenticated": f.ctx,
		"wrong role":      f.as(f.data_engineer),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.services.Auth.CreateUser(ctx, "blocked@example.test", "Blocked User", "very-secure-password", domain.RoleRiskReviewer)
			if name == "unauthenticated" && !errors.Is(err, domain.ErrUnauthenticated) {
				t.Fatalf("error = %v, want unauthenticated", err)
			}
			if name == "wrong role" && !errors.Is(err, domain.ErrForbidden) {
				t.Fatalf("error = %v, want forbidden", err)
			}
		})
	}
}

func (f *serviceFixture) as(principal domain.Principal) context.Context {
	return requestmeta.WithPrincipal(requestmeta.WithRequestID(f.ctx, "req-test"), principal)
}

func TestAuthRejectsWrongPasswordAndHonorsLogout(t *testing.T) {
	f := newServiceFixture(t)
	if _, err := f.services.Auth.Login(f.ctx, LoginInput{Email: f.ml_engineer.Email, Password: "wrong-password"}); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("wrong password error = %v", err)
	}
	if err := f.services.Auth.Logout(f.as(f.ml_engineer), f.ml_engineer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Auth.Authenticate(f.ctx, "missing-token"); err == nil {
		t.Fatal("missing token authenticated")
	}
}

func TestSessionExpiresAtConfiguredBoundary(t *testing.T) {
	f := newServiceFixture(t)
	login, err := f.services.Auth.Login(f.ctx, LoginInput{Email: f.ml_engineer.Email, Password: "very-secure-password"})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(4 * time.Hour)
	if _, err := f.services.Auth.Authenticate(f.ctx, login.Token); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("authentication at expiry boundary error = %v", err)
	}
}

func TestCatalogRejectsInvalidAggregateInputsBeforePersistence(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	invalidWorkspace := domain.Workspace{Code: "INVALID-WORKSPACE", Score: f.workspace.Score, MaxExecution: time.Hour, ReviewDeadline: time.Hour, BusinessTimezone: "UTC"}
	if _, err := f.services.Catalog.CreateWorkspace(ctx, invalidWorkspace); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid workspace error = %v", err)
	}
	invalidZone := domain.DataZone{Code: "INVALID-ZONE", Name: "Invalid timezone", Timezone: "Mars/Olympus", DailyLimit: 1, CutoffHour: 0}
	if _, err := f.services.Catalog.CreateDataZone(ctx, invalidZone); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid data zone error = %v", err)
	}
	invalidPool := domain.ComputePool{SerialNumber: "INVALID-POOL", CapacityRows: 1, AttestationDueAt: f.clock.Now().Add(time.Hour), LastReconciledAt: f.clock.Now()}
	if _, err := f.services.Catalog.CreateComputePool(ctx, invalidPool); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid compute pool error = %v", err)
	}
	invalidSnapshot := domain.DatasetSnapshot{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, SchemaFamily: "ranking", PartitionCount: 1, EstimatedRows: 10, ExpiresAt: f.clock.Now().Add(time.Hour)}
	if _, err := f.services.Catalog.RegisterSnapshot(ctx, invalidSnapshot); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid snapshot error = %v", err)
	}
}

func TestRiskReviewerCannotWriteTheCatalog(t *testing.T) {
	f := newServiceFixture(t)
	input := domain.DataZone{Code: "REVIEWER-ZONE", Name: "Reviewer zone", Timezone: "UTC", DailyLimit: 1, CutoffHour: 0}
	if _, err := f.services.Catalog.CreateDataZone(f.as(f.risk_reviewer), input); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("risk reviewer catalog error = %v", err)
	}
}

func TestExpiredIdempotencyKeyCanStartANewRun(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	firstInput := PlanInferenceRunInput{
		WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID,
		ComputePoolID: f.compute_pool.ID, Reference: "RUN-IDEMPOTENCY-OLD", SnapshotIDs: []string{f.batch.ID},
		ScheduledStartAt: f.clock.Now().Add(time.Hour), ExpectedFinishAt: f.clock.Now().Add(2 * time.Hour), IdempotencyKey: "reusable-plan-key",
	}
	first, err := f.services.Inference.PlanInferenceRun(ctx, firstInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.CancelInferenceRun(ctx, first.ID, "superseded plan"); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(25 * time.Hour)
	newSnapshot, err := f.services.Catalog.RegisterSnapshot(ctx, domain.DatasetSnapshot{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, SourceRevision: "IDEMPOTENCY-NEW", SchemaFamily: "agent-policy", PartitionCount: 1, EstimatedRows: 100, ExpiresAt: f.clock.Now().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	newSnapshot, err = f.services.Catalog.ValidateSnapshot(ctx, newSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := firstInput
	secondInput.Reference = "RUN-IDEMPOTENCY-NEW"
	secondInput.SnapshotIDs = []string{newSnapshot.ID}
	secondInput.ScheduledStartAt = f.clock.Now().Add(time.Hour)
	secondInput.ExpectedFinishAt = f.clock.Now().Add(2 * time.Hour)
	second, err := f.services.Inference.PlanInferenceRun(ctx, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.Reference != secondInput.Reference {
		t.Fatalf("expired idempotency response = %+v, first = %+v", second, first)
	}
}

func TestCancelledRunSnapshotCanBeReplanned(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	first, err := f.services.Inference.PlanInferenceRun(ctx, PlanInferenceRunInput{
		WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID,
		ComputePoolID: f.compute_pool.ID, Reference: "RUN-CANCELLED", SnapshotIDs: []string{f.batch.ID},
		ScheduledStartAt: f.clock.Now().Add(time.Hour), ExpectedFinishAt: f.clock.Now().Add(2 * time.Hour), IdempotencyKey: "cancelled-run-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.CancelInferenceRun(ctx, first.ID, "replace deployment plan"); err != nil {
		t.Fatal(err)
	}

	second, err := f.services.Inference.PlanInferenceRun(ctx, PlanInferenceRunInput{
		WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID,
		ComputePoolID: f.compute_pool.ID, Reference: "RUN-REPLANNED", SnapshotIDs: []string{f.batch.ID},
		ScheduledStartAt: f.clock.Now().Add(3 * time.Hour), ExpectedFinishAt: f.clock.Now().Add(4 * time.Hour), IdempotencyKey: "replanned-run-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatalf("replanned run reused cancelled run ID %q", first.ID)
	}

	if err := f.store.Read(ctx, func(reader repository.Reader) error {
		oldInputs, err := reader.ListInferenceRunInputs(ctx, first.ID)
		if err != nil {
			return err
		}
		if len(oldInputs) != 0 {
			t.Fatalf("cancelled run still has inputs: %+v", oldInputs)
		}
		newInputs, err := reader.ListInferenceRunInputs(ctx, second.ID)
		if err != nil {
			return err
		}
		if len(newInputs) != 1 || newInputs[0].ID != f.batch.ID || newInputs[0].InferenceRunID != second.ID {
			t.Fatalf("replanned inputs = %+v", newInputs)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDailyLimitUsesTheZoneCutoffAcrossUTCDates(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	source, err := f.services.Catalog.CreateDataZone(ctx, domain.DataZone{Code: "LIMIT-ZONE", Name: "Limited source", Timezone: "Asia/Shanghai", DailyLimit: 1, CutoffHour: 6})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := f.services.Catalog.CreateComputePool(ctx, domain.ComputePool{SerialNumber: "LIMIT-POOL", CapacityRows: 1000, AttestationDueAt: f.clock.Now().Add(72 * time.Hour), LastReconciledAt: f.clock.Now()})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.services.Catalog.RegisterSnapshot(ctx, domain.DatasetSnapshot{WorkspaceID: f.workspace.ID, SourceZoneID: source.ID, SourceRevision: "LIMIT-REV", SchemaFamily: "agent-policy", PartitionCount: 1, EstimatedRows: 100, ExpiresAt: f.clock.Now().Add(72 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = f.services.Catalog.ValidateSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	existingStart := time.Date(2026, 8, 18, 23, 30, 0, 0, time.UTC)
	existing := domain.InferenceRun{ID: "run_daily_limit", WorkspaceID: f.workspace.ID, SourceZoneID: source.ID, TargetZoneID: f.destination.ID, ComputePoolID: pool.ID, Reference: "LIMIT-EXISTING", State: domain.InferenceRunQueued, ScheduledStartAt: existingStart, ExpectedFinishAt: existingStart.Add(time.Hour), TotalEstimatedRows: 10, Version: 1, CreatedAt: f.clock.Now(), UpdatedAt: f.clock.Now()}
	if err := f.store.WithTx(ctx, func(tx repository.Tx) error { return tx.InsertInferenceRun(ctx, existing) }); err != nil {
		t.Fatal(err)
	}
	newStart := time.Date(2026, 8, 19, 0, 30, 0, 0, time.UTC)
	_, err = f.services.Inference.PlanInferenceRun(ctx, PlanInferenceRunInput{WorkspaceID: f.workspace.ID, SourceZoneID: source.ID, TargetZoneID: f.destination.ID, ComputePoolID: pool.ID, Reference: "LIMIT-NEW", SnapshotIDs: []string{snapshot.ID}, ScheduledStartAt: newStart, ExpectedFinishAt: newStart.Add(time.Hour), IdempotencyKey: "daily-limit-key"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("same business-day plan error = %v", err)
	}
}

func TestApprovalExpiresAtTheExactDeadline(t *testing.T) {
	f := newServiceFixture(t)
	run := f.planAndStart(t, "RUN-APPROVAL-DEADLINE")
	approval, err := f.services.Approval.CreateApprovalTask(f.as(f.data_engineer), CreateApprovalTaskInput{InferenceRunID: run.ID, RequesterID: f.ml_engineer.UserID, ReviewerID: f.data_engineer.UserID, ReviewQueue: "safety-review"})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(30 * time.Minute)
	if _, err := f.services.Approval.ResolveApprovalTask(f.as(f.data_engineer), approval.ID, true, "late acceptance"); !errors.Is(err, domain.ErrExpired) {
		t.Fatalf("approval at exact deadline error = %v", err)
	}
}

func TestInferenceIsIdempotentAndReservesRelatedEntities(t *testing.T) {
	f := newServiceFixture(t)
	input := PlanInferenceRunInput{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID, ComputePoolID: f.compute_pool.ID, Reference: "SHIP-1", SnapshotIDs: []string{f.batch.ID}, ScheduledStartAt: f.clock.Now().Add(time.Hour), ExpectedFinishAt: f.clock.Now().Add(2 * time.Hour), IdempotencyKey: "plan-key"}
	ctx := f.as(f.ml_engineer)
	first, err := f.services.Inference.PlanInferenceRun(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.services.Inference.PlanInferenceRun(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Reference != "SHIP-1" {
		t.Fatalf("idempotent responses differ: %+v / %+v", first, second)
	}
	if err := f.store.Read(ctx, func(reader repository.Reader) error {
		batch, err := reader.GetDatasetSnapshot(ctx, f.batch.ID)
		if err != nil {
			return err
		}
		if batch.State != domain.SnapshotReserved || batch.InferenceRunID != first.ID {
			t.Fatalf("reserved batch = %+v", batch)
		}
		compute_pool, err := reader.GetComputePool(ctx, f.compute_pool.ID)
		if err != nil {
			return err
		}
		if compute_pool.State != domain.ComputePoolReserved || compute_pool.ReservedRunID != first.ID {
			t.Fatalf("reserved compute_pool = %+v", compute_pool)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestInferenceRejectsDifferentIdempotencyPayload(t *testing.T) {
	f := newServiceFixture(t)
	input := PlanInferenceRunInput{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID, ComputePoolID: f.compute_pool.ID, Reference: "SHIP-1", SnapshotIDs: []string{f.batch.ID}, ScheduledStartAt: f.clock.Now().Add(time.Hour), ExpectedFinishAt: f.clock.Now().Add(2 * time.Hour), IdempotencyKey: "plan-key"}
	ctx := f.as(f.ml_engineer)
	if _, err := f.services.Inference.PlanInferenceRun(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.Reference = "SHIP-OTHER"
	if _, err := f.services.Inference.PlanInferenceRun(ctx, input); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("different payload error = %v", err)
	}
}

func TestInferenceLifecycleMovesSnapshotsAndComputePool(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	run, err := f.services.Inference.PlanInferenceRun(ctx, PlanInferenceRunInput{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID, ComputePoolID: f.compute_pool.ID, Reference: "SHIP-LIFE", SnapshotIDs: []string{f.batch.ID}, ScheduledStartAt: f.clock.Now().Add(time.Hour), ExpectedFinishAt: f.clock.Now().Add(2 * time.Hour), IdempotencyKey: "life-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.StageInferenceRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.StartInferenceRun(f.as(f.data_engineer), run.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Read(ctx, func(reader repository.Reader) error {
		batch, err := reader.GetDatasetSnapshot(ctx, f.batch.ID)
		if err != nil {
			return err
		}
		if batch.State != domain.SnapshotMaterializing {
			t.Fatalf("in execution batch = %+v", batch)
		}
		compute_pool, err := reader.GetComputePool(ctx, f.compute_pool.ID)
		if err != nil {
			return err
		}
		if compute_pool.State != domain.ComputePoolAllocated {
			t.Fatalf("in execution compute_pool = %+v", compute_pool)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.CompleteInferenceRun(f.as(f.data_engineer), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.ArchiveInferenceRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
}

func TestApprovalOnlyReceiverCanResolve(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	run, err := f.services.Inference.PlanInferenceRun(ctx, PlanInferenceRunInput{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID, ComputePoolID: f.compute_pool.ID, Reference: "SHIP-HAND", SnapshotIDs: []string{f.batch.ID}, ScheduledStartAt: f.clock.Now().Add(time.Hour), ExpectedFinishAt: f.clock.Now().Add(2 * time.Hour), IdempotencyKey: "hand-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.StageInferenceRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.StartInferenceRun(f.as(f.data_engineer), run.ID); err != nil {
		t.Fatal(err)
	}
	approval_task, err := f.services.Approval.CreateApprovalTask(f.as(f.data_engineer), CreateApprovalTaskInput{InferenceRunID: run.ID, RequesterID: f.ml_engineer.UserID, ReviewerID: f.data_engineer.UserID, ReviewQueue: "Dock 2"})
	if err != nil {
		t.Fatal(err)
	}
	other := domain.Principal{UserID: "other-data-engineer", Role: domain.RoleDataEngineer}
	if _, err := f.services.Approval.ResolveApprovalTask(f.as(other), approval_task.ID, true, "wrong actor"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("wrong actor error = %v", err)
	}
	if _, err := f.services.Approval.ResolveApprovalTask(f.as(f.data_engineer), approval_task.ID, true, "seal intact"); err != nil {
		t.Fatal(err)
	}
}

func (f *serviceFixture) planAndStart(t *testing.T, ref string) domain.InferenceRun {
	t.Helper()
	run, err := f.services.Inference.PlanInferenceRun(f.as(f.ml_engineer), PlanInferenceRunInput{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, TargetZoneID: f.destination.ID, ComputePoolID: f.compute_pool.ID, Reference: ref, SnapshotIDs: []string{f.batch.ID}, ScheduledStartAt: f.clock.Now().Add(time.Hour), ExpectedFinishAt: f.clock.Now().Add(2 * time.Hour), IdempotencyKey: ref + "-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.StageInferenceRun(f.as(f.ml_engineer), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.StartInferenceRun(f.as(f.data_engineer), run.ID); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestScoreDriftIncidentQuarantinesAndReviewerClears(t *testing.T) {
	f := newServiceFixture(t)
	run := f.planAndStart(t, "RUN-DRIFT")
	observation, drift_incident, err := f.services.Metrics.RecordObservation(f.as(f.data_engineer), RecordObservationInput{InferenceRunID: run.ID, MetricKey: "sensor-1", Sequence: 1, Score: 12000, RecordedAt: f.clock.Now().Add(10 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if observation.ID == "" || drift_incident == nil || drift_incident.ObservationCount != 1 {
		t.Fatalf("observation=%+v drift_incident=%+v", observation, drift_incident)
	}
	if _, err := f.services.Inference.CompleteInferenceRun(f.as(f.data_engineer), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Review.StartReview(f.as(f.risk_reviewer), drift_incident.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Review.Decide(f.as(f.risk_reviewer), DecideInput{DriftIncidentID: drift_incident.ID, Decision: domain.DriftIncidentCleared, Rationale: "validated logger trace"}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Read(f.ctx, func(reader repository.Reader) error {
		batch, err := reader.GetDatasetSnapshot(f.ctx, f.batch.ID)
		if err != nil {
			return err
		}
		if batch.State != domain.SnapshotApproved {
			t.Fatalf("batch after clear = %+v", batch)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestInRangeObservationDoesNotOpenDriftIncident(t *testing.T) {
	f := newServiceFixture(t)
	run := f.planAndStart(t, "RUN-IN-RANGE")
	_, drift_incident, err := f.services.Metrics.RecordObservation(f.as(f.data_engineer), RecordObservationInput{InferenceRunID: run.ID, MetricKey: "sensor-1", Sequence: 1, Score: 5000, RecordedAt: f.clock.Now()})
	if err != nil || drift_incident != nil {
		t.Fatalf("in range result drift_incident=%+v error=%v", drift_incident, err)
	}
}

func TestQueryReadinessReportsBlockers(t *testing.T) {
	f := newServiceFixture(t)
	run := f.planAndStart(t, "RUN-REPORT")
	report, err := f.services.Query.ReconcileInferenceRun(f.as(f.ml_engineer), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.ExpectedSnapshotCount != 1 || report.Complete {
		t.Fatalf("report = %+v", report)
	}
}

func TestContextCancellationReachesTransaction(t *testing.T) {
	f := newServiceFixture(t)
	cancelled, cancel := context.WithCancel(f.as(f.ml_engineer))
	cancel()
	_, err := f.services.Catalog.ValidateSnapshot(cancelled, f.batch.ID)
	if err == nil {
		t.Fatal("cancelled command succeeded")
	}
}

func TestComputePoolReconcilingAndRetirementLifecycle(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.as(f.ml_engineer)
	reconciliation, err := f.services.ComputePools.StartReconciliation(ctx, f.compute_pool.ID)
	if err != nil || reconciliation.State != domain.ComputePoolReconciling {
		t.Fatalf("start reconciliation = %+v, error=%v", reconciliation, err)
	}
	f.clock.Advance(time.Hour)
	available, err := f.services.ComputePools.CompleteReconciliation(ctx, f.compute_pool.ID)
	if err != nil || available.State != domain.ComputePoolAvailable || !available.LastReconciledAt.Equal(f.clock.Now()) {
		t.Fatalf("complete reconciliation = %+v, error=%v", available, err)
	}
	retired, err := f.services.ComputePools.Retire(ctx, f.compute_pool.ID, "attestation program ended")
	if err != nil || retired.State != domain.ComputePoolRetired {
		t.Fatalf("retire = %+v, error=%v", retired, err)
	}
	if _, err := f.services.ComputePools.StartReconciliation(ctx, f.compute_pool.ID); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("clean retired error = %v", err)
	}
}

func TestBulkRegistrationReturnsPartialFailures(t *testing.T) {
	f := newServiceFixture(t)
	now := f.clock.Now()
	inputs := []domain.DatasetSnapshot{
		{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, SourceRevision: "BULK-OK", SchemaFamily: "serum", PartitionCount: 1, EstimatedRows: 20, ExpiresAt: now.Add(time.Hour)},
		{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, SourceRevision: "", SchemaFamily: "serum", PartitionCount: 1, EstimatedRows: 20, ExpiresAt: now.Add(time.Hour)},
		{WorkspaceID: f.workspace.ID, SourceZoneID: f.origin.ID, SourceRevision: "BULK-OK", SchemaFamily: "serum", PartitionCount: 1, EstimatedRows: 20, ExpiresAt: now.Add(time.Hour)},
	}
	result, err := f.services.Catalog.BulkRegisterSnapshots(f.as(f.ml_engineer), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 1 || result.Failed != 2 || len(result.Items) != 3 {
		t.Fatalf("bulk result = %+v", result)
	}
	if result.Items[0].Code != "created" || result.Items[1].Code != "invalid" || result.Items[2].Code != "conflict" {
		t.Fatalf("bulk item codes = %+v", result.Items)
	}
}

func TestPlatformSummaryRequiresReadPermissionAndCountsRows(t *testing.T) {
	f := newServiceFixture(t)
	if _, err := f.services.Query.PlatformSummary(f.as(f.risk_reviewer)); err != nil {
		t.Fatalf("risk_reviewer summary: %v", err)
	}
	summary, err := f.services.Query.PlatformSummary(f.as(f.ml_engineer))
	if err != nil {
		t.Fatal(err)
	}
	if summary.WorkspacesActive != 1 || summary.SnapshotsValidated != 1 || summary.ComputePoolsAvailable != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if _, err := f.services.Query.PlatformSummary(f.as(domain.Principal{UserID: "data_engineer", Role: domain.RoleDataEngineer})); err != nil {
		t.Fatalf("data_engineer read summary: %v", err)
	}
}
