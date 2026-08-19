package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestQualityRangeBoundaries(t *testing.T) {
	min := MilliScore(2000)
	max := MilliScore(8000)
	rangeValue, err := NewQualityRange(min, max)
	if err != nil {
		t.Fatalf("create range: %v", err)
	}
	tests := []struct {
		name  string
		value MilliScore
		want  bool
	}{
		{name: "minimum included", value: min, want: true},
		{name: "middle included", value: 5000, want: true},
		{name: "maximum included", value: max, want: true},
		{name: "below minimum", value: 1999, want: false},
		{name: "above maximum", value: 8001, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := rangeValue.Contains(test.value); got != test.want {
				t.Fatalf("Contains(%d) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestScoreParsingRejectsInvalidValues(t *testing.T) {
	for _, value := range []float64{-197, 101, math.NaN()} {
		_, err := ScoreFromFloat(value)
		if err == nil {
			t.Fatalf("ScoreFromFloat(%v) succeeded", value)
		}
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("error %v does not wrap validation", err)
		}
	}
}

func TestSnapshotTransitionTable(t *testing.T) {
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	base := DatasetSnapshot{State: SnapshotRegistered, ExpiresAt: now.Add(24 * time.Hour)}
	cases := []struct {
		name string
		from SnapshotState
		to   SnapshotState
		want bool
	}{
		{"registered to validated", SnapshotRegistered, SnapshotValidated, true},
		{"validated to reserved", SnapshotValidated, SnapshotReserved, true},
		{"reserved to materializing", SnapshotReserved, SnapshotMaterializing, true},
		{"materializing to materialized", SnapshotMaterializing, SnapshotMaterialized, true},
		{"materialized to approved", SnapshotMaterialized, SnapshotApproved, true},
		{"materialized to quarantine", SnapshotMaterialized, SnapshotQuarantined, true},
		{"quarantine to rejected", SnapshotQuarantined, SnapshotRejected, true},
		{"registered to approved", SnapshotRegistered, SnapshotApproved, false},
		{"approved to validated", SnapshotApproved, SnapshotValidated, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			batch := base
			batch.State = test.from
			err := batch.Transition(test.to, now)
			if (err == nil) != test.want {
				t.Fatalf("transition %s -> %s error = %v, want allowed=%v", test.from, test.to, err, test.want)
			}
			if test.want && batch.State != test.to {
				t.Fatalf("state = %s, want %s", batch.State, test.to)
			}
		})
	}
}

func TestExpiredSnapshotCanOnlyBeDestroyedOrQuarantined(t *testing.T) {
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	batch := DatasetSnapshot{State: SnapshotMaterialized, ExpiresAt: now.Add(-time.Minute)}
	if err := batch.Transition(SnapshotApproved, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("expired release error = %v, want conflict", err)
	}
	batch.State = SnapshotMaterialized
	if err := batch.Transition(SnapshotQuarantined, now); err != nil {
		t.Fatalf("expired quarantine failed: %v", err)
	}
}

func TestInferenceRunTransitionSetsTimestamps(t *testing.T) {
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	run := InferenceRun{State: InferenceRunQueued, ScheduledStartAt: now, ExpectedFinishAt: now.Add(2 * time.Hour)}
	if err := run.Transition(InferenceRunStaged, now); err != nil {
		t.Fatalf("pack: %v", err)
	}
	if err := run.Transition(InferenceRunRunning, now.Add(time.Minute)); err != nil {
		t.Fatalf("start: %v", err)
	}
	if run.StartedAt == nil || !run.StartedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("started_at = %v", run.StartedAt)
	}
	if err := run.Transition(InferenceRunCompleted, now.Add(time.Hour)); err != nil {
		t.Fatalf("arrive: %v", err)
	}
	if err := run.Transition(InferenceRunArchived, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("close: %v", err)
	}
	if run.ArchivedAt == nil {
		t.Fatal("archived_at is nil")
	}
}

func TestInferenceRunRejectsSkippedState(t *testing.T) {
	run := InferenceRun{State: InferenceRunQueued}
	err := run.Transition(InferenceRunCompleted, time.Now())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v, want invalid transition", err)
	}
}

