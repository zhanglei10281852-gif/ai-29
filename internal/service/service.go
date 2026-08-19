package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/audit"
	"github.com/zhanglei10281852-gif/ai/internal/clock"
	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
)

type Services struct {
	Auth         *AuthService
	Catalog      *CatalogService
	ComputePools *ComputePoolService
	Inference    *InferenceService
	Approval     *ApprovalService
	Metrics      *MetricsService
	Review       *ReviewService
	Query        *QueryService
}

type dependencies struct {
	store repository.Store
	clock clock.Clock
	audit audit.Recorder
}

func New(store repository.Store, c clock.Clock, sessionTTL, approval_taskTTL time.Duration) *Services {
	deps := dependencies{store: store, clock: c, audit: audit.NewRecorder(c)}
	return &Services{
		Auth:         &AuthService{dependencies: deps, sessionTTL: sessionTTL},
		Catalog:      &CatalogService{dependencies: deps},
		ComputePools: &ComputePoolService{dependencies: deps},
		Inference:    &InferenceService{dependencies: deps},
		Approval:     &ApprovalService{dependencies: deps, approval_taskTTL: approval_taskTTL},
		Metrics:      &MetricsService{dependencies: deps},
		Review:       &ReviewService{dependencies: deps},
		Query:        &QueryService{dependencies: deps},
	}
}

func requireRole(principal domain.Principal, roles ...domain.Role) error {
	if principal.UserID == "" {
		return fmt.Errorf("authentication required: %w", domain.ErrUnauthenticated)
	}
	if !principal.Can(roles...) {
		return fmt.Errorf("role %s is not permitted: %w", principal.Role, domain.ErrForbidden)
	}
	return nil
}

func requireAction(principal domain.Principal, action domain.Action) error {
	if principal.UserID == "" {
		return fmt.Errorf("authentication required: %w", domain.ErrUnauthenticated)
	}
	if !principal.CanAction(action) {
		return fmt.Errorf("role %s cannot perform %s: %w", principal.Role, action, domain.ErrForbidden)
	}
	return nil
}

func isNotFound(err error) bool { return errors.Is(err, domain.ErrNotFound) }
