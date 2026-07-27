# sub2api Radar P1 Control Plane and Lease Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the database-backed radar control plane that versions datasets, creates paired evaluation runs, enforces budget and concurrency, and safely leases idempotent assignments to controlled Workers.

**Architecture:** PostgreSQL is the source of truth for datasets, plans, runs, samples, and assignment leases; Redis is notification-only. Admin APIs create immutable dataset versions and plans. The orchestrator expands a baseline/candidate matrix into samples and assignments in one transaction, while Workers claim work through `FOR UPDATE SKIP LOCKED`, renew fenced leases, and submit evidence through idempotent endpoints.

**Tech Stack:** Go 1.26.5, Gin, Ent for simple entities, `database/sql` for lease transactions, PostgreSQL, Redis, existing S3-compatible client, Wire, Testify, testcontainers.

## Global Constraints

- Requires completion of `2026-07-25-sub2api-radar-p0-route-evidence.md`.
- PostgreSQL is authoritative; losing Redis notifications must not lose work.
- P1 datasets contain only public or platform-maintained synthetic cases.
- Dataset versions, case payload hashes, execution image digests, model parameters, and grader versions are immutable after publication.
- Assignment idempotency key is `run_id + case_id + model_route + sample_index + attempt`.
- Lease completion requires the current random lease token; an expired Worker cannot overwrite a newer attempt.
- At 80% budget, emit a budget warning; at 100%, lease only P0 sentinel assignments and pause all lower priorities.
- Worker credentials cannot access Admin APIs, production databases, or upstream account credentials.

---

## File Structure

- `backend/migrations/191_add_radar_control_plane.sql`: dataset, plan, run, sample, assignment, Worker, and artifact tables.
- `backend/ent/schema/evaluation_dataset_version.go`, `evaluation_case.go`, `evaluation_plan.go`, `evaluation_run.go`: configuration entities.
- `backend/internal/service/evaluation.go`: shared enums and domain types.
- `backend/internal/service/evaluation_repository.go`: repository boundary.
- `backend/internal/repository/evaluation_repo.go`: CRUD, matrix expansion, leases, and fenced transitions.
- `backend/internal/repository/evaluation_queue.go`: Redis notification adapter.
- `backend/internal/service/evaluation_dataset_service.go`: immutable dataset publication.
- `backend/internal/service/evaluation_orchestrator.go`: run creation, budget checks, and scheduling.
- `backend/internal/service/evaluation_retention_service.go`: raw-artifact and aggregate retention enforcement.
- `backend/internal/service/evaluation_lease_service.go`: claim, heartbeat, evidence, and completion state machine.
- `backend/internal/service/evaluation_artifact.go`: presigned artifact policy.
- `backend/internal/repository/evaluation_artifact_s3.go`: S3-compatible presigned PUT/GET implementation.
- `backend/internal/server/middleware/radar_worker_auth.go`: Worker bearer-token authentication.
- `backend/internal/handler/internal/radar_worker_handler.go`: controlled Worker endpoints.
- `backend/internal/handler/admin/evaluation_handler.go`: dataset, plan, and run Admin APIs.
- `backend/internal/server/routes/radar_worker.go`, `backend/internal/server/routes/admin.go`: route registration.
- Existing Wire provider files and `backend/cmd/server/wire_gen.go`: DI.

### Task 1: Create the Control-Plane Schema and State Vocabulary

**Files:**
- Create: `backend/migrations/191_add_radar_control_plane.sql`
- Create: `backend/ent/schema/evaluation_dataset_version.go`
- Create: `backend/ent/schema/evaluation_case.go`
- Create: `backend/ent/schema/evaluation_plan.go`
- Create: `backend/ent/schema/evaluation_run.go`
- Create: `backend/internal/service/evaluation.go`
- Test: `backend/internal/repository/migrations_schema_integration_test.go`

**Interfaces:**
- Produces: typed `DatasetStatus`, `RunStatus`, `AssignmentStatus`, `FailureClass`, `CapabilityDomain`, and `CasePriority` constants.
- Produces: immutable configuration tables and mutable execution tables.