func TestApprovalTaskResolutionAndExpiry(t *testing.T) {
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	approval_task := ApprovalTask{Status: ApprovalTaskPending, ExpiresAt: now.Add(time.Hour)}
	if err := approval_task.Resolve(ApprovalTaskAccepted, "seal intact", now); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if approval_task.Status != ApprovalTaskAccepted || approval_task.ResolvedAt == nil {
		t.Fatalf("approval_task after accept = %+v", approval_task)
	}
	approval_task = ApprovalTask{Status: ApprovalTaskPending, ExpiresAt: now.Add(-time.Minute)}
	if err := approval_task.Resolve(ApprovalTaskAccepted, "", now); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired accept error = %v", err)
	}
	if err := approval_task.Resolve(ApprovalTaskExpired, "expired", now); err != nil {
		t.Fatalf("expire: %v", err)
	}
}

func TestDriftIncidentAggregatesObservations(t *testing.T) {
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	drift_incident := DriftIncident{Status: DriftIncidentOpen}
	observations := []QualityObservation{
		{Score: 9200, RecordedAt: now.Add(10 * time.Minute)},
		{Score: 8500, RecordedAt: now.Add(5 * time.Minute)},
		{Score: 11000, RecordedAt: now.Add(20 * time.Minute)},
	}
	for _, observation := range observations {
		drift_incident.Include(observation, now)
	}
	if drift_incident.ObservationCount != 3 || drift_incident.Minimum != 8500 || drift_incident.Maximum != 11000 {
		t.Fatalf("aggregate = %+v", drift_incident)
	}
	if !drift_incident.FirstObservationAt.Equal(now.Add(5*time.Minute)) || !drift_incident.LastObservationAt.Equal(now.Add(20*time.Minute)) {
		t.Fatalf("observation window = %v..%v", drift_incident.FirstObservationAt, drift_incident.LastObservationAt)
	}
}

func TestDriftIncidentDecisionTable(t *testing.T) {
	now := time.Now().UTC()
	for _, decision := range []DriftIncidentStatus{DriftIncidentCleared, DriftIncidentRejected} {
		drift_incident := DriftIncident{Status: DriftIncidentReviewing}
		if err := drift_incident.Decide(decision, now); err != nil {
			t.Fatalf("decision %s: %v", decision, err)
		}
		if drift_incident.Status != decision {
			t.Fatalf("status = %s, want %s", drift_incident.Status, decision)
		}
	}
	drift_incident := DriftIncident{Status: DriftIncidentCleared}
	if err := drift_incident.Decide(DriftIncidentRejected, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("decide closed drift_incident = %v", err)
	}
}

