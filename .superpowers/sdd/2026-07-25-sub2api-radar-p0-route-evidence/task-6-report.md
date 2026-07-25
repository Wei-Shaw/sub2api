# Task 6 Report: Wire, Migrate, and Verify P0 Isolation

## Implementation Summary

- Registered `NewEvaluationRouteEvidenceRepository` and the typed `NewEvaluationEvidenceMiddleware` constructor with Wire, regenerated `cmd/server/wire_gen.go`, and propagated the middleware through `ProvideRouter`, `SetupRouter`, and `RegisterGatewayRoutes`.
- Installed evidence finalization immediately after all 24 API-key authentication sites. This includes the versioned gateway, both Google-compatible groups, root aliases, Codex direct routes, image/video routes, and both Antigravity families.
- Added route-order coverage across seven representative gateway families. Each request proves signed evaluation authentication has populated both evaluation identity and a server-generated route trace before evidence middleware runs.
- Added a Docker-gated P0 integration test for normal/evaluation key isolation, success and four failure terminal classes, one row per trace, requested/resolved model linkage, billing fields, and HMAC-redacted route attempts.
- Added the operational environment names `RADAR_CONTEXT_SIGNING_KEY` and `RADAR_EVIDENCE_HASH_KEY`; the legacy `RADAR_SIGNING_SECRET` and `RADAR_HASHING_SECRET` names remain ordered fallbacks.
- Added deployment and isolated-identity provisioning documentation with exact API, SQL, limit, rotation, and privacy requirements.

## TDD RED Evidence

Route wiring RED:

```bash
GOCACHE=/tmp/codex-task6-gocache go test -tags=unit ./internal/server/routes -run GatewayRoutesRunEvaluationEvidence
```

Failed because `middleware.EvaluationEvidenceMiddleware` was undefined and the new middleware argument made the existing `RegisterGatewayRoutes` contract fail to compile.

Integration RED:

```bash
GOCACHE=/tmp/codex-task6-gocache go test -tags=integration ./internal/integration -run RadarP0
```

Failed because `NewEvaluationEvidenceMiddleware` did not exist.

Configuration RED:

```bash
GOCACHE=/tmp/codex-task6-gocache go test -tags=unit ./internal/config -run OperationalEnvironmentNames
```

Failed because the canonical signing-key environment name was not bound, leaving `radar.signing_secret` empty and causing enabled-Radar validation to fail.

The focused route, integration compile, and configuration tests turned green after adding the typed provider, router propagation, route placement, and canonical environment bindings.

## Generation

Wire was regenerated from the provider sets:

```bash
GOCACHE=/tmp/codex-task6-gocache go generate ./cmd/server
```

The generated diff is limited to constructing `EvaluationRouteEvidenceRepository`, constructing `EvaluationEvidenceMiddleware`, and passing the middleware to `ProvideRouter`.

## Verification

Controller focused route and configuration verification:

```bash
GOCACHE=/private/tmp/radar-p0-task6-gocache go test -tags=unit ./internal/server/routes ./internal/config
```

Exited 0; routes completed in 2.693s and config in 2.592s.

The full unit command first failed inside the sandbox only because `httptest`, `miniredis`, and other local test servers were denied socket binds with `operation not permitted`. The controller reran the exact command in controlled elevated mode:

```bash
GOCACHE=/private/tmp/radar-p0-task6-gocache go test -tags=unit ./...
```

Exited 0; all packages passed, with `internal/service` completing in 150.577s.

Controller focused integration verification:

```bash
GOCACHE=/private/tmp/radar-p0-task6-gocache go test -tags=integration ./internal/integration ./internal/repository -run 'RadarP0|MigrationsSchema|Evaluation'
```

Exited 0; integration completed in 1.384s and repository in 1.083s. Earlier verbose executions established that Docker is unavailable on this host, so the Testcontainers tests compiled but skipped before PostgreSQL assertions. This result must not be interpreted as executed database coverage.

The controller's PATH-based lint command was unavailable:

```bash
GOCACHE=/private/tmp/radar-p0-task6-gocache golangci-lint run ./...
```

Exited 127 with `command not found`.

Repository-pinned golangci-lint v2.9.0 was installed under `/tmp/codex-task6-bin`. Its first run found one real `errcheck` defect in the Task 5 evaluation repository:

```text
internal/repository/evaluation_route_evidence_repo.go:158:18: Error return value of `(*database/sql.Rows).Close` is not checked (errcheck)
```

After changing that defer to the repository's established checked-discard pattern, the pinned command completed cleanly:

```bash
GOCACHE=/tmp/codex-task6-gocache \
GOLANGCI_LINT_CACHE=/tmp/codex-task6-golangci-cache \
/tmp/codex-task6-bin/golangci-lint run ./...
```

Output: `0 issues.`

## Default-Tag Bootability Fix

Task 5 left `recordUsageEvidenceRepoStub` and `evaluationRecordUsageContext` in `gateway_record_usage_test.go`, which has a `//go:build unit` constraint. The untagged `openai_gateway_record_usage_test.go` also uses those helpers, so default-tag package analysis failed with undefined symbols and prevented the Task 6 full lint/build check.

The helpers were moved unchanged into the untagged `evaluation_record_usage_test.go`; no new behavioral coverage was added. This organization is required so both build modes can compile the shared production contract: the Anthropic and OpenAI billing paths attach finalized tokens, TTFT, latency, and billed amount to evaluation evidence on a live detached context, while evidence persistence remains best-effort and cannot change customer billing behavior.

Focused verification before the controller handoff passed in both modes:

```bash
GOCACHE=/tmp/codex-task6-gocache go test -count=1 ./internal/service -run 'RecordUsage.*Evidence|GatewayServiceRecordUsage'
GOCACHE=/tmp/codex-task6-gocache go test -count=1 -tags=unit ./internal/service -run 'RecordUsage.*Evidence|GatewayServiceRecordUsage'
```

Both commands exited 0.

## Self-Review

- Exact source counts are 24 API-key authentication occurrences and 24 evidence middleware occurrences; every pair is adjacent and authentication precedes evidence.
- Both Google authentication groups place evidence after `APIKeyAuthWithSubscriptionGoogle`, not before it.
- The seven-family route-order test covers the versioned gateway, Google-compatible route, root alias, Codex direct route, chat alias, Antigravity v1, and Antigravity Google route.
- The integration assertions cover `succeeded`, `upstream_failed`, `protocol_failed`, `client_cancelled`, and `gateway_failed`, plus normal-key copied-header rejection and evaluation-key missing-token rejection before inference.
- Billing assertions cover input/output tokens, TTFT, latency, and exact decimal billed amount. Privacy assertions reject raw `account_id`/`channel_id` JSON keys and raw numeric IDs in route references.
- Viper documents that multiple names passed to `BindEnv` are checked in specified order. Canonical names are therefore first and legacy names are fallbacks; existing legacy configuration tests remain intact.
- Documentation endpoint paths and JSON fields match the registered `/api/v1/admin/groups`, `/api/v1/admin/users`, and `/api/v1/keys` handlers. Migration `190_add_radar_route_evidence.sql`, the evaluation header name, and all documented configuration fields match source.
- `git diff --check` passed. The generated Wire file has only three additions and one changed call line.

## Concerns

- Docker-backed schema, convergence, and P0 lifecycle assertions did not execute on this host. They still need a Docker-capable CI runner or development machine.
- The normal PATH did not expose golangci-lint. Static analysis was completed with the repository-pinned v2.9.0 binary under `/tmp`; CI should continue to provide the pinned tool through its standard toolchain.
