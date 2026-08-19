package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/identity"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

type MetricsService struct{ dependencies }

type RecordObservationInput struct {
	InferenceRunID string
	MetricKey      string
	Sequence       int64
	Score          domain.MilliScore
	RecordedAt     time.Time
}

func (s *MetricsService) RecordObservation(ctx context.Context, input RecordObservationInput) (domain.QualityObservation, *domain.DriftIncident, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleDataEngineer, domain.RoleMLEngineer); err != nil {
		return domain.QualityObservation{}, nil, err
	}
	now := s.clock.Now()
	observation := domain.QualityObservation{ID: identity.New("obs"), InferenceRunID: input.InferenceRunID, MetricKey: input.MetricKey, Sequence: input.Sequence, Score: input.Score, RecordedAt: input.RecordedAt.UTC(), ReceivedAt: now}
	if err := observation.Validate(); err != nil {
		return domain.QualityObservation{}, nil, err
	}
	var drift_incident *domain.DriftIncident
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		run, err := tx.GetInferenceRun(ctx, input.InferenceRunID)
		if err != nil {
			return err
		}
		workspace, err := tx.GetWorkspace(ctx, run.WorkspaceID)
		if err != nil {
			return err
		}
		if run.State != domain.InferenceRunRunning && run.State != domain.InferenceRunCompleted {
			return domain.ConflictError{Resource: "inference_run", Reason: "quality observations require active execution"}
		}
		if err := tx.InsertObservation(ctx, observation); err != nil {
			return err
		}
		if workspace.Score.Contains(observation.Score) {
			return s.audit.Record(ctx, tx, "quality_observation_recorded", "inference_run", run.ID, "success", map[string]string{"in_range": "true"})
		}
		active, err := tx.GetActiveDriftIncident(ctx, run.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if errors.Is(err, domain.ErrNotFound) {
			active = domain.DriftIncident{ID: identity.New("drift"), InferenceRunID: run.ID, Status: domain.DriftIncidentOpen, ReviewDueAt: now.Add(workspace.ReviewDeadline), Version: 1, CreatedAt: now, UpdatedAt: now}
			active.Include(observation, now)
			if err := tx.InsertDriftIncident(ctx, active); err != nil {
				return err
			}
		} else {
			before := active.Version
			active.Include(observation, now)
			if err := tx.UpdateDriftIncident(ctx, active, before); err != nil {
				return err
			}
		}
		items, err := tx.ListInferenceRunInputs(ctx, run.ID)
		if err != nil {
			return err
		}
		for _, batch := range items {
			if batch.State == domain.SnapshotMaterializing || batch.State == domain.SnapshotMaterialized {
				batch.State = domain.SnapshotQuarantined
				batch.QuarantineNote = fmt.Sprintf("quality drift incident %s", active.ID)
				batch.UpdatedAt = now
				if err := tx.UpdateDatasetSnapshot(ctx, batch, batch.Version); err != nil {
					return err
				}
			}
		}
		payload, err := json.Marshal(active.ID)
		if err != nil {
			return fmt.Errorf("encode drift incident review payload: %w", err)
		}
		if err := tx.InsertJob(ctx, domain.OutboxJob{ID: identity.New("job"), Kind: "drift_incident_review", AggregateID: active.ID, Payload: payload, Status: domain.JobPending, MaxAttempts: 5, AvailableAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		drift_incident = &active
		return s.audit.Record(ctx, tx, "quality_drift_incident_opened", "drift_incident", active.ID, "success", map[string]string{"run_id": run.ID})
	})
	return observation, drift_incident, err
}