func TestDataZoneBusinessDayUsesCutoffAndTimezone(t *testing.T) {
	data_zone := DataZone{Timezone: "Asia/Shanghai", CutoffHour: 6}
	before, err := data_zone.BusinessDay(time.Date(2026, 8, 18, 21, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if before != "2026-08-18" {
		t.Fatalf("business day = %s", before)
	}
	after, err := data_zone.BusinessDay(time.Date(2026, 8, 18, 22, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if after != "2026-08-19" {
		t.Fatalf("business day after cutoff = %s", after)
	}
}

func TestComputePoolEligibility(t *testing.T) {
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	base := ComputePool{State: ComputePoolAvailable, CapacityRows: 1000, AttestationDueAt: now.Add(time.Hour)}
	if err := base.EligibleFor(now, 1000); err != nil {
		t.Fatalf("capacity boundary: %v", err)
	}
	if err := base.EligibleFor(now, 1001); !errors.Is(err, ErrCapacityExceeded) {
		t.Fatalf("capacity overflow = %v", err)
	}
	base.State = ComputePoolReserved
	if err := base.EligibleFor(now, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("reserved compute_pool = %v", err)
	}
}

func TestReadinessEvaluate(t *testing.T) {
	report := RunReadiness{InferenceRunState: InferenceRunCompleted, ExpectedSnapshotCount: 2, MaterializedSnapshotCount: 2, PendingApprovalTask: true}
	report.Evaluate()
	if report.Complete || len(report.Blockers) != 1 || report.Blockers[0] != "pending approval task" {
		t.Fatalf("report = %+v", report)
	}
	report.PendingApprovalTask = false
	report.Evaluate()
	if !report.Complete {
		t.Fatalf("resolved report = %+v", report)
	}
}

func TestAuditAndJobCloneIsolation(t *testing.T) {
	event := AuditEvent{Metadata: map[string]string{"one": "1"}}
	clone := event.Clone()
	clone.Metadata["one"] = "2"
	if event.Metadata["one"] != "1" {
		t.Fatal("audit metadata was shared")
	}
	job := OutboxJob{Payload: []byte("payload")}
	jobClone := job.Clone()
	jobClone.Payload[0] = 'P'
	if string(job.Payload) != "payload" {
		t.Fatal("job payload was shared")
	}
}

func TestExecutionWindowChecksWorkspaceLimitAndExpiry(t *testing.T) {
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	workspace := Workspace{MaxExecution: 2 * time.Hour}
	batch := DatasetSnapshot{ExpiresAt: now.Add(4 * time.Hour)}
	valid := ExecutionWindow{StartAt: now.Add(time.Hour), FinishAt: now.Add(2 * time.Hour)}
	if err := valid.Validate(workspace, []DatasetSnapshot{batch}, now); err != nil {
		t.Fatalf("valid window: %v", err)
	}
	tooLong := ExecutionWindow{StartAt: now.Add(time.Hour), FinishAt: now.Add(4 * time.Hour)}
	if err := tooLong.Validate(workspace, []DatasetSnapshot{batch}, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("long window = %v", err)
	}
	batch.ExpiresAt = now.Add(90 * time.Minute)
	if err := valid.Validate(workspace, []DatasetSnapshot{batch}, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("expired batch window = %v", err)
	}
}

func TestPrincipalActionMatrix(t *testing.T) {
	cases := []struct {
		role   Role
		action Action
		want   bool
	}{
		{RoleMLEngineer, ActionPlanInference, true},
		{RoleMLEngineer, ActionReviewDrift, false},
		{RoleDataEngineer, ActionRecordMetrics, true},
		{RoleDataEngineer, ActionCatalogWrite, false},
		{RoleRiskReviewer, ActionReviewDrift, true},
		{RoleComplianceAuditor, ActionReadAudit, true},
		{RoleComplianceAuditor, ActionRunInference, false},
	}
	for _, test := range cases {
		principal := Principal{Role: test.role}
		if got := principal.CanAction(test.action); got != test.want {
			t.Fatalf("%s %s = %v, want %v", test.role, test.action, got, test.want)
		}
	}
}

func TestIdentifierNormalizationAndValidation(t *testing.T) {
	if got := NormalizeCode("  data-zone-sh-01 "); got != "DATA-ZONE-SH-01" {
		t.Fatalf("normalized code = %q", got)
	}
	for _, value := range []string{"A", "with spaces", "ümlaut", "", strings.Repeat("X", 65)} {
		if err := ValidateBusinessCode("code", value); err == nil {
			t.Fatalf("invalid code %q passed", value)
		}
	}
	for _, value := range []string{"valid-key", "request-1234", strings.Repeat("x", 128)} {
		if err := ValidateIdempotencyKey(value); err != nil {
			t.Fatalf("valid idempotency key %q: %v", value, err)
		}
	}
	for _, value := range []string{"short", "line\nbreak", strings.Repeat("x", 129)} {
		if err := ValidateIdempotencyKey(value); err == nil {
			t.Fatalf("invalid idempotency key %q passed", value)
		}
	}
}

func TestTerminalStateHelpers(t *testing.T) {
	if !InferenceRunArchived.IsTerminal() || !InferenceRunCancelled.IsTerminal() || InferenceRunCompleted.IsTerminal() {
		t.Fatal("run terminal states are incorrect")
	}
	if !SnapshotApproved.IsTerminal() || !SnapshotRejected.IsTerminal() || SnapshotQuarantined.IsTerminal() {
		t.Fatal("snapshot terminal states are incorrect")
	}
	if !DriftIncidentCleared.IsResolved() || !DriftIncidentRejected.IsResolved() || DriftIncidentOpen.IsResolved() {
		t.Fatal("drift_incident resolved states are incorrect")
	}
}
