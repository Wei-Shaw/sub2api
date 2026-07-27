# sub2api Radar P0 Route Evidence and Evaluation Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every authorized evaluation request an immutable identity and persist complete, redacted route, performance, token, and billing evidence without trusting client-supplied metadata.

**Architecture:** A dedicated evaluation API key is marked in the existing API-key record. A short-lived HMAC token binds that key to a run and sample; the authentication middleware validates it and installs a mutable per-request trace collector in `context.Context`. Gateway selection records attempts into the collector, a response middleware persists transport outcomes, and the existing usage paths attach final token and cost data through an idempotent upsert.

**Tech Stack:** Go 1.26.5, Gin, Ent, PostgreSQL, existing Wire DI, HMAC-SHA256, Testify, sqlmock.

## Global Constraints

- Work from upstream `sub2api` commit `43d4bae` or rebase this plan against the new head before editing.
- Evaluation traffic uses a dedicated user, group, API key, user concurrency limit, API-key quota, and API-key rate limits.
- Do not record customer prompts, hidden reasoning, upstream credentials, raw account IDs, or raw channel IDs in radar evidence.
- A normal API key presenting any `X-Sub2API-Evaluation-*` header is rejected with HTTP 403.
- An evaluation API key without a valid signed context is rejected with HTTP 403.
- `route_trace_id` is server-generated and cannot be supplied by the Worker.
- Evidence writes are idempotent by `route_trace_id`; transport finalization and billing attachment may arrive in either order.

---

## File Structure

- `backend/migrations/190_add_radar_route_evidence.sql`: P0 tables, constraints, and indexes.
- `backend/ent/schema/api_key.go`: evaluation-key marker.
- `backend/ent/schema/evaluation_route_evidence.go`: Ent representation of redacted evidence.
- `backend/internal/config/config.go`: radar signing, hashing, region, and route-profile settings.
- `backend/internal/pkg/ctxkey/ctxkey.go`: typed evaluation-context key.
- `backend/internal/service/evaluation_context.go`: signed context and request trace collector.
- `backend/internal/service/evaluation_route_evidence.go`: evidence domain types and recorder interface.
- `backend/internal/repository/evaluation_route_evidence_repo.go`: idempotent PostgreSQL upserts.
- `backend/internal/server/middleware/evaluation_context.go`: API-key/header enforcement and post-response finalization.
- `backend/internal/service/request_metadata.go`: shared attempt recording helpers.
- `backend/internal/service/gateway_usage_billing.go`: Anthropic/Gemini usage attachment.
- `backend/internal/service/openai_gateway_usage.go`: OpenAI-compatible usage attachment.
- `backend/internal/repository/wire.go`, `backend/internal/service/wire.go`, `backend/internal/server/middleware/wire.go`, `backend/cmd/server/wire_gen.go`: dependency wiring.
- `backend/internal/server/routes/gateway.go`: middleware placement after API-key authentication.
- `docs/model-quality-radar-configuration.md`: evaluation-key and P0 secret provisioning.

### Task 1: Persist Evaluation-Key Identity and Route Evidence

**Files:**
- Create: `backend/migrations/190_add_radar_route_evidence.sql`
- Create: `backend/ent/schema/evaluation_route_evidence.go`
- Modify: `backend/ent/schema/api_key.go`
- Modify: `backend/internal/service/api_key.go`
- Modify: `backend/internal/repository/api_key_repo.go`
- Test: `backend/internal/repository/migrations_schema_integration_test.go`
- Test: `backend/internal/repository/api_key_repo_integration_test.go`

**Interfaces:**
- Produces: `service.APIKey.IsEvaluation bool`.
- Produces: table `evaluation_route_evidence` keyed by `route_trace_id VARCHAR(64)`.

- [ ] **Step 1: Add failing schema assertions**

```go
requireColumn(t, tx, "api_keys", "is_evaluation", "boolean", 0, false)
requireColumn(t, tx, "evaluation_route_evidence", "route_trace_id", "character varying", 64, false)
requireColumn(t, tx, "evaluation_route_evidence", "fallback_chain", "jsonb", 0, false)
requireColumn(t, tx, "evaluation_route_evidence", "billed_amount", "numeric", 20, true)
```

Add an API-key repository test that creates an evaluation key and verifies `GetByKeyForAuth` returns `IsEvaluation == true`.

