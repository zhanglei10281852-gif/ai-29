package domain

import "time"

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobDead      JobStatus = "dead"
)

func (s JobStatus) IsTerminal() bool {
	return s == JobSucceeded || s == JobDead
}

func (s JobStatus) CanRetry() bool { return s == JobPending || s == JobFailed }

type OutboxJob struct {
	ID          string
	Kind        string
	AggregateID string
	Payload     []byte
	Status      JobStatus
	Attempts    int
	MaxAttempts int
	AvailableAt time.Time
	LockedAt    *time.Time
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (j OutboxJob) Clone() OutboxJob {
	clone := j
	clone.Payload = append([]byte(nil), j.Payload...)
	return clone
}
