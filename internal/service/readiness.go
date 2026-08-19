package service

import (
	"context"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

func (s *QueryService) ReconcileInferenceRun(ctx context.Context, runID string) (domain.RunReadiness, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer, domain.RoleDataEngineer, domain.RoleRiskReviewer, domain.RoleComplianceAuditor); err != nil {
		return domain.RunReadiness{}, err
	}
	var report domain.RunReadiness
	err := s.store.Read(ctx, func(reader repository.Reader) error {
		var err error
		report, err = reader.GetRunReadiness(ctx, runID)
		return err
	})
	return report, err
}
