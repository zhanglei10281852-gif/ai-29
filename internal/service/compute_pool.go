package service

import (
	"context"
	"strings"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

type ComputePoolService struct{ dependencies }

func (s *ComputePoolService) StartReconciliation(ctx context.Context, compute_poolID string) (domain.ComputePool, error) {
	return s.change(ctx, compute_poolID, "compute_pool_reconciliation_started", func(compute_pool *domain.ComputePool) error {
		return compute_pool.StartReconciliation(s.clock.Now())
	})
}

func (s *ComputePoolService) CompleteReconciliation(ctx context.Context, compute_poolID string) (domain.ComputePool, error) {
	return s.change(ctx, compute_poolID, "compute_pool_reconciliation_completed", func(compute_pool *domain.ComputePool) error {
		return compute_pool.CompleteReconciliation(s.clock.Now())
	})
}

func (s *ComputePoolService) Retire(ctx context.Context, compute_poolID, reason string) (domain.ComputePool, error) {
	if strings.TrimSpace(reason) == "" {
		return domain.ComputePool{}, domain.FieldError{Field: "reason", Message: "is required"}
	}
	return s.changeWithMetadata(ctx, compute_poolID, "compute_pool_retired", map[string]string{"reason": strings.TrimSpace(reason)}, func(compute_pool *domain.ComputePool) error {
		return compute_pool.Retire(s.clock.Now())
	})
}

func (s *ComputePoolService) change(ctx context.Context, compute_poolID, action string, mutate func(*domain.ComputePool) error) (domain.ComputePool, error) {
	return s.changeWithMetadata(ctx, compute_poolID, action, nil, mutate)
}

func (s *ComputePoolService) changeWithMetadata(ctx context.Context, compute_poolID, action string, metadata map[string]string, mutate func(*domain.ComputePool) error) (domain.ComputePool, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.ComputePool{}, err
	}
	var result domain.ComputePool
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		compute_pool, err := tx.GetComputePool(ctx, compute_poolID)
		if err != nil {
			return err
		}
		before := compute_pool.Version
		if err := mutate(&compute_pool); err != nil {
			return err
		}
		if err := tx.UpdateComputePool(ctx, compute_pool, before); err != nil {
			return err
		}
		result = compute_pool
		return s.audit.Record(ctx, tx, action, "compute_pool", compute_pool.ID, "success", metadata)
	})
	return result, err
}
