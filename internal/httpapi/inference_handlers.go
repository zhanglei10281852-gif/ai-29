package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
	"github.com/zhanglei10281852-gif/ai/internal/service"
)

type planInferenceRunRequest struct {
	WorkspaceID      string    `json:"workspace_id"`
	SourceZoneID     string    `json:"source_zone_id"`
	TargetZoneID     string    `json:"target_zone_id"`
	ComputePoolID    string    `json:"compute_pool_id"`
	Reference        string    `json:"reference"`
	SnapshotIDs      []string  `json:"snapshot_ids"`
	ScheduledStartAt time.Time `json:"scheduled_start_at"`
	ExpectedFinishAt time.Time `json:"expected_finish_at"`
}

func (s *Server) planInferenceRun(w http.ResponseWriter, r *http.Request) {
	var input planInferenceRunRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	run, err := s.services.Inference.PlanInferenceRun(r.Context(), service.PlanInferenceRunInput{WorkspaceID: input.WorkspaceID, SourceZoneID: input.SourceZoneID, TargetZoneID: input.TargetZoneID, ComputePoolID: input.ComputePoolID, Reference: input.Reference, SnapshotIDs: append([]string(nil), input.SnapshotIDs...), ScheduledStartAt: input.ScheduledStartAt, ExpectedFinishAt: input.ExpectedFinishAt, IdempotencyKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) getInferenceRun(w http.ResponseWriter, r *http.Request) {
	run, items, err := s.services.Query.InferenceRun(r.Context(), parseID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "items": items})
}

func (s *Server) reconcileInferenceRun(w http.ResponseWriter, r *http.Request) {
	report, err := s.services.Query.ReconcileInferenceRun(r.Context(), parseID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) listInferenceRuns(w http.ResponseWriter, r *http.Request) {
	from, err := parseTimeQuery(r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	to, err := parseTimeQuery(r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	page, err := s.services.Query.InferenceRuns(r.Context(), repository.InferenceRunFilter{Page: parsePage(r), WorkspaceID: r.URL.Query().Get("workspace_id"), SourceZoneID: r.URL.Query().Get("source_zone_id"), TargetZoneID: r.URL.Query().Get("target_zone_id"), State: domain.InferenceRunState(r.URL.Query().Get("state")), From: from, To: to})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) stageInferenceRun(w http.ResponseWriter, r *http.Request) {
	s.writeInferenceRunTransition(w, r, s.services.Inference.StageInferenceRun)
}
func (s *Server) startInferenceRun(w http.ResponseWriter, r *http.Request) {
	s.writeInferenceRunTransition(w, r, s.services.Inference.StartInferenceRun)
}
func (s *Server) completeInferenceRun(w http.ResponseWriter, r *http.Request) {
	s.writeInferenceRunTransition(w, r, s.services.Inference.CompleteInferenceRun)
}
func (s *Server) archiveInferenceRun(w http.ResponseWriter, r *http.Request) {
	s.writeInferenceRunTransition(w, r, s.services.Inference.ArchiveInferenceRun)
}

func (s *Server) writeInferenceRunTransition(w http.ResponseWriter, r *http.Request, transition func(context.Context, string) (domain.InferenceRun, error)) {
	run, err := transition(r.Context(), parseID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) cancelInferenceRun(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Note string `json:"note"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	run, err := s.services.Inference.CancelInferenceRun(r.Context(), parseID(r), input.Note)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
