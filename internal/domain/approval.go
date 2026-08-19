package domain

import (
	"strings"
	"time"
)

type ApprovalTaskStatus string

const (
	ApprovalTaskPending  ApprovalTaskStatus = "pending"
	ApprovalTaskAccepted ApprovalTaskStatus = "accepted"
	ApprovalTaskRejected ApprovalTaskStatus = "rejected"
	ApprovalTaskExpired  ApprovalTaskStatus = "expired"
)

func (s ApprovalTaskStatus) IsResolved() bool {
	return s == ApprovalTaskAccepted || s == ApprovalTaskRejected || s == ApprovalTaskExpired
}

func (s ApprovalTaskStatus) IsPending() bool { return s == ApprovalTaskPending }

type ApprovalTask struct {
	ID             string             `json:"id"`
	InferenceRunID string             `json:"run_id"`
	RequesterID    string             `json:"requester_id"`
	ReviewerID     string             `json:"reviewer_id"`
	ReviewQueue    string             `json:"review_queue"`
	Status         ApprovalTaskStatus `json:"status"`
	ExpiresAt      time.Time          `json:"expires_at"`
	ResolvedAt     *time.Time         `json:"resolved_at,omitempty"`
	ResolutionNote string             `json:"resolution_note,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Version        int64              `json:"version"`
}

func (h ApprovalTask) Validate() error {
	if strings.TrimSpace(h.InferenceRunID) == "" || strings.TrimSpace(h.RequesterID) == "" || strings.TrimSpace(h.ReviewerID) == "" {
		return FieldError{Field: "approval_task", Message: "run, requester and reviewer are required"}
	}
	if h.RequesterID == h.ReviewerID {
		return FieldError{Field: "reviewer_id", Message: "must differ from requester"}
	}
	if strings.TrimSpace(h.ReviewQueue) == "" || h.ExpiresAt.IsZero() {
		return FieldError{Field: "approval_task", Message: "review_queue and expiry are required"}
	}
	switch h.Status {
	case ApprovalTaskPending, ApprovalTaskAccepted, ApprovalTaskRejected, ApprovalTaskExpired:
		return nil
	default:
		return FieldError{Field: "approval_task_status", Message: "is invalid"}
	}
}

func (h *ApprovalTask) Resolve(status ApprovalTaskStatus, note string, now time.Time) error {
	if h.Status != ApprovalTaskPending {
		return TransitionError{Entity: "approval_task", From: string(h.Status), To: string(status)}
	}
	if !now.Before(h.ExpiresAt) && status != ApprovalTaskExpired {
		return ErrExpired
	}
	if status != ApprovalTaskAccepted && status != ApprovalTaskRejected && status != ApprovalTaskExpired {
		return FieldError{Field: "approval_task_status", Message: "unsupported resolution"}
	}
	now = now.UTC()
	h.Status = status
	h.ResolutionNote = strings.TrimSpace(note)
	h.ResolvedAt = &now
	h.UpdatedAt = now
	return nil
}