- [ ] **Step 2: Run the focused tests and verify red**

Run: `cd backend && go test -tags=integration ./internal/repository -run 'TestMigrationsSchema|TestAPIKeyRepository.*Evaluation'`

Expected: FAIL because `is_evaluation` and `evaluation_route_evidence` do not exist.

- [ ] **Step 3: Add the idempotent migration and Ent fields**

The migration must create this shape, with check constraints for terminal transport status and nonnegative counters:

```sql
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS is_evaluation BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS evaluation_route_evidence (
    route_trace_id VARCHAR(64) PRIMARY KEY,
    evaluation_run_id UUID NOT NULL,
    sample_id UUID NOT NULL,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id),
    request_id VARCHAR(128),
    requested_model VARCHAR(200) NOT NULL,
    resolved_model VARCHAR(200),
    route_profile_version VARCHAR(100) NOT NULL,
    provider VARCHAR(32),
    channel_ref VARCHAR(64),
    account_pool_ref VARCHAR(64),
    region VARCHAR(64) NOT NULL,
    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    fallback_chain JSONB NOT NULL DEFAULT '[]'::jsonb,
    finish_reason VARCHAR(64),
    input_tokens INT CHECK (input_tokens >= 0),
    output_tokens INT CHECK (output_tokens >= 0),
    ttft_ms INT CHECK (ttft_ms >= 0),
    latency_ms INT CHECK (latency_ms >= 0),
    billed_amount NUMERIC(20,8),
    transport_status VARCHAR(24) NOT NULL DEFAULT 'started'
      CHECK (transport_status IN ('started','succeeded','upstream_failed','protocol_failed','client_cancelled','gateway_failed')),
    error_code VARCHAR(100),
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_eval_route_evidence_run_sample
  ON evaluation_route_evidence (evaluation_run_id, sample_id);
CREATE INDEX IF NOT EXISTS idx_eval_route_evidence_model_finished
  ON evaluation_route_evidence (requested_model, finished_at DESC);
```

Add `field.Bool("is_evaluation").Default(false)` to `APIKey.Fields()`, mirror the table in the new Ent schema, and map `IsEvaluation` in create, update, auth-select, and `apiKeyEntityToService` paths.

- [ ] **Step 4: Generate Ent code and run green tests**

Run: `cd backend && go generate ./ent`

Run: `cd backend && go test -tags=integration ./internal/repository -run 'TestMigrationsSchema|TestAPIKeyRepository.*Evaluation'`

Expected: PASS.

- [ ] **Step 5: Commit the schema boundary**

```bash
git add backend/migrations/190_add_radar_route_evidence.sql backend/ent backend/internal/service/api_key.go backend/internal/repository/api_key_repo.go backend/internal/repository/*test.go
git commit -m "feat(radar): add evaluation key and route evidence schema"
```

### Task 2: Sign and Verify Evaluation Context

**Files:**
- Create: `backend/internal/service/evaluation_context.go`
- Test: `backend/internal/service/evaluation_context_test.go`
- Modify: `backend/internal/config/config.go`
- Test: `backend/internal/config/radar_config_test.go`
- Modify: `backend/internal/pkg/ctxkey/ctxkey.go`

**Interfaces:**
- Produces: `NewEvaluationContextSigner(key []byte, ttl time.Duration) (*EvaluationContextSigner, error)`.
- Produces: `Sign(EvaluationContext) (string, error)` and `Verify(token string, apiKeyID int64, now time.Time) (EvaluationContext, error)`.
- Produces: `WithEvaluationContext(context.Context, EvaluationContext) context.Context` and `EvaluationContextFromContext(context.Context) (EvaluationContext, bool)`.

- [ ] **Step 1: Write failing signer tests**

```go
func TestEvaluationContextSignerBindsAPIKeyAndExpiry(t *testing.T) {
    signer, err := NewEvaluationContextSigner([]byte(strings.Repeat("k", 32)), 5*time.Minute)
    require.NoError(t, err)
    issued := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
    token, err := signer.Sign(EvaluationContext{
        RunID: "018f4f20-3d12-7e50-9000-000000000001", SampleID: "018f4f20-3d12-7e50-9000-000000000002",
        DatasetVersion: "core-v1", ExpectedModelAlias: "qwen3-coder", ExpectedRouteProfile: "route-v42",
        APIKeyID: 41, IssuedAt: issued, ExpiresAt: issued.Add(5*time.Minute),
    })
    require.NoError(t, err)
    _, err = signer.Verify(token, 42, issued.Add(time.Minute))
    require.ErrorIs(t, err, ErrEvaluationContextAPIKeyMismatch)
    _, err = signer.Verify(token, 41, issued.Add(6*time.Minute))
    require.ErrorIs(t, err, ErrEvaluationContextExpired)
}
```

