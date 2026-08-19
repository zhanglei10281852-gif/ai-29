package service

import (
	"context"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

type QueryService struct{ dependencies }

func (s *QueryService) PlatformSummary(ctx context.Context) (repository.PlatformSummary, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireAction(principal, domain.ActionReadPlatform); err != nil {
		return repository.PlatformSummary{}, err
	}
	var summary repository.PlatformSummary
	err := s.store.Read(ctx, func(reader repository.Reader) error {
		var err error
		summary, err = reader.GetPlatformSummary(ctx)
		return err
	})
	return summary, err
}

func (s *QueryService) InferenceRun(ctx context.Context, id string) (domain.InferenceRun, []domain.DatasetSnapshot, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer, domain.RoleDataEngineer, domain.RoleRiskReviewer, domain.RoleComplianceAuditor); err != nil {
		return domain.InferenceRun{}, nil, err
	}
	var run domain.InferenceRun
	var items []domain.DatasetSnapshot
	err := s.store.Read(ctx, func(reader repository.Reader) error {
		var err error
		run, err = reader.GetInferenceRun(ctx, id)
		if err != nil {
			return err
		}
		items, err = reader.ListInferenceRunInputs(ctx, id)
		return err
	})
	return run, items, err
}

func (s *QueryService) InferenceRuns(ctx context.Context, filter repository.InferenceRunFilter) (repository.InferenceRunPage, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer, domain.RoleDataEngineer, domain.RoleRiskReviewer, domain.RoleComplianceAuditor); err != nil {
		return repository.InferenceRunPage{}, err
	}
	var page repository.InferenceRunPage
	err := s.store.Read(ctx, func(reader repository.Reader) error {
		var err error
		page, err = reader.ListInferenceRuns(ctx, filter)
		return err
	})
	return page, err
}

func (s *QueryService) Snapshots(ctx context.Context, filter repository.SnapshotFilter) (repository.SnapshotPage, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer, domain.RoleDataEngineer, domain.RoleRiskReviewer, domain.RoleComplianceAuditor); err != nil {
		return repository.SnapshotPage{}, err
	}
	var page repository.SnapshotPage
	err := s.store.Read(ctx, func(reader repository.Reader) error {
		var err error
		page, err = reader.ListSnapshots(ctx, filter)
		return err
	})
	return page, err
}

func (s *QueryService) DriftIncidents(ctx context.Context, filter repository.DriftIncidentFilter) (repository.DriftIncidentPage, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer, domain.RoleRiskReviewer, domain.RoleComplianceAuditor); err != nil {
		return repository.DriftIncidentPage{}, err
	}
	var page repository.DriftIncidentPage
	err := s.store.Read(ctx, func(reader repository.Reader) error {
		var err error
		page, err = reader.ListDriftIncidents(ctx, filter)
		return err
	})
	return page, err
}

func (s *QueryService) Audit(ctx context.Context, filter repository.AuditFilter) (repository.AuditPage, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleComplianceAuditor, domain.RoleMLEngineer); err != nil {
		return repository.AuditPage{}, err
	}
	var page repository.AuditPage
	err := s.store.Read(ctx, func(reader repository.Reader) error {
		var err error
		page, err = reader.ListAuditEvents(ctx, filter)
		return err
	})
	return page, err
}
