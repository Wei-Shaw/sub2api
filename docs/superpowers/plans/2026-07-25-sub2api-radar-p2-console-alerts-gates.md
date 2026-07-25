# sub2api Radar P2 Console, Alerts, and Release Gates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn radar evidence into an auditable operating workflow with scoped RBAC, model-health views, attribution alerts, approved baselines, and release decisions that become enforceable after a two-week calibration period.

**Architecture:** Go services consume immutable aggregate snapshots and route evidence to create alerts and gate decisions. Baselines and policies are explicitly versioned; baseline promotion requires two different approvers with Quality Admin and Release Manager roles. A dense Vue admin workspace exposes overview, model matrix, runs, alerts, datasets, gates, and Worker health, while all mutating actions use existing audit and idempotency middleware.

**Tech Stack:** Go 1.26.5, Gin, PostgreSQL, OpenTelemetry metrics, Vue 3, TypeScript 5.6, Pinia, Vue Router, Chart.js, Vitest, existing Tailwind/component conventions.

## Global Constraints

- Requires completion of the P0, control-plane, and Worker/grading/statistics plans.
- The first 14 full days are record-only; no gate blocks until `enforcement_starts_at` is reached.
- Any new P0 Coding, protocol, Tool Call, billing, or route-identity failure blocks after calibration.
- A critical domain regression of at least 3 percentage points blocks or requires explicit review only when the paired 95% CI excludes zero.
- A significant aggregate regression of at least 2 percentage points blocks only when the paired 95% CI excludes zero.
- P99, error rate, retry rate, truncation, and cost have independent SLO rules; improved quality cannot mask reliability failure and vice versa.
- Missing samples, wide confidence intervals, Judge disagreement, or missing route evidence yield `insufficient_evidence`, never `pass` or `regressed`.
- P1/P2 can recommend isolation, channel disable, or route rollback but cannot execute production routing changes automatically.
- Raw account/channel IDs and confidential case contents are never returned in list/overview APIs.

---

## File Structure

- `backend/migrations/193_add_radar_governance.sql`: RBAC bindings, baselines, approvals, policies, decisions, waivers, alerts, and attribution records.
- `backend/internal/service/evaluation_rbac.go`: radar roles and permission checks.
- `backend/internal/repository/evaluation_governance_repo.go`: governance persistence.
- `backend/internal/service/evaluation_baseline_service.go`: two-party baseline promotion.
- `backend/internal/service/evaluation_gate_service.go`: calibration and gate rules.
- `backend/internal/service/evaluation_alert_service.go`: alert lifecycle and attribution.
- `backend/internal/service/evaluation_report_service.go`: overview/matrix/run/report projections.
- `backend/internal/handler/admin/evaluation_governance_handler.go`: RBAC, baseline, policy, gate, waiver, and alert actions.
- `backend/internal/handler/admin/evaluation_report_handler.go`: read APIs and exports.
- `backend/internal/handler/evaluation_health_handler.go`: enterprise-customer-safe model health API.
- `backend/internal/observability/radar_metrics.go`: OTel counters, histograms, and gauges.
- `radar-worker/load/k6/multitenant-inference.js`: bounded multi-tenant inference load profile.
- `radar-worker/deploy/chaos/`: staging-only Toxiproxy and Chaos Mesh experiments.
- `frontend/src/api/admin/radar.ts`: typed API client.
- `frontend/src/api/radar.ts`: sanitized customer health client.
- `frontend/src/stores/radar.ts`: filters, polling, and cache state.
- `frontend/src/views/admin/radar/`: shell and seven operational views.
- `frontend/src/views/user/ModelHealthView.vue`: authenticated customer health view without route internals.
- `frontend/src/components/admin/radar/`: tables, charts, state badges, evidence drawer, and action dialogs.
- `frontend/src/router/index.ts`, `frontend/src/components/layout/AppSidebar.vue`: route/navigation integration.
- `frontend/src/i18n/locales/{zh,en}/admin/radar.ts`: all radar copy.

### Task 1: Add Governance Schema and Scoped Radar RBAC

