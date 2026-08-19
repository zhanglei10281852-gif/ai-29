package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	requestmiddleware "github.com/zhanglei10281852-gif/ai/internal/middleware"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
	"github.com/zhanglei10281852-gif/ai/internal/requestmeta"
	"github.com/zhanglei10281852-gif/ai/internal/service"
)

func TestUnknownBearerTokenKeepsAuthenticationStatus(t *testing.T) {
	f := newHTTPFixture(t)
	response := f.request(t, http.MethodGet, "/api/v1/summary", nil, "token-that-does-not-exist")
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
	body := readResponse(t, response)
	errorBody, ok := body["error"].(map[string]any)
	if !ok || errorBody["code"] != "unauthenticated" {
		t.Fatalf("error response = %+v", body)
	}
}

func TestRequestScopeRetainsClientLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil).WithContext(ctx)
	var downstreamErr error
	handler := requestmiddleware.RequestContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstreamErr = r.Context().Err()
		w.WriteHeader(http.StatusNoContent)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if !errors.Is(downstreamErr, context.Canceled) {
		t.Fatalf("downstream context error = %v, want canceled", downstreamErr)
	}
}

func TestObservationScoreUsesWorkspaceScale(t *testing.T) {
	f := newHTTPFixture(t)
	login, err := f.services.Auth.Login(context.Background(), service.LoginInput{
		Email: "ops@example.test", Password: "very-secure-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	run := startHTTPObservationRun(t, f, login.Principal)
	recordedAt := time.Date(2026, 8, 18, 8, 15, 0, 0, time.UTC)
	response := f.request(t, http.MethodPost, "/api/v1/inference-runs/"+run.ID+"/observations", map[string]any{
		"metric_key": "policy-quality", "sequence": 1, "score": 5.0, "recorded_at": recordedAt,
	}, login.Token)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, body = %+v", response.StatusCode, readResponse(t, response))
	}
	body := readResponse(t, response)
	if body["drift_incident"] != nil {
		t.Fatalf("in-range score opened drift incident: %+v", body["drift_incident"])
	}
}

func TestComputePoolPreservesAttestationInstant(t *testing.T) {
	f := newHTTPFixture(t)
	login, err := f.services.Auth.Login(context.Background(), service.LoginInput{
		Email: "ops@example.test", Password: "very-secure-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+8", 8*60*60)
	dueAt := time.Date(2026, 8, 20, 10, 0, 0, 0, zone)
	reconciledAt := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	response := f.request(t, http.MethodPost, "/api/v1/compute-pools", map[string]any{
		"serial_number": "OFFSET-POOL", "capacity_rows": 1000,
		"attestation_due_at": dueAt, "last_reconciled_at": reconciledAt,
	}, login.Token)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, body = %+v", response.StatusCode, readResponse(t, response))
	}
	body := readResponse(t, response)
	stored, err := time.Parse(time.RFC3339, body["attestation_due_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Equal(dueAt) {
		t.Fatalf("attestation instant = %s, want %s", stored, dueAt)
	}
}

func TestBulkSnapshotResponseKeepsIndependentItems(t *testing.T) {
	f := newHTTPFixture(t)
	login, err := f.services.Auth.Login(context.Background(), service.LoginInput{
		Email: "ops@example.test", Password: "very-secure-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	principalCtx := requestmeta.WithPrincipal(context.Background(), login.Principal)
	minimum, _ := domain.ScoreFromFloat(2)
	maximum, _ := domain.ScoreFromFloat(8)
	score, _ := domain.NewQualityRange(minimum, maximum)
	workspace, err := f.services.Catalog.CreateWorkspace(principalCtx, domain.Workspace{
		Code: "BULK-HTTP", Name: "Bulk HTTP workspace", Score: score,
		MaxExecution: 4 * time.Hour, ReviewDeadline: time.Hour, BusinessTimezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = f.services.Catalog.ActivateWorkspace(principalCtx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	zone, err := f.services.Catalog.CreateDataZone(principalCtx, domain.DataZone{
		Code: "BULK-SOURCE", Name: "Bulk source", Timezone: "UTC", DailyLimit: 10, CutoffHour: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	response := f.request(t, http.MethodPost, "/api/v1/dataset-snapshots/bulk", map[string]any{
		"snapshots": []map[string]any{
			{"workspace_id": workspace.ID, "source_zone_id": zone.ID, "source_revision": "BULK-REV-A", "schema_family": "policy", "partition_count": 1, "estimated_rows": 20, "expires_at": time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)},
			{"workspace_id": workspace.ID, "source_zone_id": zone.ID, "source_revision": "BULK-REV-B", "schema_family": "policy", "partition_count": 1, "estimated_rows": 30, "expires_at": time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)},
		},
	}, login.Token)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("bulk status = %d, body = %+v", response.StatusCode, readResponse(t, response))
	}
	body := readResponse(t, response)
	items, ok := body["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("bulk items = %+v", body["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("first bulk item = %+v", items[0])
	}
	second, ok := items[1].(map[string]any)
	if !ok {
		t.Fatalf("second bulk item = %+v", items[1])
	}
	firstSnapshot, ok := first["batch"].(map[string]any)
	if !ok {
		t.Fatalf("first response snapshot = %+v", first)
	}
	secondSnapshot, ok := second["batch"].(map[string]any)
	if !ok {
		t.Fatalf("second response snapshot = %+v", second)
	}
	if firstSnapshot["source_revision"] != "BULK-REV-A" || secondSnapshot["source_revision"] != "BULK-REV-B" || firstSnapshot["id"] == secondSnapshot["id"] {
		t.Fatalf("bulk response snapshots = %+v / %+v", firstSnapshot, secondSnapshot)
	}

	page, err := f.services.Query.Snapshots(principalCtx, repository.SnapshotFilter{
		Page: repository.PageRequest{Limit: 10}, WorkspaceID: workspace.ID, DataZoneID: zone.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("persisted bulk snapshots = %+v", page)
	}
}

func startHTTPObservationRun(t *testing.T, f *httpFixture, principal domain.Principal) domain.InferenceRun {
	t.Helper()
	ctx := requestmeta.WithPrincipal(context.Background(), principal)
	minimum, _ := domain.ScoreFromFloat(2)
	maximum, _ := domain.ScoreFromFloat(8)
	rangeValue, _ := domain.NewQualityRange(minimum, maximum)
	workspace, err := f.services.Catalog.CreateWorkspace(ctx, domain.Workspace{
		Code: "HTTP-SCORE", Name: "HTTP score workspace", Score: rangeValue,
		MaxExecution: 4 * time.Hour, ReviewDeadline: time.Hour, BusinessTimezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = f.services.Catalog.ActivateWorkspace(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.services.Catalog.CreateDataZone(ctx, domain.DataZone{
		Code: "HTTP-SOURCE", Name: "HTTP source", Timezone: "UTC", DailyLimit: 5, CutoffHour: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := f.services.Catalog.CreateDataZone(ctx, domain.DataZone{
		Code: "HTTP-TARGET", Name: "HTTP target", Timezone: "UTC", DailyLimit: 5, CutoffHour: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	pool, err := f.services.Catalog.CreateComputePool(ctx, domain.ComputePool{
		SerialNumber: "HTTP-SCORE-POOL", CapacityRows: 1000,
		AttestationDueAt: now.Add(48 * time.Hour), LastReconciledAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.services.Catalog.RegisterSnapshot(ctx, domain.DatasetSnapshot{
		WorkspaceID: workspace.ID, SourceZoneID: source.ID, SourceRevision: "HTTP-SCORE-REV",
		SchemaFamily: "policy-score", PartitionCount: 1, EstimatedRows: 100,
		ExpiresAt: now.Add(48 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = f.services.Catalog.ValidateSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.services.Inference.PlanInferenceRun(ctx, service.PlanInferenceRunInput{
		WorkspaceID: workspace.ID, SourceZoneID: source.ID, TargetZoneID: target.ID,
		ComputePoolID: pool.ID, Reference: "HTTP-SCORE-RUN", SnapshotIDs: []string{snapshot.ID},
		ScheduledStartAt: now.Add(time.Hour), ExpectedFinishAt: now.Add(2 * time.Hour),
		IdempotencyKey: "http-score-run-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.services.Inference.StageInferenceRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	run, err = f.services.Inference.StartInferenceRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return run
}
