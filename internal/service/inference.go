package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/idempotency"
	"github.com/zhanglei10281852-gif/ai/internal/identity"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

type InferenceService struct{ dependencies }

type PlanInferenceRunInput struct {
	WorkspaceID      string
	SourceZoneID     string
	TargetZoneID     string
	ComputePoolID    string
	Reference        string
	SnapshotIDs      []string
	ScheduledStartAt time.Time
	ExpectedFinishAt time.Time
	IdempotencyKey   string
}

func (s *InferenceService) PlanInferenceRun(ctx context.Context, input PlanInferenceRunInput) (domain.InferenceRun, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.InferenceRun{}, err
	}
	if len(input.SnapshotIDs) == 0 {
		return domain.InferenceRun{}, domain.FieldError{Field: "snapshot_ids", Message: "at least one snapshot is required"}
	}
	if err := domain.ValidateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.InferenceRun{}, err
	}
	hash, err := idempotency.Hash(input)
	if err != nil {
		return domain.InferenceRun{}, err
	}
	var run domain.InferenceRun
	err = s.store.WithTx(ctx, func(tx repository.Tx) error {
		now := s.clock.Now()
		if existing, err := tx.GetIdempotency(ctx, "plan-run", input.IdempotencyKey); err == nil {
			if existing.ExpiresAt.After(now) {
				if existing.RequestHash != hash {
					return domain.ConflictError{Resource: "idempotency_key", Reason: "request payload differs"}
				}
				return json.Unmarshal(existing.ResponseBody, &run)
			}
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		workspace, err := tx.GetWorkspace(ctx, input.WorkspaceID)
		if err != nil {
			return err
		}
		if !workspace.CanAcceptInferenceRuns() {
			return domain.ConflictError{Resource: "workspace", Reason: "workspace is not active"}
		}
		source, err := tx.GetDataZone(ctx, input.SourceZoneID)
		if err != nil {
			return err
		}
		target, err := tx.GetDataZone(ctx, input.TargetZoneID)
		if err != nil {
			return err
		}
		if source.Status != domain.DataZoneActive || target.Status != domain.DataZoneActive {
			return domain.ConflictError{Resource: "data_zone", Reason: "source and target zones must be active"}
		}
		if err := domain.ValidateRoute(source, target); err != nil {
			return err
		}
		dayStart, dayEnd, err := source.BusinessDayWindow(input.ScheduledStartAt)
		if err != nil {
			return err
		}
		count, err := tx.CountDataZoneInferenceRunsForBusinessDay(ctx, source.ID, dayStart, dayEnd)
		if err != nil {
			return err
		}
		if count >= source.DailyLimit {
			return domain.ConflictError{Resource: "data_zone", Reason: "daily run limit reached"}
		}
		compute_pool, err := tx.GetComputePool(ctx, input.ComputePoolID)
		if err != nil {
			return err
		}
		if err := compute_pool.EligibleFor(input.ScheduledStartAt, 1); err != nil {
			return err
		}
		run = domain.InferenceRun{ID: identity.New("run"), WorkspaceID: input.WorkspaceID, SourceZoneID: input.SourceZoneID,
			TargetZoneID: input.TargetZoneID, ComputePoolID: input.ComputePoolID, Reference: strings.TrimSpace(input.Reference),
			State: domain.InferenceRunQueued, ScheduledStartAt: input.ScheduledStartAt.UTC(), ExpectedFinishAt: input.ExpectedFinishAt.UTC(), Version: 1, CreatedAt: now, UpdatedAt: now}
		volume := 0
		batches := make([]domain.DatasetSnapshot, 0, len(input.SnapshotIDs))
		seen := make(map[string]struct{}, len(input.SnapshotIDs))
		for _, batchID := range input.SnapshotIDs {
			if _, exists := seen[batchID]; exists {
				return domain.ConflictError{Resource: "dataset_snapshot", Reason: "duplicate snapshot in request"}
			}
			seen[batchID] = struct{}{}
			batch, err := tx.GetDatasetSnapshot(ctx, batchID)
			if err != nil {
				return err
			}
			if batch.WorkspaceID != workspace.ID || batch.SourceZoneID != source.ID {
				return domain.ConflictError{Resource: "dataset_snapshot", Reason: "snapshot belongs to another workspace or source zone"}
			}
			if err := batch.Transition(domain.SnapshotReserved, now); err != nil {
				return err
			}
			volume += batch.EstimatedRows
			batches = append(batches, batch)
		}
		if err := (domain.ExecutionWindow{StartAt: input.ScheduledStartAt.UTC(), FinishAt: input.ExpectedFinishAt.UTC()}).Validate(workspace, batches, now); err != nil {
			return err
		}
		run.TotalEstimatedRows = volume
		if err := compute_pool.EligibleFor(input.ScheduledStartAt, volume); err != nil {
			return err
		}
		if err := run.Validate(); err != nil {
			return err
		}
		if err := tx.InsertInferenceRun(ctx, run); err != nil {
			return err
		}
		for _, batch := range batches {
			batch.InferenceRunID = run.ID
			if err := tx.UpdateDatasetSnapshot(ctx, batch, batch.Version); err != nil {
				return err
			}
			if err := tx.InsertInferenceRunInput(ctx, domain.InferenceRunInput{InferenceRunID: run.ID, SnapshotID: batch.ID, AddedAt: now}); err != nil {
				return err
			}
		}
		compute_pool.State = domain.ComputePoolReserved
		compute_pool.ReservedRunID = run.ID
		compute_pool.UpdatedAt = now
		if err := tx.UpdateComputePool(ctx, compute_pool, compute_pool.Version); err != nil {
			return err
		}
		body, err := idempotency.Encode(run)
		if err != nil {
			return err
		}
		if err := tx.PutIdempotency(ctx, repository.IdempotencyRecord{Scope: "plan-run", Key: input.IdempotencyKey, RequestHash: hash, ResponseCode: 201, ResponseBody: body, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now}); err != nil {
			return err
		}
		if err := tx.InsertJob(ctx, domain.OutboxJob{ID: identity.New("job"), Kind: "inference_run_planned", AggregateID: run.ID, Payload: body, Status: domain.JobPending, MaxAttempts: 5, AvailableAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, "inference_run_planned", "inference_run", run.ID, "success", map[string]string{"snapshot_count": fmt.Sprint(len(batches))})
	})
	return run, err
}

func (s *InferenceService) StageInferenceRun(ctx context.Context, runID string) (domain.InferenceRun, error) {
	return s.transition(ctx, runID, domain.InferenceRunStaged, domain.RoleMLEngineer, "run_staged")
}

func (s *InferenceService) StartInferenceRun(ctx context.Context, runID string) (domain.InferenceRun, error) {
	return s.transitionAny(ctx, runID, domain.InferenceRunRunning, []domain.Role{domain.RoleDataEngineer, domain.RoleMLEngineer}, "run_started")
}

func (s *InferenceService) CompleteInferenceRun(ctx context.Context, runID string) (domain.InferenceRun, error) {
	return s.transitionAny(ctx, runID, domain.InferenceRunCompleted, []domain.Role{domain.RoleDataEngineer, domain.RoleMLEngineer}, "run_completed")
}

func (s *InferenceService) ArchiveInferenceRun(ctx context.Context, runID string) (domain.InferenceRun, error) {
	return s.transitionAny(ctx, runID, domain.InferenceRunArchived, []domain.Role{domain.RoleMLEngineer}, "run_archived")
}

func (s *InferenceService) CancelInferenceRun(ctx context.Context, runID string, note string) (domain.InferenceRun, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.InferenceRun{}, err
	}
	var result domain.InferenceRun
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		run, err := tx.GetInferenceRun(ctx, runID)
		if err != nil {
			return err
		}
		if run.State != domain.InferenceRunQueued && run.State != domain.InferenceRunStaged {
			return domain.TransitionError{Entity: "inference_run", From: string(run.State), To: string(domain.InferenceRunCancelled)}
		}
		now := s.clock.Now()
		items, err := tx.ListInferenceRunInputs(ctx, run.ID)
		if err != nil {
			return err
		}
		if err := run.Transition(domain.InferenceRunCancelled, now); err != nil {
			return err
		}
		for _, batch := range items {
			if err := batch.Transition(domain.SnapshotValidated, now); err != nil {
				return err
			}
			batch.InferenceRunID = ""
			if err := tx.UpdateDatasetSnapshot(ctx, batch, batch.Version); err != nil {
				return err
			}
		}
		if err := tx.DeleteInferenceRunInputs(ctx, run.ID); err != nil {
			return err
		}
		compute_pool, err := tx.GetComputePool(ctx, run.ComputePoolID)
		if err != nil {
			return err
		}
		compute_pool.State = domain.ComputePoolAvailable
		compute_pool.ReservedRunID = ""
		compute_pool.UpdatedAt = now
		if err := tx.UpdateComputePool(ctx, compute_pool, compute_pool.Version); err != nil {
			return err
		}
		if err := tx.UpdateInferenceRun(ctx, run, run.Version); err != nil {
			return err
		}
		result = run
		return s.audit.Record(ctx, tx, "run_cancelled", "run", run.ID, "success", map[string]string{"note": strings.TrimSpace(note)})
	})
	return result, err
}

