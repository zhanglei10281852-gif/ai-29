CREATE INDEX idx_dataset_snapshots_run_state ON dataset_snapshots(run_id, state);
CREATE INDEX idx_inference_runs_workspace_created ON inference_runs(workspace_id, created_at);
CREATE INDEX idx_approval_tasks_custodians ON approval_tasks(reviewer_id, status, created_at);
CREATE INDEX idx_jobs_aggregate ON outbox_jobs(kind, aggregate_id, created_at);