Also cover signature tampering, malformed UUIDs, empty dataset/model/route, keys shorter than 32 bytes, and TTL greater than `Radar.MaxContextTTLSeconds`.

- [ ] **Step 2: Run the signer tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/config -run 'EvaluationContext|RadarConfig'`

Expected: FAIL with undefined signer/config symbols.

- [ ] **Step 3: Implement canonical payload signing**

Use versioned `base64.RawURLEncoding(payloadJSON) + "." + base64.RawURLEncoding(HMAC-SHA256(payloadJSON))`. Reject unknown `version`, accept no clock skew after `ExpiresAt`, allow at most 30 seconds before `IssuedAt`, and compare signatures with `hmac.Equal`.

```go
type EvaluationContext struct {
    Version              int       `json:"v"`
    RunID                string    `json:"run_id"`
    SampleID             string    `json:"sample_id"`
    DatasetVersion       string    `json:"dataset_version"`
    ExpectedModelAlias   string    `json:"expected_model_alias"`
    ExpectedRouteProfile string    `json:"expected_route_profile"`
    APIKeyID             int64     `json:"api_key_id"`
    IssuedAt             time.Time `json:"issued_at"`
    ExpiresAt            time.Time `json:"expires_at"`
    RouteTraceID         string    `json:"-"`
}
```

Add `Radar RadarConfig` to `config.Config`; require 32-byte signing and hashing secrets when enabled, default `MaxContextTTLSeconds=900`, and expose env mappings through the existing Viper loader.

- [ ] **Step 4: Run tests and commit**

Run: `cd backend && go test -tags=unit ./internal/service ./internal/config -run 'EvaluationContext|RadarConfig'`

Expected: PASS.

```bash
git add backend/internal/service/evaluation_context* backend/internal/config backend/internal/pkg/ctxkey/ctxkey.go
git commit -m "feat(radar): sign evaluation request context"
```

### Task 3: Enforce Evaluation Headers at Authentication

**Files:**
- Create: `backend/internal/server/middleware/evaluation_context.go`
- Test: `backend/internal/server/middleware/evaluation_context_test.go`
- Modify: `backend/internal/server/middleware/api_key_auth.go`
- Modify: `backend/internal/server/middleware/api_key_auth_google.go`

**Interfaces:**
- Consumes: `APIKey.IsEvaluation`, `EvaluationContextSigner.Verify`.
- Produces: authenticated `EvaluationContext` in the request context with server-generated `RouteTraceID`.

- [ ] **Step 1: Add the authentication matrix test**

Test these exact outcomes: normal key/no header passes; normal key/evaluation header returns 403 `EVALUATION_CONTEXT_FORBIDDEN`; evaluation key/no token returns 403 `EVALUATION_CONTEXT_REQUIRED`; evaluation key/bad token returns 403 `EVALUATION_CONTEXT_INVALID`; evaluation key/valid token passes and has a nonempty server-generated trace ID.

```go
ctx, ok := service.EvaluationContextFromContext(c.Request.Context())
require.True(t, ok)
require.Equal(t, "run UUID", ctx.RunID)
require.NotEmpty(t, ctx.RouteTraceID)
require.Empty(t, response.Header().Get("X-Sub2API-Route-Trace-ID"))
```

- [ ] **Step 2: Run the middleware test and verify red**

Run: `cd backend && go test -tags=unit ./internal/server/middleware -run EvaluationContext`

Expected: FAIL because the header policy is not implemented.

- [ ] **Step 3: Bind only the signed token**

Recognize only `X-Sub2API-Evaluation-Token`; reject any header whose canonical MIME name starts with `X-Sub2api-Evaluation-` for normal keys. Call the same `bindEvaluationContext` helper in both OpenAI/Anthropic and Google authentication variants immediately before `c.Next()`. Generate `RouteTraceID` with `uuid.NewString()` after verification and never return it to public clients.

- [ ] **Step 4: Run auth regression tests and commit**

Run: `cd backend && go test -tags=unit ./internal/server/middleware`

Expected: PASS.

```bash
git add backend/internal/server/middleware
git commit -m "feat(radar): enforce signed evaluation headers"
```

### Task 4: Collect Redacted Route Attempts

**Files:**
- Create: `backend/internal/service/evaluation_route_evidence.go`
- Test: `backend/internal/service/evaluation_route_evidence_test.go`
- Modify: `backend/internal/service/request_metadata.go`
- Modify: `backend/internal/handler/gateway_handler.go`
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/gemini_v1beta_handler.go`