- [ ] **Step 1: Write failing migration and enum tests**

Assert all ten tables exist and that database constraints reject `completed -> running`, a negative budget, duplicate sample identity, and an unknown failure class.

```go
requireTable(t, tx, "evaluation_dataset_versions")
requireTable(t, tx, "evaluation_cases")
requireTable(t, tx, "evaluation_plans")
requireTable(t, tx, "evaluation_runs")
requireTable(t, tx, "evaluation_samples")
requireTable(t, tx, "evaluation_assignments")
requireTable(t, tx, "evaluation_workers")
requireTable(t, tx, "evaluation_artifacts")
requireTable(t, tx, "evaluation_run_events")
requireTable(t, tx, "evaluation_budget_ledger")
```

- [ ] **Step 2: Run schema tests and verify red**

Run: `cd backend && go test -tags=integration ./internal/repository -run 'MigrationsSchema|RadarControlPlaneConstraints'`

Expected: FAIL because migration 191 has not been applied.

- [ ] **Step 3: Implement exact state and identity constraints**

Use UUID primary keys generated in Go. The migration must include these identity and transition-critical fields:

```sql
CREATE TABLE IF NOT EXISTS evaluation_dataset_versions (
  id UUID PRIMARY KEY, dataset_key VARCHAR(100) NOT NULL, version VARCHAR(100) NOT NULL,
  manifest_sha256 CHAR(64) NOT NULL, source_type VARCHAR(20) NOT NULL,
  status VARCHAR(20) NOT NULL CHECK (status IN ('draft','published','retired')),
  created_by BIGINT NOT NULL REFERENCES users(id), published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), UNIQUE(dataset_key, version)
);
CREATE TABLE IF NOT EXISTS evaluation_cases (
  id UUID PRIMARY KEY, dataset_version_id UUID NOT NULL REFERENCES evaluation_dataset_versions(id),
  case_key VARCHAR(160) NOT NULL, capability_domain VARCHAR(32) NOT NULL,
  priority VARCHAR(4) NOT NULL CHECK (priority IN ('P0','P1','P2')),
  weight NUMERIC(10,4) NOT NULL CHECK (weight > 0), sample_count INT NOT NULL CHECK (sample_count BETWEEN 1 AND 10),
  prompt_spec JSONB, expected_spec JSONB, encrypted_spec TEXT, execution_spec JSONB NOT NULL,
  grader_id VARCHAR(100) NOT NULL, grader_version VARCHAR(100) NOT NULL,
  content_sha256 CHAR(64) NOT NULL, confidentiality VARCHAR(20) NOT NULL,
  UNIQUE(dataset_version_id, case_key),
  CHECK (
    (confidentiality IN ('public','synthetic') AND prompt_spec IS NOT NULL AND expected_spec IS NOT NULL AND encrypted_spec IS NULL)
    OR (confidentiality IN ('restricted','safety') AND prompt_spec IS NULL AND expected_spec IS NULL AND encrypted_spec IS NOT NULL)
  )
);
CREATE TABLE IF NOT EXISTS evaluation_plans (
  id UUID PRIMARY KEY, name VARCHAR(120) NOT NULL, dataset_version_id UUID NOT NULL REFERENCES evaluation_dataset_versions(id),
  trigger_type VARCHAR(20) NOT NULL, cron_expression VARCHAR(100), model_matrix JSONB NOT NULL,
  max_run_cost NUMERIC(20,8) NOT NULL CHECK (max_run_cost > 0), daily_cost_limit NUMERIC(20,8) NOT NULL CHECK (daily_cost_limit > 0),
  max_concurrency INT NOT NULL CHECK (max_concurrency BETWEEN 1 AND 1000), enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_by BIGINT NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS evaluation_runs (
  id UUID PRIMARY KEY, plan_id UUID NOT NULL REFERENCES evaluation_plans(id), trigger_source VARCHAR(20) NOT NULL,
  baseline_ref JSONB NOT NULL, candidate_ref JSONB NOT NULL, status VARCHAR(24) NOT NULL,
  budget_limit NUMERIC(20,8) NOT NULL, reserved_cost NUMERIC(20,8) NOT NULL DEFAULT 0,
  actual_cost NUMERIC(20,8) NOT NULL DEFAULT 0, calibration_mode BOOLEAN NOT NULL DEFAULT TRUE,
  created_by BIGINT REFERENCES users(id), started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

`evaluation_samples` has the unique key `(run_id, case_id, model_route, sample_index)`. `evaluation_assignments` has `(sample_id, attempt)` unique, a random `lease_token_hash`, `leased_by`, `lease_expires_at`, `heartbeat_at`, `evidence_manifest`, and statuses matching the specification. The Worker table stores only a SHA-256 token hash. The artifact table stores object key, SHA-256, byte count, MIME type, scan status, and retention deadline.

- [ ] **Step 4: Generate Ent and verify green**

Run: `cd backend && go generate ./ent`

Run: `cd backend && go test -tags=integration ./internal/repository -run 'MigrationsSchema|RadarControlPlaneConstraints'`

Expected: PASS.

- [ ] **Step 5: Commit schema and domain vocabulary**

```bash
git add backend/migrations/191_add_radar_control_plane.sql backend/ent backend/internal/service/evaluation.go backend/internal/repository/migrations_schema_integration_test.go
git commit -m "feat(radar): add control plane schema"
```

### Task 2: Implement Atomic Matrix Expansion and Fenced Leases

**Files:**
- Create: `backend/internal/service/evaluation_repository.go`
- Create: `backend/internal/repository/evaluation_repo.go`
- Test: `backend/internal/repository/evaluation_repo_integration_test.go`

**Interfaces:**
- Produces: `CreateRunWithMatrix(ctx context.Context, input CreateRunInput) (*EvaluationRun, error)`.
- Produces: `ClaimAssignment(ctx context.Context, workerID uuid.UUID, capabilities []string, leaseTTL time.Duration) (*AssignmentLease, error)`.
- Produces: `RenewLease(ctx context.Context, assignmentID uuid.UUID, leaseToken string, extendBy time.Duration) (time.Time, error)`.
- Produces: `TransitionAssignment(ctx context.Context, input AssignmentTransition) error`.

- [ ] **Step 1: Add concurrent lease and expansion tests**

Create two goroutines claiming from the same pool and assert different assignment IDs. Expire the first lease, reclaim it as attempt 2, then prove attempt 1 cannot heartbeat or complete it. Expand two cases, two routes, baseline/candidate sides, and three samples; assert `2*2*2*3 = 24` samples and one initial assignment per sample.

```go
require.ErrorIs(t,
    repo.TransitionAssignment(ctx, AssignmentTransition{AssignmentID: old.ID, LeaseToken: old.Token, To: AssignmentCompleted}),
    service.ErrLeaseFenced,
)
```

- [ ] **Step 2: Run lease tests and verify red**

Run: `cd backend && go test -tags=integration ./internal/repository -run 'EvaluationRepository|ConcurrentLease|LeaseFencing'`

Expected: FAIL with undefined repository methods.

- [ ] **Step 3: Implement matrix expansion transaction**

Lock the plan row, verify its dataset is `published`, insert the run, samples, assignments, budget reservations, and `run_created` event in one transaction. Calculate idempotency as SHA-256 of the canonical string:

```go
fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d", runID, caseID, modelRoute, sampleIndex, attempt)
```

- [ ] **Step 4: Implement claim and fencing SQL**

Claim with one transaction using `FOR UPDATE SKIP LOCKED`, first reclaiming expired `leased/running` rows into a new attempt. Store `sha256(leaseToken)` and return the plaintext exactly once. Every mutation uses `WHERE id=$1 AND lease_token_hash=$2 AND lease_expires_at>NOW()`.

- [ ] **Step 5: Run integration tests and commit**

Run: `cd backend && go test -race -tags=integration ./internal/repository -run 'EvaluationRepository|ConcurrentLease|LeaseFencing'`

Expected: PASS.

```bash
git add backend/internal/service/evaluation_repository.go backend/internal/repository/evaluation_repo*
git commit -m "feat(radar): add atomic runs and fenced leases"
```

### Task 3: Publish Immutable Dataset Versions

**Files:**
- Create: `backend/internal/service/evaluation_dataset_service.go`
- Test: `backend/internal/service/evaluation_dataset_service_test.go`
- Create: `backend/internal/handler/admin/evaluation_dataset_handler.go`
- Test: `backend/internal/handler/admin/evaluation_dataset_handler_test.go`

**Interfaces:**
- Produces: `CreateDraft(ctx, actorID int64, input DatasetDraftInput) (*DatasetVersion, error)`.
- Produces: `Publish(ctx, actorID int64, id uuid.UUID, expectedManifestSHA256 string) (*DatasetVersion, error)`.
- Produces: Admin endpoints `POST /api/v1/admin/radar/datasets`, `POST /:id/cases:batch`, and `POST /:id/publish`.
- Consumes: the existing `SecretEncryptor` for `restricted` and `safety` case payloads.

- [ ] **Step 1: Write dataset immutability tests**

Test canonical JSON hashing, duplicate case keys, unsupported capability domains, P0 sample count outside 1-10, publish hash mismatch, edits after publication, and a second publish call returning the same published object.

- [ ] **Step 2: Run tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run EvaluationDataset`

