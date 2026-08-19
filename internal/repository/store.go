package repository

import (
	"context"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
)

type Store interface {
	WithTx(ctx context.Context, fn func(Tx) error) error
	Read(ctx context.Context, fn func(Reader) error) error
	Ping(ctx context.Context) error
	Close() error
}

type Reader interface {
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	GetUser(ctx context.Context, id string) (domain.User, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error)
	GetWorkspace(ctx context.Context, id string) (domain.Workspace, error)
	GetDataZone(ctx context.Context, id string) (domain.DataZone, error)
	GetDatasetSnapshot(ctx context.Context, id string) (domain.DatasetSnapshot, error)
	GetComputePool(ctx context.Context, id string) (domain.ComputePool, error)
	GetInferenceRun(ctx context.Context, id string) (domain.InferenceRun, error)
	ListInferenceRunInputs(ctx context.Context, runID string) ([]domain.DatasetSnapshot, error)
	GetPendingApprovalTask(ctx context.Context, runID string) (domain.ApprovalTask, error)
	GetApprovalTask(ctx context.Context, id string) (domain.ApprovalTask, error)
	GetActiveDriftIncident(ctx context.Context, runID string) (domain.DriftIncident, error)
	GetDriftIncident(ctx context.Context, id string) (domain.DriftIncident, error)
	GetRunReadiness(ctx context.Context, runID string) (domain.RunReadiness, error)
	GetPlatformSummary(ctx context.Context) (PlatformSummary, error)
	ListInferenceRuns(ctx context.Context, filter InferenceRunFilter) (InferenceRunPage, error)
	ListSnapshots(ctx context.Context, filter SnapshotFilter) (SnapshotPage, error)
	ListDriftIncidents(ctx context.Context, filter DriftIncidentFilter) (DriftIncidentPage, error)
	ListAuditEvents(ctx context.Context, filter AuditFilter) (AuditPage, error)
	GetIdempotency(ctx context.Context, scope, key string) (IdempotencyRecord, error)
	CountDataZoneInferenceRunsForBusinessDay(ctx context.Context, data_zoneID string, startAt, endAt time.Time) (int, error)
}

type Tx interface {
	Reader
	InsertUser(ctx context.Context, user domain.User) error
	InsertSession(ctx context.Context, session domain.Session) error
	RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error
	InsertWorkspace(ctx context.Context, workspace domain.Workspace) error
	UpdateWorkspace(ctx context.Context, workspace domain.Workspace, expectedVersion int64) error
	InsertDataZone(ctx context.Context, data_zone domain.DataZone) error
	InsertDatasetSnapshot(ctx context.Context, batch domain.DatasetSnapshot) error
	UpdateDatasetSnapshot(ctx context.Context, batch domain.DatasetSnapshot, expectedVersion int64) error
	InsertComputePool(ctx context.Context, compute_pool domain.ComputePool) error
	UpdateComputePool(ctx context.Context, compute_pool domain.ComputePool, expectedVersion int64) error
	InsertInferenceRun(ctx context.Context, run domain.InferenceRun) error
	UpdateInferenceRun(ctx context.Context, run domain.InferenceRun, expectedVersion int64) error
	InsertInferenceRunInput(ctx context.Context, item domain.InferenceRunInput) error
	DeleteInferenceRunInputs(ctx context.Context, runID string) error
	InsertApprovalTask(ctx context.Context, approval_task domain.ApprovalTask) error
	UpdateApprovalTask(ctx context.Context, approval_task domain.ApprovalTask, expectedVersion int64) error
	InsertObservation(ctx context.Context, observation domain.QualityObservation) error
	InsertDriftIncident(ctx context.Context, drift_incident domain.DriftIncident) error
	UpdateDriftIncident(ctx context.Context, drift_incident domain.DriftIncident, expectedVersion int64) error
	InsertRiskDecision(ctx context.Context, decision domain.RiskDecision) error
	InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error
	PutIdempotency(ctx context.Context, record IdempotencyRecord) error
	InsertJob(ctx context.Context, job domain.OutboxJob) error
	ClaimJobs(ctx context.Context, now time.Time, limit int) ([]domain.OutboxJob, error)
	CompleteJob(ctx context.Context, id string, now time.Time) error
	RetryJob(ctx context.Context, id string, availableAt time.Time, lastError string, dead bool) error
	ExpireApprovalTasks(ctx context.Context, now time.Time, limit int) ([]domain.ApprovalTask, error)
}

type PageRequest struct {
	Limit  int
	Offset int
	Sort   string
	Desc   bool
}

func (p PageRequest) Normalize(max int) PageRequest {
	if p.Limit < 1 {
		p.Limit = 50
	}
	if p.Limit > max {
		p.Limit = max
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

type InferenceRunFilter struct {
	Page         PageRequest
	WorkspaceID  string
	SourceZoneID string
	TargetZoneID string
	State        domain.InferenceRunState
	From         *time.Time
	To           *time.Time
}

type InferenceRunPage struct {
	Items []domain.InferenceRun
	Total int
}

type SnapshotFilter struct {
	Page           PageRequest
	WorkspaceID    string
	DataZoneID     string
	InferenceRunID string
	State          domain.SnapshotState
	ExpiresBy      *time.Time
}

type SnapshotPage struct {
	Items []domain.DatasetSnapshot
	Total int
}

type DriftIncidentFilter struct {
	Page           PageRequest
	InferenceRunID string
	Status         domain.DriftIncidentStatus
	DueBefore      *time.Time
}

type DriftIncidentPage struct {
	Items []domain.DriftIncident
	Total int
}

type AuditFilter struct {
	Page       PageRequest
	EntityType string
	EntityID   string
	Actor      string
	RequestID  string
}

type AuditPage struct {
	Items []domain.AuditEvent
	Total int
}

type PlatformSummary struct {
	WorkspacesActive       int `json:"workspaces_active"`
	SnapshotsValidated     int `json:"dataset_snapshots_validated"`
	SnapshotsMaterializing int `json:"dataset_snapshots_materializing"`
	SnapshotsQuarantined   int `json:"dataset_snapshots_quarantined"`
	ComputePoolsAvailable  int `json:"compute_pools_available"`
	InferenceRunsActive    int `json:"inference_runs_active"`
	OpenDriftIncidents     int `json:"open_drift_incidents"`
	PendingApprovalTasks   int `json:"pending_approval_tasks"`
	FailedJobs             int `json:"failed_jobs"`
}

type IdempotencyRecord struct {
	Scope        string
	Key          string
	RequestHash  string
	ResponseCode int
	ResponseBody []byte
	ExpiresAt    time.Time
	CreatedAt    time.Time
}
