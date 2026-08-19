package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/identity"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

type ReviewService struct{ dependencies }

func (s *ReviewService) StartReview(ctx context.Context, drift_incidentID string) (domain.DriftIncident, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleRiskReviewer); err != nil {
		return domain.DriftIncident{}, err
	}
	var result domain.DriftIncident
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		drift_incident, err := tx.GetDriftIncident(ctx, drift_incidentID)
		if err != nil {
			return err
		}
		before := drift_incident.Version
		if err := drift_incident.StartReview(s.clock.Now()); err != nil {
			return err
		}
		if err := tx.UpdateDriftIncident(ctx, drift_incident, before); err != nil {
			return err
		}
		result = drift_incident
		return s.audit.Record(ctx, tx, "drift_incident_review_started", "drift_incident", drift_incident.ID, "success", nil)
	})
	return result, err
}

type DecideInput struct {
	DriftIncidentID string
	Decision        domain.DriftIncidentStatus
	Rationale       string
}

func (s *ReviewService) Decide(ctx context.Context, input DecideInput) (domain.DriftIncident, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleRiskReviewer); err != nil {
		return domain.DriftIncident{}, err
	}
	if strings.TrimSpace(input.Rationale) == "" {
		return domain.DriftIncident{}, domain.FieldError{Field: "rationale", Message: "is required"}
	}
	var result domain.DriftIncident
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		drift_incident, err := tx.GetDriftIncident(ctx, input.DriftIncidentID)
		if err != nil {
			return err
		}
		before := drift_incident.Version
		if err := drift_incident.Decide(input.Decision, s.clock.Now()); err != nil {
			return err
		}
		if err := tx.UpdateDriftIncident(ctx, drift_incident, before); err != nil {
			return err
		}
		run, err := tx.GetInferenceRun(ctx, drift_incident.InferenceRunID)
		if err != nil {
			return err
		}
		items, err := tx.ListInferenceRunInputs(ctx, run.ID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, batch := range items {
			switch input.Decision {
			case domain.DriftIncidentCleared:
				if batch.State != domain.SnapshotQuarantined {
					continue
				}
				batch.State = domain.SnapshotApproved
				batch.QuarantineNote = ""
			case domain.DriftIncidentRejected:
				if batch.State != domain.SnapshotQuarantined {
					continue
				}
				batch.State = domain.SnapshotRejected
				batch.QuarantineNote = strings.TrimSpace(input.Rationale)
			default:
				return fmt.Errorf("unsupported review decision: %w", domain.ErrValidation)
			}
			batch.UpdatedAt = now
			if err := tx.UpdateDatasetSnapshot(ctx, batch, batch.Version); err != nil {
				return err
			}
		}
		decision := domain.RiskDecision{ID: identity.New("decision"), DriftIncidentID: drift_incident.ID, Reviewer: principal.UserID, Decision: input.Decision, Rationale: strings.TrimSpace(input.Rationale), CreatedAt: now}
		if err := tx.InsertRiskDecision(ctx, decision); err != nil {
			return err
		}
		result = drift_incident
		return s.audit.Record(ctx, tx, "drift_incident_decided", "drift_incident", drift_incident.ID, "success", map[string]string{"decision": string(input.Decision)})
	})
	return result, err
}

func (s *ReviewService) EnsureReviewable(ctx context.Context, drift_incidentID string) error {
	return s.store.Read(ctx, func(reader repository.Reader) error {
		drift_incident, err := reader.GetDriftIncident(ctx, drift_incidentID)
		if err != nil {
			return err
		}
		if drift_incident.Status != domain.DriftIncidentOpen && drift_incident.Status != domain.DriftIncidentReviewing {
			return errors.New("drift_incident is already decided")
		}
		return nil
	})
}
