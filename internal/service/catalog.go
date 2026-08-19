package service

import (
	"context"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/identity"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
	"github.com/zhanglei10281852-gif/ai/internal/requestmeta"
)

type CatalogService struct{ dependencies }

func (s *CatalogService) CreateWorkspace(ctx context.Context, workspace domain.Workspace) (domain.Workspace, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.Workspace{}, err
	}
	now := s.clock.Now()
	workspace.ID = identity.New("workspace")
	workspace.Code = domain.NormalizeCode(workspace.Code)
	if err := domain.ValidateBusinessCode("code", workspace.Code); err != nil {
		return domain.Workspace{}, err
	}
	workspace.Status = domain.WorkspaceDraft
	workspace.Version = 1
	workspace.CreatedAt, workspace.UpdatedAt = now, now
	if err := workspace.Validate(); err != nil {
		return domain.Workspace{}, err
	}
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertWorkspace(ctx, workspace); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, "workspace_created", "workspace", workspace.ID, "success", nil)
	})
	return workspace, err
}

func (s *CatalogService) ActivateWorkspace(ctx context.Context, workspaceID string) (domain.Workspace, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.Workspace{}, err
	}
	var result domain.Workspace
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		workspace, err := tx.GetWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		if workspace.Status != domain.WorkspaceDraft {
			return domain.TransitionError{Entity: "workspace", From: string(workspace.Status), To: string(domain.WorkspaceActive)}
		}
		before := workspace.Version
		workspace.Status = domain.WorkspaceActive
		workspace.UpdatedAt = s.clock.Now()
		if err := tx.UpdateWorkspace(ctx, workspace, before); err != nil {
			return err
		}
		result = workspace
		return s.audit.Record(ctx, tx, "workspace_activated", "workspace", workspace.ID, "success", nil)
	})
	return result, err
}

func (s *CatalogService) CreateDataZone(ctx context.Context, data_zone domain.DataZone) (domain.DataZone, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.DataZone{}, err
	}
	now := s.clock.Now()
	data_zone.ID = identity.New("data_zone")
	data_zone.Code = domain.NormalizeCode(data_zone.Code)
	if err := domain.ValidateBusinessCode("code", data_zone.Code); err != nil {
		return domain.DataZone{}, err
	}
	data_zone.Status = domain.DataZoneActive
	data_zone.Version = 1
	data_zone.CreatedAt, data_zone.UpdatedAt = now, now
	if err := data_zone.Validate(); err != nil {
		return domain.DataZone{}, err
	}
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertDataZone(ctx, data_zone); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, "data_zone_created", "data_zone", data_zone.ID, "success", nil)
	})
	return data_zone, err
}

func (s *CatalogService) CreateComputePool(ctx context.Context, compute_pool domain.ComputePool) (domain.ComputePool, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.ComputePool{}, err
	}
	now := s.clock.Now()
	compute_pool.ID = identity.New("pool")
	compute_pool.SerialNumber = domain.NormalizeCode(compute_pool.SerialNumber)
	if err := domain.ValidateBusinessCode("serial_number", compute_pool.SerialNumber); err != nil {
		return domain.ComputePool{}, err
	}
	compute_pool.State = domain.ComputePoolAvailable
	compute_pool.Version = 1
	compute_pool.CreatedAt, compute_pool.UpdatedAt = now, now
	if err := compute_pool.Validate(); err != nil {
		return domain.ComputePool{}, err
	}
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertComputePool(ctx, compute_pool); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, "compute_pool_created", "compute_pool", compute_pool.ID, "success", nil)
	})
	return compute_pool, err
}

func (s *CatalogService) RegisterSnapshot(ctx context.Context, batch domain.DatasetSnapshot) (domain.DatasetSnapshot, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.DatasetSnapshot{}, err
	}
	now := s.clock.Now()
	batch.ID = identity.New("snapshot")
	batch.State = domain.SnapshotRegistered
	batch.Version = 1
	batch.CreatedAt, batch.UpdatedAt = now, now
	if err := batch.Validate(); err != nil {
		return domain.DatasetSnapshot{}, err
	}
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertDatasetSnapshot(ctx, batch); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, "snapshot_registered", "dataset_snapshot", batch.ID, "success", nil)
	})
	return batch, err
}

func (s *CatalogService) ValidateSnapshot(ctx context.Context, snapshotID string) (domain.DatasetSnapshot, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.DatasetSnapshot{}, err
	}
	var result domain.DatasetSnapshot
	err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		batch, err := tx.GetDatasetSnapshot(ctx, snapshotID)
		if err != nil {
			return err
		}
		if err := batch.Transition(domain.SnapshotValidated, s.clock.Now()); err != nil {
			return err
		}
		if err := tx.UpdateDatasetSnapshot(ctx, batch, batch.Version); err != nil {
			return err
		}
		result = batch
		return s.audit.Record(ctx, tx, "snapshot_validated", "dataset_snapshot", batch.ID, "success", nil)
	})
	return result, err
}

func principalOrEmpty(ctx context.Context) (domain.Principal, bool) {
	principal, ok := requestmeta.Principal(ctx)
	return principal, ok
}
