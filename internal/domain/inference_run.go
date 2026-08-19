package domain

import (
	"strings"
	"time"
)

type InferenceRunState string

const (
	InferenceRunQueued    InferenceRunState = "queued"
	InferenceRunStaged    InferenceRunState = "staged"
	InferenceRunRunning   InferenceRunState = "running"
	InferenceRunCompleted InferenceRunState = "completed"
	InferenceRunArchived  InferenceRunState = "archived"
	InferenceRunCancelled InferenceRunState = "cancelled"
)

type InferenceRun struct {
	ID                 string            `json:"id"`
	WorkspaceID        string            `json:"workspace_id"`
	SourceZoneID       string            `json:"source_zone_id"`
	TargetZoneID       string            `json:"target_zone_id"`
	ComputePoolID      string            `json:"compute_pool_id"`
	Reference          string            `json:"reference"`
	State              InferenceRunState `json:"state"`
	ScheduledStartAt   time.Time         `json:"scheduled_start_at"`
	ExpectedFinishAt   time.Time         `json:"expected_finish_at"`
	StartedAt          *time.Time        `json:"started_at,omitempty"`
	CompletedAt        *time.Time        `json:"completed_at,omitempty"`
	ArchivedAt         *time.Time        `json:"archived_at,omitempty"`
	TotalEstimatedRows int               `json:"total_estimated_rows"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	Version            int64             `json:"version"`
}

type InferenceRunInput struct {
	InferenceRunID string
	SnapshotID     string
	AddedAt        time.Time
}

func (s InferenceRun) Validate() error {
	if strings.TrimSpace(s.WorkspaceID) == "" || strings.TrimSpace(s.SourceZoneID) == "" || strings.TrimSpace(s.TargetZoneID) == "" {
		return FieldError{Field: "run", Message: "workspace, source zone and target zone are required"}
	}
	if s.SourceZoneID == s.TargetZoneID {
		return FieldError{Field: "target_zone_id", Message: "must differ from source zone"}
	}
	if strings.TrimSpace(s.Reference) == "" || strings.TrimSpace(s.ComputePoolID) == "" {
		return FieldError{Field: "run", Message: "reference and compute_pool are required"}
	}
	if !s.ExpectedFinishAt.After(s.ScheduledStartAt) {
		return FieldError{Field: "expected_finish_at", Message: "must be after scheduled start"}
	}
	if s.TotalEstimatedRows < 1 {
		return FieldError{Field: "total_estimated_rows", Message: "must be positive"}
	}
	return validateInferenceRunState(s.State)
}

func validateInferenceRunState(state InferenceRunState) error {
	switch state {
	case InferenceRunQueued, InferenceRunStaged, InferenceRunRunning, InferenceRunCompleted, InferenceRunArchived, InferenceRunCancelled:
		return nil
	default:
		return FieldError{Field: "run_state", Message: "is invalid"}
	}
}

func (s InferenceRunState) IsTerminal() bool {
	return s == InferenceRunArchived || s == InferenceRunCancelled
}

func (s *InferenceRun) Transition(to InferenceRunState, now time.Time) error {
	allowed := map[InferenceRunState]map[InferenceRunState]bool{
		InferenceRunQueued:    {InferenceRunStaged: true, InferenceRunCancelled: true},
		InferenceRunStaged:    {InferenceRunRunning: true, InferenceRunCancelled: true},
		InferenceRunRunning:   {InferenceRunCompleted: true},
		InferenceRunCompleted: {InferenceRunArchived: true},
	}
	if !allowed[s.State][to] {
		return TransitionError{Entity: "inference_run", From: string(s.State), To: string(to)}
	}
	now = now.UTC()
	switch to {
	case InferenceRunRunning:
		s.StartedAt = &now
	case InferenceRunCompleted:
		s.CompletedAt = &now
	case InferenceRunArchived:
		s.ArchivedAt = &now
	}
	s.State = to
	s.UpdatedAt = now
	return nil
}