**Files:**
- Create: `backend/migrations/193_add_radar_governance.sql`
- Create: `backend/internal/service/evaluation_rbac.go`
- Test: `backend/internal/service/evaluation_rbac_test.go`
- Create: `backend/internal/repository/evaluation_governance_repo.go`
- Test: `backend/internal/repository/evaluation_governance_repo_integration_test.go`
- Modify: `backend/internal/config/config.go`

**Interfaces:**
- Produces roles `viewer`, `test_operator`, `quality_admin`, `release_manager`, and `platform_admin`.
- Produces: `RadarAuthorizer.Require(ctx context.Context, actorID int64, permission RadarPermission) error`.
- Produces: `RadarAuthorizer.ListPermissions(ctx context.Context, actorID int64) ([]RadarPermission, error)`.

- [ ] **Step 1: Add failing permission and constraint tests**

Test every role/permission pair, disabled bindings, duplicate bindings, self-approval, same-person dual approval, expired waiver use, and a deployment with radar enabled but no Platform Admin bootstrap IDs.

```go
require.NoError(t, auth.Require(ctx, qualityAdminID, PermissionDatasetPublish))
require.ErrorIs(t, auth.Require(ctx, qualityAdminID, PermissionRouteAction), ErrRadarForbidden)
require.NoError(t, auth.Require(ctx, platformAdminID, PermissionRoleManage))
```

- [ ] **Step 2: Run tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/service -run RadarRBAC && go test -tags=integration ./internal/repository -run EvaluationGovernance`

Expected: FAIL because governance tables and authorizer do not exist.

- [ ] **Step 3: Create governance tables and exact permissions**

Create `evaluation_role_bindings`, `evaluation_baselines`, `evaluation_baseline_approvals`, `evaluation_gate_policies`, `evaluation_gate_decisions`, `evaluation_gate_waivers`, `evaluation_alerts`, `evaluation_alert_events`, and `evaluation_attributions`. Use unique active bindings, immutable policy version numbers, decision evidence JSON plus hash, waiver expiry, and append-only approval/alert-event rows.

Permission mapping:

```go
var radarRolePermissions = map[RadarRole][]RadarPermission{
    RoleViewer:         {PermissionView},
    RoleTestOperator:   {PermissionView, PermissionRunStart, PermissionRunRetry, PermissionWorkerManage},
    RoleQualityAdmin:   {PermissionView, PermissionDatasetManage, PermissionDatasetPublish, PermissionPolicyManage, PermissionBaselineQualityApprove},
    RoleReleaseManager: {PermissionView, PermissionGateDecide, PermissionGateWaive, PermissionBaselineReleaseApprove},
    RolePlatformAdmin:  {PermissionView, PermissionRoleManage, PermissionRouteAction},
}
```

- [ ] **Step 4: Bootstrap without implicit broad grants**

Add `Radar.BootstrapPlatformAdminIDs []int64`. When radar is enabled and zero active Platform Admin bindings exist, startup must require at least one configured active admin ID and insert those bindings transactionally. Never grant all existing admins automatically.

- [ ] **Step 5: Run tests and commit**

Run: `cd backend && go test -tags=unit ./internal/service -run RadarRBAC && go test -tags=integration ./internal/repository -run EvaluationGovernance`

Expected: PASS.

```bash
git add backend/migrations/193_add_radar_governance.sql backend/internal/service/evaluation_rbac* backend/internal/repository/evaluation_governance_repo* backend/internal/config
git commit -m "feat(radar): add governance and scoped rbac"
```

### Task 2: Promote Immutable Baselines with Two-Party Approval

**Files:**
- Create: `backend/internal/service/evaluation_baseline_service.go`
- Test: `backend/internal/service/evaluation_baseline_service_test.go`
- Create: `backend/internal/handler/admin/evaluation_baseline_handler.go`
- Test: `backend/internal/handler/admin/evaluation_baseline_handler_test.go`

**Interfaces:**
- Produces: `Propose(ctx, actorID int64, input BaselineProposal) (*Baseline, error)`.
- Produces: `Approve(ctx, actorID int64, baselineID uuid.UUID, role RadarRole) (*Baseline, error)`.
- Produces: `Promote(ctx, actorID int64, baselineID uuid.UUID) (*Baseline, error)`.
- Produces endpoints under `/api/v1/admin/radar/baselines`.

- [ ] **Step 1: Write failing baseline tests**

Require a passed full run, no open P0/P1 alerts, immutable dataset/grader/route/build/model parameter hashes, Quality Admin approval, Release Manager approval from a different user, and idempotent promotion. Reject a run with insufficient evidence or a changed route profile.

- [ ] **Step 2: Run tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run EvaluationBaseline`

