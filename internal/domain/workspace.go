package domain

import (
	"strings"
	"time"
)

type WorkspaceStatus string

const (
	WorkspaceDraft    WorkspaceStatus = "draft"
	WorkspaceActive   WorkspaceStatus = "active"
	WorkspaceArchived WorkspaceStatus = "archived"
)

type Workspace struct {
	ID               string          `json:"id"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	Status           WorkspaceStatus `json:"status"`
	Score            QualityRange    `json:"score"`
	MaxExecution     time.Duration   `json:"max_execution"`
	ReviewDeadline   time.Duration   `json:"review_deadline"`
	BusinessTimezone string          `json:"business_timezone"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	Version          int64           `json:"version"`
}

func (s Workspace) Validate() error {
	if strings.TrimSpace(s.Code) == "" {
		return FieldError{Field: "code", Message: "is required"}
	}
	if strings.TrimSpace(s.Name) == "" {
		return FieldError{Field: "name", Message: "is required"}
	}
	if err := s.Score.Validate(); err != nil {
		return err
	}
	if s.MaxExecution <= 0 || s.MaxExecution > 14*24*time.Hour {
		return FieldError{Field: "max_execution", Message: "must be between zero and fourteen days"}
	}
	if s.ReviewDeadline <= 0 || s.ReviewDeadline > 7*24*time.Hour {
		return FieldError{Field: "review_deadline", Message: "must be between zero and seven days"}
	}
	if _, err := time.LoadLocation(s.BusinessTimezone); err != nil {
		return FieldError{Field: "business_timezone", Message: "is invalid"}
	}
	switch s.Status {
	case WorkspaceDraft, WorkspaceActive, WorkspaceArchived:
		return nil
	default:
		return FieldError{Field: "status", Message: "is invalid"}
	}
}

func (s Workspace) CanAcceptInferenceRuns() bool { return s.Status == WorkspaceActive }

func (s Workspace) ExecutionWithinLimit(startAt, finishAt time.Time) bool {
	return finishAt.After(startAt) && finishAt.Sub(startAt) <= s.MaxExecution
}

func (s Workspace) IsClosed() bool { return s.Status == WorkspaceArchived }