Expected: FAIL because the service and handlers do not exist.

- [ ] **Step 3: Implement strict case validation**

Accept domains `coding`, `reasoning`, `instruction`, `long_context`, `tool_call`, `protocol`, `safety`, `performance`, and `cost`. Compute each `content_sha256` from RFC 8785-style canonicalized JSON fields plus grader ID/version. Store public/synthetic prompt and expected JSON directly; encrypt restricted/safety prompt and expected JSON together with the existing `SecretEncryptor`, leaving plaintext columns null. Compute the manifest hash from sorted `(case_key, content_sha256)` pairs. A published version can only transition to `retired`.

- [ ] **Step 4: Implement admin DTOs without secret fields**

Return case metadata and hashes in list APIs. Return `prompt_spec` and `expected_spec` only from the single-case endpoint and rely on the existing Admin audit middleware to record the sensitive read.

- [ ] **Step 5: Run tests and commit**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run EvaluationDataset`

Expected: PASS.

```bash
git add backend/internal/service/evaluation_dataset_service* backend/internal/handler/admin/evaluation_dataset_handler*
git commit -m "feat(radar): publish immutable evaluation datasets"
```

### Task 4: Enforce Run Budgets, Priority, and Scheduling

**Files:**
- Create: `backend/internal/repository/evaluation_queue.go`
- Test: `backend/internal/repository/evaluation_queue_test.go`
- Create: `backend/internal/service/evaluation_orchestrator.go`
- Test: `backend/internal/service/evaluation_orchestrator_test.go`
- Create: `backend/internal/service/evaluation_retention_service.go`
- Test: `backend/internal/service/evaluation_retention_service_test.go`
- Modify: `backend/internal/service/wire.go`

**Interfaces:**
- Produces: `EvaluationQueue.Notify(ctx context.Context, runID uuid.UUID) error` and `Wait(ctx context.Context, max time.Duration) error`.
- Produces: `StartRun(ctx context.Context, actorID int64, input StartRunInput) (*EvaluationRun, error)`.
- Produces: `Tick(ctx context.Context, now time.Time) error` for cron plans and stale leases.
- Produces: `EvaluationRetentionService.Cleanup(ctx context.Context, now time.Time, batchSize int) (RetentionResult, error)`.

- [ ] **Step 1: Add budget boundary tests**

Use per-case estimated cost to prove: 79.99% reserves normally; 80% records one `budget_warning` event; 100% pauses P1/P2 assignments; P0 protocol/route/billing sentinels remain claimable only while their reserved cost fits the absolute run limit; no reservation may make `reserved_cost > budget_limit`. Also prove raw responses and verifier logs become deletable after 30 days, while aggregate snapshots and redacted evidence remain until 13 calendar months and are then deleted in bounded batches.

- [ ] **Step 2: Run tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/repository -run 'EvaluationOrchestrator|EvaluationQueue'`