Expected: FAIL because baseline service is absent.

- [ ] **Step 3: Implement proposal and approval snapshots**

Hash canonical JSON containing `run_id`, dataset manifest, grader versions, execution images, model routes and parameters, route profile version, build artifact version, aggregate IDs, and policy version. Store approvals against that hash. Any proposal mutation invalidates prior approvals by creating a new baseline row, never by updating the evidence hash.

- [ ] **Step 4: Implement promotion transaction**

Lock the candidate baseline and current model-route baseline, validate two distinct approvers and alerts again, retire the prior active baseline, activate the candidate, and append an audit event in one transaction.

- [ ] **Step 5: Run tests and commit**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run EvaluationBaseline`

Expected: PASS.

```bash
git add backend/internal/service/evaluation_baseline_service* backend/internal/handler/admin/evaluation_baseline_handler*
git commit -m "feat(radar): require dual approval for baselines"
```

### Task 3: Evaluate Calibration-Aware Release Gates

**Files:**
- Create: `backend/internal/service/evaluation_gate_service.go`
- Test: `backend/internal/service/evaluation_gate_service_test.go`
- Create: `backend/internal/handler/admin/evaluation_gate_handler.go`
- Test: `backend/internal/handler/admin/evaluation_gate_handler_test.go`

**Interfaces:**
- Produces: `Evaluate(ctx context.Context, input GateEvaluationInput) (*GateDecision, error)`.
- Produces decision statuses `recorded`, `passed`, `blocked`, `review_required`, `insufficient_evidence`, and `waived`.
- Produces endpoint `POST /api/v1/admin/radar/gates:evaluate` plus policy/decision/waiver APIs.

- [ ] **Step 1: Add a complete gate truth-table test**

Cover observation day 13/day 14, each P0 domain, route mismatch, -2/-3 exact boundaries, CI upper equal to and below zero, insufficient pairs, wide CI, P99 SLO, error/retry/truncation/cost SLOs, existing baseline failures versus new candidate failures, active waiver, expired waiver, and duplicate evaluate calls.

```go
policy := GatePolicy{
    ObservationDays: 14, CriticalDomainDeltaPP: -3, AggregateDeltaPP: -2,
    ConfidenceLevel: 0.95, RequireCIExcludeZero: true,
}
```

- [ ] **Step 2: Run tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run EvaluationGate`

Expected: FAIL because the gate engine is absent.

- [ ] **Step 3: Implement deterministic precedence**

Apply rules in this order: invalid/missing evidence; record-only calibration; new P0 failures or route mismatch; independent reliability/SLO blocks; critical-domain significant regression; aggregate significant regression; manual-review conditions; pass. Save all evaluated rule IDs, inputs, aggregate/evidence IDs, result, and policy hash.

- [ ] **Step 4: Implement controlled waivers**

Require Release Manager permission and fields `business_reason`, `risk_owner_user_id`, `mitigation`, `expires_at`, and `retest_plan`. Expired waivers never affect new decisions. Waivers change the effective status while preserving the original blocked decision and cannot promote a baseline.