func (s *InferenceService) transition(ctx context.Context, runID string, target domain.InferenceRunState, role domain.Role, action string) (domain.InferenceRun, error) {
	return s.transitionAny(ctx, runID, target, []domain.Role{role}, action)
}

func (s *InferenceService) transitionAny(ctx context.Context, runID string, target domain.InferenceRunState, roles []domain.Role, action string) (domain.InferenceRun, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, roles...); err != nil {
		return domain.InferenceRun{}, err
	}
	var result domain.InferenceRun
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		run, err := tx.GetInferenceRun(ctx, runID)
		if err != nil {
			return err
		}
		if err := run.Transition(target, s.clock.Now()); err != nil {
			return err
		}
		now := s.clock.Now()
		items, err := tx.ListInferenceRunInputs(ctx, run.ID)
		if err != nil {
			return err
		}
		for _, batch := range items {
			switch target {
			case domain.InferenceRunRunning:
				if err := batch.Transition(domain.SnapshotMaterializing, now); err != nil {
					return err
				}
			case domain.InferenceRunCompleted:
				if batch.State != domain.SnapshotQuarantined && batch.State != domain.SnapshotRejected && batch.State != domain.SnapshotApproved {
					if err := batch.Transition(domain.SnapshotMaterialized, now); err != nil {
						return err
					}
				}
			case domain.InferenceRunArchived:
				if batch.State != domain.SnapshotApproved && batch.State != domain.SnapshotRejected && batch.State != domain.SnapshotMaterialized {
					return domain.ConflictError{Resource: "dataset_snapshot", Reason: "all snapshots must be resolved before archiving"}
				}
			}
			if target == domain.InferenceRunRunning || target == domain.InferenceRunCompleted {
				if err := tx.UpdateDatasetSnapshot(ctx, batch, batch.Version); err != nil {
					return err
				}
			}
		}
		compute_pool, err := tx.GetComputePool(ctx, run.ComputePoolID)
		if err != nil {
			return err
		}
		switch target {
		case domain.InferenceRunRunning:
			compute_pool.State = domain.ComputePoolAllocated
		case domain.InferenceRunArchived, domain.InferenceRunCancelled:
			compute_pool.State = domain.ComputePoolAvailable
			compute_pool.ReservedRunID = ""
		}
		compute_pool.UpdatedAt = now
		if err := tx.UpdateComputePool(ctx, compute_pool, compute_pool.Version); err != nil {
			return err
		}
		if err := tx.UpdateInferenceRun(ctx, run, run.Version); err != nil {
			return err
		}
		result = run
		return s.audit.Record(ctx, tx, action, "inference_run", run.ID, "success", nil)
	})
	return result, err
}