Expected: FAIL with undefined orchestrator/queue.

- [ ] **Step 3: Implement database-first scheduling**

Commit run/assignment changes before publishing Redis channel `radar:assignments`. Treat publish failure as a metric/log event, not a transaction failure. Worker long-poll always queries PostgreSQL before waiting on Redis, then queries again after wake or timeout.

- [ ] **Step 4: Add scheduler lifecycle**

Register a single leader-elected 30-second tick using the existing `LeaderLockCache` and `TimingWheelService`. `Tick` creates due cron runs, requeues expired leases, marks runs finished when all samples are terminal, and stops creating runs when the plan is disabled. Register a separate daily retention job: delete S3 raw response/verifier artifacts after 30 days, retain hashes and redacted metadata, and delete aggregate/redacted evidence after PostgreSQL interval `13 months`; each run processes at most the configured batch size and records counts/errors in `evaluation_run_events`.

- [ ] **Step 5: Run tests and commit**

Run: `cd backend && go test -race -tags=unit ./internal/service ./internal/repository -run 'EvaluationOrchestrator|EvaluationQueue'`

Expected: PASS.

```bash
git add backend/internal/repository/evaluation_queue* backend/internal/service/evaluation_orchestrator* backend/internal/service/wire.go
git commit -m "feat(radar): schedule runs with hard budgets"
```

