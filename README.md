# Agentic AI Governance Mesh

Agentic AI Governance Mesh is a pure Go backend for operating governed feature datasets and safety-controlled agentic inference workloads. It coordinates dataset snapshot validation, GPU compute-pool allocation, inference execution, quality drift review, risk approval, audit, idempotency, and durable background delivery without relying on an external online service.

## Architecture

- `cmd/server` wires configuration, SQLite, services, HTTP middleware, workers, and graceful shutdown.
- `cmd/seed-user` creates or updates a local login account with a bcrypt password hash.
- `internal/domain` owns state machines, value objects, validation, authorization roles, and sentinel errors.
- `internal/service` orchestrates transactional catalog, inference, quality, approval, review, query, and bulk workflows.
- `internal/repository` defines persistence contracts without exposing SQL to the HTTP or domain layers.
- `internal/storage/sqlite` implements migrations, optimistic updates, transactional writes, pagination, recovery, and job claiming with real SQL.
- `internal/httpapi` provides authenticated JSON APIs and stable error contracts.
- `internal/middleware` supplies request IDs, structured request logging, and panic recovery.
- `internal/worker` claims durable outbox jobs, retries transient failures with bounded backoff, expires approval tasks, and honors cancellation.
- `internal/audit` records actor, request, action, entity, result, and metadata inside the owning transaction.

## Data Model

The SQLite schema is created by the versioned migrations in `internal/storage/sqlite/migrations`. The root `migrations` directory mirrors the deployable SQL.

- `users` own login credentials, roles, lifecycle state, and optimistic versions; `sessions` reference users and support expiry and revocation.
- `workspaces` define quality thresholds, execution limits, review deadlines, and business time zones.
- `data_zones` define source and target execution boundaries, daily capacity, and local business-day cutoff rules.
- `dataset_snapshots` belong to a workspace and source zone; revisions are unique within a workspace.
- `compute_pools` represent attested GPU capacity and carry an exclusive run reservation.
- `inference_runs` connect a workspace, two zones, and a compute pool. `inference_run_inputs` connects each snapshot to at most one run.
- `approval_tasks`, `quality_observations`, `drift_incidents`, and `review_decisions` capture execution governance and evidence.
- `audit_events`, `idempotency_records`, and `outbox_jobs` provide traceability, replay protection, and durable asynchronous processing.

Planning an inference run is a cross-entity transaction: it validates the workspace and zones, reserves every dataset snapshot, checks total row capacity, reserves the compute pool, writes the run and input links, stores the idempotent response, enqueues an outbox job, and records audit evidence. Any intermediate error rolls the transaction back. Optimistic versions and database constraints prevent concurrent reservation and update loss. Reopening the SQLite file restores all durable state and pending work.

## State Machines

- Workspace: `draft -> active -> archived`.
- Dataset snapshot: `registered -> validated -> reserved -> materializing -> materialized`, with quarantine and approved/rejected terminal review outcomes.
- Inference run: `queued -> staged -> running -> completed -> archived`, with cancellation allowed before execution.
- Compute pool: `available -> reserved -> allocated`, plus reconciliation and retirement paths.
- Drift incident: `open -> reviewing -> cleared|rejected`.
- Approval task: `pending -> accepted|rejected|expired`.

## Configuration

Copy values from `.env.example` into the process environment. Supported variables are `HTTP_ADDR`, `DATABASE_PATH`, `BUSINESS_TIMEZONE`, `APPROVAL_TTL`, `SESSION_TTL`, `WORKER_INTERVAL`, `WORKER_BATCH_SIZE`, `SHUTDOWN_TIMEOUT`, and `LOG_LEVEL`.

No password or token is committed. Create a local account explicitly:

```bash
DATABASE_PATH=./data/featuremesh.db \
BOOTSTRAP_EMAIL=ml@example.test \
BOOTSTRAP_PASSWORD='change-this-local-password' \
BOOTSTRAP_DISPLAY_NAME='ML Engineer' \
BOOTSTRAP_ROLE=ml_engineer \
go run ./cmd/seed-user
```

Start the API with `go run ./cmd/server`. The process applies unapplied migrations, checks the database before reporting readiness, runs background workers, and shuts down HTTP and workers when the process context is cancelled.

## HTTP API

Health endpoints are public: `GET /healthz` and `GET /readyz`.

Authentication endpoints are `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, and `POST /api/v1/users`.

Authenticated operations include workspace activation, data-zone and compute-pool management, dataset snapshot validation, inference planning and lifecycle transitions, approval tasks, quality observations, drift review, audit search, and platform summary. Run planning requires an `Idempotency-Key` header and `workspace_id`, `source_zone_id`, `target_zone_id`, `compute_pool_id`, `reference`, `snapshot_ids`, `scheduled_start_at`, and `expected_finish_at`.

Successful login example:

```bash
curl -sS http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"ml@example.test","password":"change-this-local-password"}'
```

Use the returned value as `Authorization: Bearer <token>`. Errors use one JSON shape with a stable code, readable message, and request ID:

```json
{"error":{"code":"business_conflict","message":"...","request_id":"req_..."}}
```

## Build And Test

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
```

Repository integration tests use temporary on-disk SQLite databases and cover migrations, cross-entity rollback, optimistic conflicts, restart recovery, pagination, HTTP contracts, authentication, state transitions, idempotency, context cancellation, worker retry, and concurrent business outcomes.

Build and run the Linux image:

```bash
docker build --platform linux/amd64 -t ai-featuremesh .
docker run --rm -p 8080:8080 -v featuremesh-data:/data \
  -e DATABASE_PATH=/data/featuremesh.db ai-featuremesh
```

The domain, transaction, repository, HTTP, authentication, audit, and worker boundaries are deliberately separated so future maintenance exercises can target realistic operational behavior while the baseline remains fully working.