**Interfaces:**
- Produces: `NewRouteTrace(EvaluationContext, RouteTraceConfig) *RouteTrace`.
- Produces: `RecordAttempt(RouteAttempt)` and `Snapshot() RouteEvidence`.
- Produces: stable `RedactedResourceRef(kind string, id int64, key []byte) string`.

- [ ] **Step 1: Add collector tests**

```go
trace.RecordAttempt(RouteAttempt{Provider: "openai", AccountID: 12, ChannelID: 4, ResolvedModel: "gpt-5.4", Region: "cn-east", ErrorCode: "429"})
trace.RecordAttempt(RouteAttempt{Provider: "openai", AccountID: 13, ChannelID: 4, ResolvedModel: "gpt-5.4", Region: "cn-east"})
got := trace.Snapshot()
require.Equal(t, 2, got.Attempts)
require.Len(t, got.FallbackChain, 2)
require.NotContains(t, string(mustJSON(got)), `"account_id":12`)
require.Equal(t, RedactedResourceRef("account", 12, hashKey), got.FallbackChain[0].AccountPoolRef)
```

Run with `-race` and record attempts concurrently to prove the collector is safe across async usage completion.

- [ ] **Step 2: Run the collector test and verify red**

Run: `cd backend && go test -race -tags=unit ./internal/service -run RouteTrace`

Expected: FAIL with undefined route trace types.

- [ ] **Step 3: Implement and call the collector**

Use a mutex-protected collector pointer stored in context. The fallback entry contains only `ordinal`, `provider`, `account_pool_ref`, `channel_ref`, `resolved_model`, `region`, and `error_code`. Add one call after each successful account selection and one update when a failover reason is known in the three gateway families listed above.

- [ ] **Step 4: Run gateway and race tests, then commit**

Run: `cd backend && go test -race -tags=unit ./internal/service ./internal/handler -run 'RouteTrace|Gateway.*Failover|Gemini.*Failover'`

Expected: PASS.

```bash
git add backend/internal/service/evaluation_route_evidence* backend/internal/service/request_metadata.go backend/internal/handler/gateway_handler.go backend/internal/handler/openai_gateway_handler.go backend/internal/handler/gemini_v1beta_handler.go
git commit -m "feat(radar): collect redacted gateway route attempts"
```

### Task 5: Persist Transport Evidence and Attach Billing

**Files:**
- Create: `backend/internal/repository/evaluation_route_evidence_repo.go`
- Test: `backend/internal/repository/evaluation_route_evidence_repo_test.go`
- Create: `backend/internal/server/middleware/evaluation_evidence.go`
- Test: `backend/internal/server/middleware/evaluation_evidence_test.go`
- Modify: `backend/internal/service/gateway_usage_billing.go`
- Modify: `backend/internal/service/openai_gateway_usage.go`
- Test: `backend/internal/service/gateway_record_usage_test.go`

**Interfaces:**
- Produces: `EvaluationEvidenceRepository.UpsertTransport(ctx context.Context, evidence RouteEvidence) error`.
- Produces: `EvaluationEvidenceRepository.AttachBilling(ctx context.Context, traceID string, usage RouteUsageEvidence) error`.
- Produces: `RouteUsageEvidence{InputTokens, OutputTokens int; TTFT, Latency *int; BilledAmount decimal.Decimal; FinishReason string}`.

- [ ] **Step 1: Write out-of-order idempotency tests**

Test both sequences `UpsertTransport -> AttachBilling` and `AttachBilling -> UpsertTransport`; repeat each call twice and assert one row, unchanged immutable run/sample/key identity, final cost, and full fallback chain. Assert a second write with a different run ID returns `ErrRouteEvidenceIdentityConflict`.

- [ ] **Step 2: Run repository tests and verify red**