- [ ] **Step 5: Run tests and commit**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run EvaluationGate`

Expected: PASS.

```bash
git add backend/internal/service/evaluation_gate_service* backend/internal/handler/admin/evaluation_gate_handler*
git commit -m "feat(radar): add calibrated release gates"
```

### Task 4: Detect, Attribute, Alert, and Close the Recovery Loop

**Files:**
- Create: `backend/internal/service/evaluation_alert_service.go`
- Test: `backend/internal/service/evaluation_alert_service_test.go`
- Create: `backend/internal/handler/admin/evaluation_alert_handler.go`
- Test: `backend/internal/handler/admin/evaluation_alert_handler_test.go`
- Modify: `backend/internal/service/notification_email_service.go`

**Interfaces:**
- Produces: `ProcessAggregate(ctx context.Context, snapshotID uuid.UUID) ([]Alert, error)`.
- Produces: `Acknowledge`, `Resolve`, `RequestDiagnosticRun`, and `RecordRecoveryTest` actions.
- Produces attribution causes `upstream_model`, `channel_or_pool`, `gateway_protocol`, `service_quality`, and `insufficient_evidence`.

- [ ] **Step 1: Write attribution and lifecycle tests**

Use fixtures for: multiple channels fall together; one redacted channel/pool falls; sub2api falls while official direct stays stable; quality stable while P99/errors worsen; too few samples; route evidence missing; repeated same signal; recovered aggregate; and regression recurrence after closure.

- [ ] **Step 2: Run tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run EvaluationAlert`

Expected: FAIL because alert service is absent.

- [ ] **Step 3: Implement evidence-based attribution**

Require at least two independent route slices before choosing `upstream_model`; use the stable redacted refs for channel/pool attribution; require a configured official-direct comparator for `gateway_protocol`; separate `service_quality` when capability CI is stable but reliability SLO fails. Otherwise select `insufficient_evidence` and schedule a diagnostic run instead of asserting cause.

- [ ] **Step 4: Implement deduplication and recovery**

Deduplicate open alerts by `(model_route, capability_domain, cause, policy_version)`. Append events for observation, acknowledgment, diagnostic request, recommendation, recovery test, and closure. Close only after a successful paired recovery run meets the same policy. Email P0/P1 alerts through the existing notification service; include only redacted summaries and Admin deep links.

- [ ] **Step 5: Run tests and commit**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run EvaluationAlert`

Expected: PASS.

```bash
git add backend/internal/service/evaluation_alert_service* backend/internal/handler/admin/evaluation_alert_handler* backend/internal/service/notification_email_service.go
git commit -m "feat(radar): attribute and manage quality alerts"
```

### Task 5: Build Stable Read APIs and Evidence-Safe Exports

**Files:**
- Create: `backend/internal/service/evaluation_report_service.go`
- Test: `backend/internal/service/evaluation_report_service_test.go`
- Create: `backend/internal/handler/admin/evaluation_report_handler.go`
- Test: `backend/internal/handler/admin/evaluation_report_handler_test.go`
- Create: `backend/internal/handler/evaluation_health_handler.go`
- Test: `backend/internal/handler/evaluation_health_handler_test.go`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/internal/server/routes/user.go`
- Modify: Wire provider files and `backend/cmd/server/wire_gen.go`

**Interfaces:**
- Produces `/api/v1/admin/radar/overview`, `/models`, `/runs`, `/runs/:id`, `/alerts`, `/workers`, `/datasets`, `/gates`, and `/reports/:run_id`.
- Produces authenticated customer endpoint `/api/v1/radar/health` with model alias, health state, public domain scores, CI, sample count, freshness, P99, and error rate only.
- Produces JSON and CSV exports with identical filter semantics.

- [ ] **Step 1: Write API projection and redaction tests**

Assert 24h/48h deltas, CI/sample/freshness fields, matrix columns, run progress/budget/failure classes, alert attribution, Worker lease age, gate evidence, UTC timestamps, stable pagination, and absence of prompt specs, expected answers, raw resource IDs, credentials, and hidden reasoning. For `/api/v1/radar/health`, additionally assert the response omits route profile, fallback chain, case IDs, grader explanation, policy internals, cost amount, and alert attribution slices.

