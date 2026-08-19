package httpapi

import (
	"net/http"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
)

type bulkRegisterRequest struct {
	Snapshots []registerSnapshotRequest `json:"snapshots"`
}

func (s *Server) bulkRegisterSnapshots(w http.ResponseWriter, r *http.Request) {
	var input bulkRegisterRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	batches := make([]domain.DatasetSnapshot, 0, len(input.Snapshots))
	for _, item := range input.Snapshots {
		batches = append(batches, domain.DatasetSnapshot{WorkspaceID: item.WorkspaceID, SourceZoneID: item.SourceZoneID, SourceRevision: item.SourceRevision, SchemaFamily: item.SchemaFamily, PartitionCount: item.PartitionCount, EstimatedRows: item.EstimatedRows, ExpiresAt: item.ExpiresAt})
	}
	result, err := s.services.Catalog.BulkRegisterSnapshots(r.Context(), batches)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if result.Failed > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, result)
}

func (s *Server) startComputePoolReconciling(w http.ResponseWriter, r *http.Request) {
	compute_pool, err := s.services.ComputePools.StartReconciliation(r.Context(), parseID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, compute_pool)
}

func (s *Server) completeComputePoolReconciling(w http.ResponseWriter, r *http.Request) {
	compute_pool, err := s.services.ComputePools.CompleteReconciliation(r.Context(), parseID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, compute_pool)
}

func (s *Server) retireComputePool(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	compute_pool, err := s.services.ComputePools.Retire(r.Context(), parseID(r), input.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, compute_pool)
}