Run: `cd backend && go test -tags=unit ./internal/repository -run EvaluationRouteEvidence`

Expected: FAIL because the repository does not exist.

- [ ] **Step 3: Implement two conflict-safe upserts**

Use `INSERT ... ON CONFLICT (route_trace_id) DO UPDATE` with a `WHERE` guard that requires matching `evaluation_run_id`, `sample_id`, and `api_key_id`. If `RowsAffected()==0`, load identity and return the explicit conflict error. Never overwrite a non-null billing field with null.

- [ ] **Step 4: Add response finalization and usage attachment**

The response middleware maps Gin status and trace state to the six allowed `transport_status` values and persists after `c.Next()`. In both usage services, after `usageLog` and cost are finalized, call `AttachBilling` only when `EvaluationContextFromContext(ctx)` succeeds. Evidence failure is logged and counted but must not alter customer billing or response behavior.

- [ ] **Step 5: Run focused and full backend unit tests**

Run: `cd backend && go test -tags=unit ./internal/repository ./internal/server/middleware ./internal/service`

Expected: PASS.

- [ ] **Step 6: Commit the evidence pipeline**

```bash
git add backend/internal/repository/evaluation_route_evidence* backend/internal/server/middleware/evaluation_evidence* backend/internal/service/gateway_usage_billing.go backend/internal/service/openai_gateway_usage.go backend/internal/service/*test.go
git commit -m "feat(radar): persist route and billing evidence"
```

### Task 6: Wire, Migrate, and Verify P0 Isolation

**Files:**
- Modify: `backend/internal/repository/wire.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/server/middleware/wire.go`
- Modify: `backend/internal/server/routes/gateway.go`
- Modify: `backend/cmd/server/wire_gen.go` (generated)
- Test: `backend/internal/server/routes/gateway_test.go`
- Create: `backend/internal/integration/radar_p0_e2e_test.go`
- Create: `docs/model-quality-radar-configuration.md`

**Interfaces:**
- Consumes all P0 interfaces.
- Produces a bootable server with evidence middleware placed after API-key authentication and before gateway handlers.

- [ ] **Step 1: Add a failing route-order integration test**

Issue a signed evaluation request through the real router against a mock upstream. Assert the response succeeds, exactly one evidence row exists, the requested and resolved models differ as expected, token/cost data is attached, and raw account/channel IDs are absent from JSON evidence.

- [ ] **Step 2: Run the test and verify red**

Run: `cd backend && go test -tags=integration ./internal/integration -run RadarP0`

Expected: FAIL because DI and gateway middleware are not wired.

- [ ] **Step 3: Add providers and regenerate Wire**

Register the repository and middleware constructors in the existing provider sets, add the evidence middleware parameter to `RegisterGatewayRoutes`, and apply it immediately after each API-key auth invocation, including Google-compatible groups.

Run: `cd backend && go generate ./cmd/server`

- [ ] **Step 4: Document exact configuration and provisioning**

Document `RADAR_ENABLED`, `RADAR_CONTEXT_SIGNING_KEY`, `RADAR_EVIDENCE_HASH_KEY`, `RADAR_REGION`, `RADAR_ROUTE_PROFILE_VERSION`, and `RADAR_MAX_CONTEXT_TTL_SECONDS`. Include SQL/API steps that create a dedicated user/group/key, set `is_evaluation=true`, and configure user concurrency plus API-key quota/rate limits.

- [ ] **Step 5: Run the P0 verification suite**

Run: `cd backend && go test -tags=unit ./...`

Run: `cd backend && go test -tags=integration ./internal/integration ./internal/repository -run 'RadarP0|MigrationsSchema|Evaluation'`

Run: `cd backend && golangci-lint run ./...`

Expected: all commands PASS.

- [ ] **Step 6: Commit P0**

```bash
git add backend docs/model-quality-radar-configuration.md
git commit -m "feat(radar): complete p0 evaluation isolation"
```

## P0 Acceptance Gate

- A normal key cannot create radar evidence, even with copied headers.
- An evaluation key cannot call inference without a valid, unexpired token bound to that key.
- The database contains one route-evidence row per trace after success, upstream error, protocol error, cancellation, or gateway failure.
- Evidence links run/sample/request, requested/resolved model, route profile, redacted route attempts, region, retry count, tokens, TTFT, latency, and billed amount.
- No evaluation failure changes customer response or billing semantics.
