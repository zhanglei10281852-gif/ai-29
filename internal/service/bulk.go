package service

import (
	"context"
	"errors"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
)

type BulkSnapshotItem struct {
	Index    int                     `json:"index"`
	Snapshot *domain.DatasetSnapshot `json:"batch,omitempty"`
	Error    string                  `json:"error,omitempty"`
	Code     string                  `json:"code"`
}

type BulkSnapshotResult struct {
	Items     []BulkSnapshotItem `json:"items"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
}

func (s *CatalogService) BulkRegisterSnapshots(ctx context.Context, batches []domain.DatasetSnapshot) (BulkSnapshotResult, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return BulkSnapshotResult{}, err
	}
	if len(batches) == 0 {
		return BulkSnapshotResult{}, domain.FieldError{Field: "batches", Message: "at least one batch is required"}
	}
	if len(batches) > 100 {
		return BulkSnapshotResult{}, domain.FieldError{Field: "batches", Message: "cannot contain more than 100 items"}
	}
	result := BulkSnapshotResult{Items: make([]BulkSnapshotItem, 0, len(batches))}
	for index, input := range batches {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		created, err := s.RegisterSnapshot(ctx, input.Clone())
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, BulkSnapshotItem{Index: index, Error: err.Error(), Code: classifyBulkError(err)})
			continue
		}
		result.Succeeded++
		createdCopy := created.Clone()
		result.Items = append(result.Items, BulkSnapshotItem{Index: index, Snapshot: &createdCopy, Code: "created"})
	}
	return result, nil
}

func classifyBulkError(err error) string {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return "invalid"
	case errors.Is(err, domain.ErrConflict):
		return "conflict"
	default:
		return "failed"
	}
}