### Task 5: Expose Authenticated Worker Lease and Artifact APIs

**Files:**
- Create: `backend/internal/service/evaluation_artifact.go`
- Create: `backend/internal/repository/evaluation_artifact_s3.go`
- Test: `backend/internal/repository/evaluation_artifact_s3_test.go`
- Create: `backend/internal/server/middleware/radar_worker_auth.go`
- Test: `backend/internal/server/middleware/radar_worker_auth_test.go`
- Create: `backend/internal/handler/internal/radar_worker_handler.go`
- Test: `backend/internal/handler/internal/radar_worker_handler_test.go`
- Create: `backend/internal/server/routes/radar_worker.go`

**Interfaces:**
- Produces endpoints `POST /internal/radar/v1/leases:claim`, `POST /leases/:id/heartbeat`, `POST /leases/:id/evidence`, `POST /leases/:id/complete`, and `POST /leases/:id/fail`.
- Produces `POST /leases/:id/artifacts:presign` and `POST /leases/:id/artifacts:confirm`.
- Consumes: P0 signer to return a gateway evaluation token with each lease.

- [ ] **Step 1: Add Worker contract tests**

Verify bearer tokens are hashed before lookup, disabled Workers receive 401, capability mismatch returns 204, heartbeats return the new expiry, repeated evidence uploads return the original receipt, stale tokens return 409 `LEASE_FENCED`, and completion before evidence returns 409 `EVIDENCE_REQUIRED`.

