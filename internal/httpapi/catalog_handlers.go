package httpapi

import (
	"net/http"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
)

type createWorkspaceRequest struct {
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	MinimumScore      float64 `json:"minimum_score"`
	MaximumScore      float64 `json:"maximum_score"`
	MaxExecutionHours int     `json:"max_execution_hours"`
	ReviewHours       int     `json:"review_hours"`
	BusinessTimezone  string  `json:"business_timezone"`
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var input createWorkspaceRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	minimum, err := domain.ScoreFromFloat(input.MinimumScore)
	if err != nil {
		writeError(w, r, err)
		return
	}
	maximum, err := domain.ScoreFromFloat(input.MaximumScore)
	if err != nil {
		writeError(w, r, err)
		return
	}
	rangeValue, err := domain.NewQualityRange(minimum, maximum)
	if err != nil {
		writeError(w, r, err)
		return
	}
	workspace, err := s.services.Catalog.CreateWorkspace(r.Context(), domain.Workspace{Code: input.Code, Name: input.Name, Score: rangeValue, MaxExecution: time.Duration(input.MaxExecutionHours) * time.Hour, ReviewDeadline: time.Duration(input.ReviewHours) * time.Hour, BusinessTimezone: input.BusinessTimezone})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, workspace)
}

func (s *Server) activateWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, err := s.services.Catalog.ActivateWorkspace(r.Context(), parseID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

type createDataZoneRequest struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Timezone   string `json:"timezone"`
	DailyLimit int    `json:"daily_limit"`
	CutoffHour int    `json:"cutoff_hour"`
}

func (s *Server) createDataZone(w http.ResponseWriter, r *http.Request) {
	var input createDataZoneRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	data_zone, err := s.services.Catalog.CreateDataZone(r.Context(), domain.DataZone{Code: input.Code, Name: input.Name, Timezone: input.Timezone, DailyLimit: input.DailyLimit, CutoffHour: input.CutoffHour})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, data_zone)
}

type createComputePoolRequest struct {
	SerialNumber     string    `json:"serial_number"`
	CapacityRows     int       `json:"capacity_rows"`
	AttestationDueAt time.Time `json:"attestation_due_at"`
	LastReconciledAt time.Time `json:"last_reconciled_at"`
}

func (s *Server) createComputePool(w http.ResponseWriter, r *http.Request) {
	var input createComputePoolRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	compute_pool, err := s.services.Catalog.CreateComputePool(r.Context(), domain.ComputePool{SerialNumber: input.SerialNumber, CapacityRows: input.CapacityRows, AttestationDueAt: input.AttestationDueAt, LastReconciledAt: input.LastReconciledAt})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, compute_pool)
}

type registerSnapshotRequest struct {
	WorkspaceID    string    `json:"workspace_id"`
	SourceZoneID   string    `json:"source_zone_id"`
	SourceRevision string    `json:"source_revision"`
	SchemaFamily   string    `json:"schema_family"`
	PartitionCount int       `json:"partition_count"`
	EstimatedRows  int       `json:"estimated_rows"`
	ExpiresAt      time.Time `json:"expires_at"`
}

func (s *Server) registerSnapshot(w http.ResponseWriter, r *http.Request) {
	var input registerSnapshotRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	batch, err := s.services.Catalog.RegisterSnapshot(r.Context(), domain.DatasetSnapshot{WorkspaceID: input.WorkspaceID, SourceZoneID: input.SourceZoneID, SourceRevision: input.SourceRevision, SchemaFamily: input.SchemaFamily, PartitionCount: input.PartitionCount, EstimatedRows: input.EstimatedRows, ExpiresAt: input.ExpiresAt})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, batch)
}

func (s *Server) validateSnapshot(w http.ResponseWriter, r *http.Request) {
	batch, err := s.services.Catalog.ValidateSnapshot(r.Context(), parseID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}
