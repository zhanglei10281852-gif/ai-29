package domain

import (
	"strings"
	"time"
)

type DriftIncidentStatus string

const (
	DriftIncidentOpen      DriftIncidentStatus = "open"
	DriftIncidentReviewing DriftIncidentStatus = "reviewing"
	DriftIncidentCleared   DriftIncidentStatus = "cleared"
	DriftIncidentRejected  DriftIncidentStatus = "rejected"
)

type QualityObservation struct {
	ID             string     `json:"id"`
	InferenceRunID string     `json:"run_id"`
	MetricKey      string     `json:"metric_key"`
	Sequence       int64      `json:"sequence"`
	Score          MilliScore `json:"score_millis"`
	RecordedAt     time.Time  `json:"recorded_at"`
	ReceivedAt     time.Time  `json:"received_at"`
}

type DriftIncident struct {
	ID                 string              `json:"id"`
	InferenceRunID     string              `json:"run_id"`
	Status             DriftIncidentStatus `json:"status"`
	FirstObservationAt time.Time           `json:"first_observation_at"`
	LastObservationAt  time.Time           `json:"last_observation_at"`
	Minimum            MilliScore          `json:"minimum_score_millis"`
	Maximum            MilliScore          `json:"maximum_score_millis"`
	ObservationCount   int                 `json:"observation_count"`
	ReviewDueAt        time.Time           `json:"review_due_at"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	Version            int64               `json:"version"`
}

type RiskDecision struct {
	ID              string
	DriftIncidentID string
	Reviewer        string
	Decision        DriftIncidentStatus
	Rationale       string
	CreatedAt       time.Time
}

func (s DriftIncidentStatus) IsResolved() bool {
	return s == DriftIncidentCleared || s == DriftIncidentRejected
}

func (s DriftIncidentStatus) IsOpen() bool {
	return s == DriftIncidentOpen || s == DriftIncidentReviewing
}

func (r QualityObservation) Validate() error {
	if strings.TrimSpace(r.InferenceRunID) == "" || strings.TrimSpace(r.MetricKey) == "" {
		return FieldError{Field: "observation", Message: "run and metric key are required"}
	}
	if r.Sequence < 1 || r.RecordedAt.IsZero() {
		return FieldError{Field: "observation", Message: "sequence and recorded_at are required"}
	}
	return nil
}

func (e *DriftIncident) Include(observation QualityObservation, now time.Time) {
	if e.ObservationCount == 0 || observation.RecordedAt.Before(e.FirstObservationAt) {
		e.FirstObservationAt = observation.RecordedAt
	}
	if e.ObservationCount == 0 || observation.RecordedAt.After(e.LastObservationAt) {
		e.LastObservationAt = observation.RecordedAt
	}
	if e.ObservationCount == 0 || observation.Score < e.Minimum {
		e.Minimum = observation.Score
	}
	if e.ObservationCount == 0 || observation.Score > e.Maximum {
		e.Maximum = observation.Score
	}
	e.ObservationCount++
	e.UpdatedAt = now.UTC()
}

func (e *DriftIncident) StartReview(now time.Time) error {
	if e.Status != DriftIncidentOpen {
		return TransitionError{Entity: "drift_incident", From: string(e.Status), To: string(DriftIncidentReviewing)}
	}
	e.Status = DriftIncidentReviewing
	e.UpdatedAt = now.UTC()
	return nil
}

func (e *DriftIncident) Decide(decision DriftIncidentStatus, now time.Time) error {
	if e.Status != DriftIncidentOpen && e.Status != DriftIncidentReviewing {
		return TransitionError{Entity: "drift_incident", From: string(e.Status), To: string(decision)}
	}
	if decision != DriftIncidentCleared && decision != DriftIncidentRejected {
		return FieldError{Field: "decision", Message: "must be cleared or rejected"}
	}
	e.Status = decision
	e.UpdatedAt = now.UTC()
	return nil
}