- [ ] **Step 2: Run tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin -run EvaluationReport`

Expected: FAIL because reports are absent.

- [ ] **Step 3: Implement query projections**

Use bounded SQL queries keyed by run/model/time indexes; list page size defaults to 50 and caps at 200. Overview freshness is the age of the newest completed aggregate, not request time. Show separate capability, protocol, reliability, performance, and cost states.

- [ ] **Step 4: Implement report exports**

CSV begins with explicit schema/version columns and one row per model/domain/window. JSON includes policy/baseline/evidence hashes and aggregate IDs. Detailed case failures require `PermissionView`; confidential payload download additionally requires `PermissionDatasetManage` and creates a sensitive-read audit record.

- [ ] **Step 5: Wire, verify, and commit**

Run: `cd backend && go generate ./cmd/server`

Run: `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin ./internal/server/routes -run 'EvaluationReport|Radar'`

Expected: PASS.

```bash
git add backend/internal/service/evaluation_report_service* backend/internal/handler/admin/evaluation_report_handler* backend/internal/handler/evaluation_health_handler* backend/internal/handler/handler.go backend/internal/server/routes/admin.go backend/internal/server/routes/user.go backend/internal/*/wire.go backend/cmd/server/wire_gen.go
git commit -m "feat(radar): expose safe quality reports"
```

### Task 6: Add Typed Frontend API, Routes, Navigation, and State

**Files:**
- Create: `frontend/src/api/admin/radar.ts`
- Test: `frontend/src/api/__tests__/admin.radar.spec.ts`
- Create: `frontend/src/api/radar.ts`
- Test: `frontend/src/api/__tests__/radar.health.spec.ts`
- Create: `frontend/src/stores/radar.ts`
- Test: `frontend/src/stores/__tests__/radar.spec.ts`
- Create: `frontend/src/views/admin/radar/RadarShell.vue`
- Create: `frontend/src/views/user/ModelHealthView.vue`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/components/layout/AppSidebar.vue`
- Modify: `frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `frontend/src/i18n/locales/en/admin/index.ts`
- Create: `frontend/src/i18n/locales/zh/admin/radar.ts`
- Create: `frontend/src/i18n/locales/en/admin/radar.ts`
- Test: `frontend/src/__tests__/integration/navigation.spec.ts`

**Interfaces:**
- Produces route family `/admin/radar/{overview,models,runs,alerts,datasets,gates,workers}` and authenticated customer route `/model-health`.
- Produces typed filters and polling with 30-second overview/Worker refresh and no polling on hidden tabs.

- [ ] **Step 1: Write API, store, and navigation tests**

Verify exact endpoint/parameter encoding, request cancellation when filters change, stale response suppression, polling pause/resume, Admin-only route guards, customer health access for normal authenticated users, active sidebar state, and Chinese/English navigation labels.

- [ ] **Step 2: Run tests and verify red**

Run: `cd frontend && pnpm test:run -- src/api/__tests__/admin.radar.spec.ts src/api/__tests__/radar.health.spec.ts src/stores/__tests__/radar.spec.ts src/__tests__/integration/navigation.spec.ts`

Expected: FAIL because radar frontend modules are absent.

- [ ] **Step 3: Implement the typed client and store**

Define discriminated unions for health, failure class, decision status, and evidence sufficiency. Store filters in route query parameters; use `AbortController` per request and compare monotonically increasing request IDs before committing state.

- [ ] **Step 4: Add a top-level operational workspace route**

Add one top-level “质量雷达 / Quality Radar” Admin item and one “模型健康 / Model Health” customer item beside channel status, using icons already present in the project. `RadarShell` uses compact tabs and a router view; it contains no marketing copy, nested cards, or decorative background. `ModelHealthView` shows only the sanitized health contract. All actions are icon plus text only when the command would be ambiguous, with existing tooltip conventions.

- [ ] **Step 5: Run tests and commit**

Run: `cd frontend && pnpm test:run -- src/api/__tests__/admin.radar.spec.ts src/api/__tests__/radar.health.spec.ts src/stores/__tests__/radar.spec.ts src/__tests__/integration/navigation.spec.ts && pnpm typecheck`

Expected: PASS.

```bash
git add frontend/src/api/admin/radar.ts frontend/src/api/radar.ts frontend/src/api/__tests__/admin.radar.spec.ts frontend/src/api/__tests__/radar.health.spec.ts frontend/src/stores frontend/src/views/admin/radar/RadarShell.vue frontend/src/views/user/ModelHealthView.vue frontend/src/router/index.ts frontend/src/components/layout/AppSidebar.vue frontend/src/i18n frontend/src/__tests__/integration/navigation.spec.ts
git commit -m "feat(radar-ui): add workspace navigation and state"
```

### Task 7: Implement Overview, Matrix, Runs, Alerts, Datasets, Gates, and Workers Views

**Files:**
- Create: `frontend/src/views/admin/radar/RadarOverviewView.vue`
- Create: `frontend/src/views/admin/radar/RadarModelsView.vue`
- Create: `frontend/src/views/admin/radar/RadarRunsView.vue`
- Create: `frontend/src/views/admin/radar/RadarAlertsView.vue`
- Create: `frontend/src/views/admin/radar/RadarDatasetsView.vue`
- Create: `frontend/src/views/admin/radar/RadarGatesView.vue`
- Create: `frontend/src/views/admin/radar/RadarWorkersView.vue`
- Create: `frontend/src/components/admin/radar/RadarStatusBadge.vue`
- Create: `frontend/src/components/admin/radar/RadarDeltaCell.vue`
- Create: `frontend/src/components/admin/radar/RadarTrendChart.vue`
- Create: `frontend/src/components/admin/radar/RadarEvidenceDrawer.vue`
- Create: `frontend/src/components/admin/radar/RadarActionDialog.vue`
- Create: `frontend/src/views/admin/radar/__tests__/radarViews.spec.ts`

**Interfaces:**
- Consumes the typed API/store from Task 6.
- Produces complete loading, empty, stale, insufficient-evidence, partial-error, forbidden, and populated states.

- [ ] **Step 1: Write view behavior tests first**

Test table headers and cell semantics; stale-data timestamp; CI and sample count; capability/reliability separation; P0 prominence; filters; run retry/cancel confirmation; alert timeline/recovery action; baseline dual approval; waiver expiry fields; permission-hidden actions; Worker heartbeat/lease health; and long Chinese/English text wrapping at 375px.

- [ ] **Step 2: Run tests and verify red**

Run: `cd frontend && pnpm test:run -- src/views/admin/radar/__tests__/radarViews.spec.ts`

Expected: FAIL because views do not exist.

- [ ] **Step 3: Build the dense operational layouts**

Overview uses a full-width status strip, domain table, trend chart, open-alert list, gate queue, and Worker/queue health. Models is a horizontally scrollable matrix with sticky model column. Runs, alerts, datasets, gates, and Workers use unframed filter bars and dense tables; only repeated alert/run items and dialogs may use bordered cards, with radius at most 8px.

- [ ] **Step 4: Implement evidence and action workflows**

The evidence drawer shows baseline/candidate score, delta, CI, samples, failure-class counts, redacted route chain, token/latency/cost, response diff, grader explanation, hashes, and audit timeline. Mutations require confirmation, surface server validation, disable during submission, send idempotency keys, and refresh only the affected resource.

- [ ] **Step 5: Verify responsive and accessibility behavior**

At 375px, tables scroll instead of shrinking text below the existing body minimum; buttons keep fixed icon dimensions; no text overlaps or is clipped; drawers become full viewport. Every status uses text plus icon, never color alone. Charts have table equivalents and meaningful accessible labels.

- [ ] **Step 6: Run frontend checks and commit**

Run: `cd frontend && pnpm test:run -- src/views/admin/radar src/api/__tests__/admin.radar.spec.ts src/stores/__tests__/radar.spec.ts`

Run: `cd frontend && pnpm typecheck && pnpm lint:check`

Expected: all commands PASS.

```bash
git add frontend/src/views/admin/radar frontend/src/components/admin/radar
git commit -m "feat(radar-ui): build quality operations console"
```

### Task 8: Add SLO Telemetry, Full E2E Gates, and Calibration Launch Controls

**Files:**
- Create: `backend/internal/observability/radar_metrics.go`
- Test: `backend/internal/observability/radar_metrics_test.go`
- Create: `backend/internal/integration/radar_release_gate_e2e_test.go`
- Modify: `frontend/src/__tests__/integration/navigation.spec.ts`
- Modify: `.github/workflows/backend-ci.yml`
- Modify: `.github/workflows/radar-worker-ci.yml`
- Create: `docs/operations/model-quality-radar.md`

**Interfaces:**
- Produces OTel metrics for assignment age, Worker heartbeat, evidence-link rate, grading delay, alert delay, duplicate-score attempts, failure classes, cost, and gate outcomes.
- Produces one release-gate endpoint usable from CI with exit semantics documented for `passed`, `blocked`, `review_required`, and `insufficient_evidence`.

- [ ] **Step 1: Write metric and release E2E tests**

Test the four platform SLOs: 95% of 15-minute sentinels finish before the next period; evidence association is at least 99.9%; 99% of completed evidence is scored within 10 minutes; P0 alert creation is within 5 minutes; duplicate current score count remains zero. Run a known-good candidate, a -3 point critical regression, a P0 protocol regression, an insufficient sample, and an active waiver.

- [ ] **Step 2: Run tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/observability -run Radar && go test -tags=integration ./internal/integration -run RadarReleaseGate`

Expected: FAIL because metrics and final workflow are not wired.

- [ ] **Step 3: Instrument without high-cardinality labels**

Allowed labels are domain, priority, status, failure class, Worker capability, and gate result. Never label by run ID, sample ID, prompt, account/channel ref, model response, or request ID. Expose run/sample identifiers only in logs/traces.

- [ ] **Step 4: Implement the calibration launch sequence**

On enablement, create a versioned default policy in record-only mode with `enforcement_starts_at = enabled_at + 14*24h`. A daily calibration report shows observed noise, false positives, unstable-case rate, and proposed threshold changes. Policy changes require Quality Admin permission, a written reason, before/after values, and a new version; they never rewrite prior decisions. Document the scale trigger: when valid samples exceed 10,000,000 or radar trend queries breach the database latency budget for seven consecutive days, open an architecture review to add Outbox/CDC to ClickHouse while retaining PostgreSQL as the control-plane source of truth.

- [ ] **Step 5: Run complete repository verification**

Run: `cd backend && go test -tags=unit ./... && golangci-lint run ./...`

Run: `cd backend && go test -tags=integration ./...`

Run: `cd radar-worker && pytest -q && ruff check . && mypy src`

Run: `cd frontend && pnpm test:run && pnpm typecheck && pnpm lint:check && pnpm build`

Expected: all commands PASS.

- [ ] **Step 6: Perform browser verification**

Start the repository's normal frontend/backend dev environment. Verify desktop `1440x900` and mobile `375x812` screenshots for every radar route, inspect network/console errors, test filters, evidence drawer, confirmation dialogs, long labels, empty/error/stale states, and prove no element overlap or horizontal page overflow outside the matrix/table scrollers.

- [ ] **Step 7: Commit production readiness**

```bash
git add backend frontend radar-worker .github/workflows docs/operations/model-quality-radar.md
git commit -m "feat(radar): add release gates and production operations"
```

### Task 9: Validate Multi-Tenant Load, Chaos Recovery, and Failover

**Files:**
- Create: `radar-worker/load/k6/multitenant-inference.js`
- Create: `radar-worker/load/k6/radar-sentinel.js`
- Create: `radar-worker/deploy/chaos/toxiproxy-compose.yaml`
- Create: `radar-worker/deploy/chaos/upstream-latency.yaml`
- Create: `radar-worker/deploy/chaos/worker-pod-kill.yaml`
- Create: `radar-worker/deploy/chaos/redis-network-partition.yaml`
- Create: `radar-worker/deploy/chaos/artifact-store-outage.yaml`
- Create: `backend/internal/integration/radar_chaos_test.go`
- Create: `radar-worker/tests/e2e/test_control_plane_failover.py`
- Modify: `docs/operations/model-quality-radar.md`

**Interfaces:**
- Produces k6 inputs `BASE_URL`, `TENANT_KEYS_JSON`, `MODEL`, `TARGET_RPS`, `P99_LIMIT_MS`, `ERROR_RATE_LIMIT`, and `DURATION`.
- Produces staging-only chaos experiments labeled `sub2api.io/radar-chaos=true` and blocked from namespaces lacking that label.
- Produces a recovery report with RPO, RTO, lost/duplicated assignments, customer P99 change, and evidence-association rate.

- [ ] **Step 1: Write deterministic load-script tests and dry-run validation**

Parse `TENANT_KEYS_JSON` as an array of dedicated synthetic tenant keys, select tenants round-robin, mix non-streaming and SSE requests 30/70, and tag metrics only by protocol/model/test tenant class. Set k6 thresholds `http_req_failed < ERROR_RATE_LIMIT`, `http_req_duration p(99) < P99_LIMIT_MS`, and `checks rate > 0.999`. Reject customer or evaluation production keys by requiring each fixture key name to start with `loadtest-`.

- [ ] **Step 2: Add integration fault fixtures**

Use Toxiproxy/testcontainers to inject upstream latency, connection reset, Redis unavailability, S3 refusal, Worker death after evidence upload, and PostgreSQL connection loss during claim. Assert upstream/infra classification, bounded retry, database-first recovery after Redis returns, no score duplication, no lost committed assignment, and artifact upload resumption with the original idempotency key.

- [ ] **Step 3: Run local chaos tests and verify red/green**

Run: `cd backend && go test -tags=integration ./internal/integration -run RadarChaos`

Run: `cd radar-worker && pytest tests/e2e/test_control_plane_failover.py -q`

Expected before fixtures: FAIL because scenarios are absent. Expected after implementation: PASS with zero lost committed assignments and zero duplicate current scores.

- [ ] **Step 4: Add guarded Kubernetes experiments**

Every Chaos Mesh object targets only namespaces and pods carrying `sub2api.io/radar-chaos=true`, has a duration at most 10 minutes, and includes an abort command in its annotation. Cover 500ms/2s upstream delay, one Worker pod kill per minute, a five-minute Redis network partition, and a three-minute artifact-store outage. Do not include automatic production route mutation or database data deletion.

- [ ] **Step 5: Execute the staging load ladder**

Run 10 minutes each at 1, 10, 50, 100, and 200 RPS, then a 30-minute steady-state target and a five-minute 2x burst. Run the radar sentinel simultaneously. Fail the stage when candidate P99 exceeds `P99_LIMIT_MS`, error rate exceeds its threshold, evaluation traffic raises the customer-traffic P99 by more than 5%, evidence association falls below 99.9%, or any budget/concurrency cap is bypassed.

- [ ] **Step 6: Exercise disaster recovery**

In staging, restart the PostgreSQL primary through the platform's managed failover, flush only the dedicated test Redis database, terminate half the Workers, and restart the Go control plane. Measure from fault injection to successful new lease/score visibility. Acceptance is RPO 0 for committed PostgreSQL state, RTO no greater than `lease_ttl + 2*worker_poll_interval`, no duplicate current score, and no evaluation traffic using a customer key or account pool.

- [ ] **Step 7: Document evidence and commit**

The operations guide records prechecks, exact commands, expected dashboards, abort criteria, rollback, result template, owners, and quarterly drill cadence. Store generated k6/chaos reports as 30-day radar artifacts and the signed summary as a 13-month aggregate record.

```bash
git add radar-worker/load radar-worker/deploy/chaos radar-worker/tests/e2e/test_control_plane_failover.py backend/internal/integration/radar_chaos_test.go docs/operations/model-quality-radar.md
git commit -m "test(radar): add load chaos and failover validation"
```

## Production Acceptance Gate

- Radar permissions are narrower than global Admin and baseline approval uses two different authorized people.
- The exact P0, -3 point critical-domain, and -2 point aggregate policies are versioned, tested at boundaries, and record-only for 14 days.
- Alerts expose evidence and attribution confidence, preserve separate quality/reliability/cost status, and require a passing recovery run to close.
- The console exposes complete workflows and all important loading/error/empty/stale/permission states on desktop and mobile.
- CI can consume a stable gate decision without treating insufficient evidence as pass.
- Staging load, chaos, and managed failover drills meet P99, evidence-link, RPO/RTO, and duplicate-score thresholds.
- No action in P1/P2 automatically changes production routing.