- [ ] **Step 2: Run tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/server/middleware ./internal/handler/internal ./internal/repository -run 'RadarWorker|EvaluationArtifact'`

Expected: FAIL because the Worker API is absent.

- [ ] **Step 3: Implement bounded artifact presigning**

Allow MIME types `application/json`, `text/plain`, `application/gzip`, `application/zstd`, and `application/vnd.git.patch`; maximum object size comes from `Radar.MaxArtifactBytes`. Object keys are server-generated as `radar/<run>/<sample>/<assignment>/<uuid>`. Confirmation verifies object metadata, SHA-256, byte size, and lease ownership before setting `scan_status='pending'`.

- [ ] **Step 4: Implement evidence and completion transitions**

Require `case_content_sha256`, `execution_image_digest`, request/response hashes, route trace ID, artifact receipts, timing, and Worker environment fingerprint. Validate the route trace belongs to the same run/sample before `evidence_uploaded`. Completion only advances to `grading`; it does not accept Worker-reported scores.

- [ ] **Step 5: Run Worker API tests and commit**

Run: `cd backend && go test -tags=unit ./internal/server/middleware ./internal/handler/internal ./internal/repository -run 'RadarWorker|EvaluationArtifact'`

Expected: PASS.

```bash
git add backend/internal/service/evaluation_artifact.go backend/internal/repository/evaluation_artifact_s3* backend/internal/server/middleware/radar_worker_auth* backend/internal/handler/internal/radar_worker_handler* backend/internal/server/routes/radar_worker.go
git commit -m "feat(radar): add controlled worker lease api"
```

### Task 6: Add Admin Plan/Run APIs, DI, and End-to-End Recovery Test

**Files:**
- Create: `backend/internal/handler/admin/evaluation_plan_handler.go`
- Create: `backend/internal/handler/admin/evaluation_run_handler.go`
- Test: `backend/internal/handler/admin/evaluation_handler_test.go`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/internal/server/router.go`
- Modify: Wire provider files and `backend/cmd/server/wire_gen.go`
- Create: `backend/internal/integration/radar_control_plane_e2e_test.go`
- Modify: `docs/model-quality-radar-configuration.md`

**Interfaces:**
- Produces Admin CRUD/list endpoints under `/api/v1/admin/radar/plans` and `/api/v1/admin/radar/runs`.
- Produces run actions `:start`, `:cancel`, and `:retry-failed` with existing audit logging.

- [ ] **Step 1: Write the crash/recovery E2E test**

Create and publish a two-case dataset, start a paired run, claim an assignment, stop heartbeats, advance past lease TTL, claim the replacement, prove the old lease is fenced, upload evidence twice, and assert exactly one sample advances to grading.

- [ ] **Step 2: Run the E2E test and verify red**

Run: `cd backend && go test -tags=integration ./internal/integration -run RadarControlPlaneRecovery`

Expected: FAIL because handlers and DI are not wired.

- [ ] **Step 3: Add handlers, routes, and Wire providers**

Use pagination limits `1..200`, reject unknown sort fields, require `Idempotency-Key` on start/cancel/retry actions, and return RFC 3339 UTC timestamps. Add `Evaluation` to `AdminHandlers`; register Worker routes outside `/api/v1/admin` so Admin JWT/API-key auth cannot substitute for Worker auth.

Run: `cd backend && go generate ./cmd/server`

- [ ] **Step 4: Document Worker and artifact configuration**

Document token rotation, lease TTL, heartbeat interval, S3 endpoint/bucket/prefix, presign expiry, maximum artifact bytes, Worker capabilities, and the rule that Workers have no database credentials.

- [ ] **Step 5: Run control-plane verification**

Run: `cd backend && go test -tags=unit ./...`

Run: `cd backend && go test -tags=integration ./internal/integration ./internal/repository -run 'RadarControlPlane|Evaluation'`

Run: `cd backend && golangci-lint run ./...`

Expected: all commands PASS.

- [ ] **Step 6: Commit P1 control plane**

```bash
git add backend docs/model-quality-radar-configuration.md
git commit -m "feat(radar): complete evaluation control plane"
```

## Control-Plane Acceptance Gate

- A published dataset cannot be modified and all execution inputs are content-addressed.
- Two Workers never hold a valid lease for the same assignment attempt.
- A stale lease cannot heartbeat, upload evidence, fail, or complete after reassignment.
- Redis loss delays notification but does not lose or duplicate assignments.
- Duplicate API delivery does not duplicate samples, evidence, artifacts, or grading transitions.
- Budget and concurrency caps are enforced in PostgreSQL transactions before work becomes claimable.
