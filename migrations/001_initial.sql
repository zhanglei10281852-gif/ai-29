PRAGMA foreign_keys = ON;

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE COLLATE NOCASE,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    status TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_sessions_user_expiry ON sessions(user_id, expires_at);

CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    minimum_score_millis INTEGER NOT NULL,
    maximum_score_millis INTEGER NOT NULL,
    max_execution_seconds INTEGER NOT NULL,
    review_deadline_seconds INTEGER NOT NULL,
    business_timezone TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE data_zones (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    timezone TEXT NOT NULL,
    status TEXT NOT NULL,
    daily_limit INTEGER NOT NULL,
    cutoff_hour INTEGER NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE dataset_snapshots (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    source_zone_id TEXT NOT NULL REFERENCES data_zones(id),
    source_revision TEXT NOT NULL,
    schema_family TEXT NOT NULL,
    partition_count INTEGER NOT NULL,
    estimated_rows INTEGER NOT NULL,
    state TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    run_id TEXT,
    quarantine_note TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(workspace_id, source_revision)
);
CREATE INDEX idx_dataset_snapshots_state ON dataset_snapshots(state, expires_at);
CREATE INDEX idx_dataset_snapshots_data_zone ON dataset_snapshots(source_zone_id, created_at);

CREATE TABLE compute_pools (
    id TEXT PRIMARY KEY,
    serial_number TEXT NOT NULL UNIQUE,
    state TEXT NOT NULL,
    capacity_rows INTEGER NOT NULL,
    attestation_due_at TEXT NOT NULL,
    last_reconciled_at TEXT NOT NULL,
    reserved_run_id TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_compute_pools_state_attestation ON compute_pools(state, attestation_due_at);

CREATE TABLE inference_runs (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    source_zone_id TEXT NOT NULL REFERENCES data_zones(id),
    target_zone_id TEXT NOT NULL REFERENCES data_zones(id),
    compute_pool_id TEXT NOT NULL REFERENCES compute_pools(id),
    reference TEXT NOT NULL UNIQUE,
    state TEXT NOT NULL,
    scheduled_start_at TEXT NOT NULL,
    expected_finish_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    archived_at TEXT,
    total_estimated_rows INTEGER NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_inference_runs_route_window ON inference_runs(source_zone_id, target_zone_id, scheduled_start_at);
CREATE INDEX idx_inference_runs_state ON inference_runs(state, expected_finish_at);

CREATE TABLE inference_run_inputs (
    run_id TEXT NOT NULL REFERENCES inference_runs(id) ON DELETE CASCADE,
    snapshot_id TEXT NOT NULL UNIQUE REFERENCES dataset_snapshots(id),
    added_at TEXT NOT NULL,
    PRIMARY KEY(run_id, snapshot_id)
);

CREATE TABLE approval_tasks (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES inference_runs(id),
    requester_id TEXT NOT NULL,
    reviewer_id TEXT NOT NULL,
    review_queue TEXT NOT NULL,
    status TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    resolved_at TEXT,
    resolution_note TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_approval_task_pending_run ON approval_tasks(run_id) WHERE status = 'pending';
CREATE INDEX idx_approval_task_expiry ON approval_tasks(status, expires_at);

CREATE TABLE quality_observations (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES inference_runs(id),
    metric_key TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    score_millis INTEGER NOT NULL,
    recorded_at TEXT NOT NULL,
    received_at TEXT NOT NULL,
    UNIQUE(run_id, metric_key, sequence)
);
CREATE INDEX idx_observations_run_time ON quality_observations(run_id, recorded_at);

CREATE TABLE drift_incidents (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES inference_runs(id),
    status TEXT NOT NULL,
    first_observation_at TEXT NOT NULL,
    last_observation_at TEXT NOT NULL,
    minimum_score_millis INTEGER NOT NULL,
    maximum_score_millis INTEGER NOT NULL,
    observation_count INTEGER NOT NULL,
    review_due_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_drift_incident_active_run ON drift_incidents(run_id) WHERE status IN ('open', 'reviewing');
CREATE INDEX idx_drift_incident_review_due ON drift_incidents(status, review_due_at);

CREATE TABLE review_decisions (
    id TEXT PRIMARY KEY,
    drift_incident_id TEXT NOT NULL REFERENCES drift_incidents(id),
    risk_reviewer TEXT NOT NULL,
    decision TEXT NOT NULL,
    rationale TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    outcome TEXT NOT NULL,
    metadata_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_audit_entity ON audit_events(entity_type, entity_id, created_at);
CREATE INDEX idx_audit_request ON audit_events(request_id);

CREATE TABLE idempotency_records (
    scope TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_code INTEGER NOT NULL,
    response_body BLOB NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY(scope, idempotency_key)
);

CREATE TABLE outbox_jobs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload BLOB NOT NULL,
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL,
    available_at TEXT NOT NULL,
    locked_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_outbox_claim ON outbox_jobs(status, available_at, created_at);

CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);
