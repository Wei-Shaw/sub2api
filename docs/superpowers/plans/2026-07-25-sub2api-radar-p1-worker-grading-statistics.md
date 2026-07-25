# sub2api Radar P1 Worker, Grading, and Statistics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver controlled Python Workers, independent graders, and statistically defensible regression analysis for Coding/SWE, reasoning, tool, protocol, safety, performance, and cost domains.

**Architecture:** One versioned Python image runs in explicit `runner`, `grader`, or `statistics` mode. Runners lease cases, call `sub2api` through the signed evaluation path, and upload evidence but never scores. Graders lease evidence, execute deterministic or isolated Coding verification, and submit versioned scores. The statistics mode consumes paired baseline/candidate scores, computes Bootstrap confidence intervals plus EWMA/CUSUM signals, and submits immutable aggregate snapshots.

**Tech Stack:** Python 3.12, asyncio, httpx, Pydantic 2, JSON Schema, NumPy, SciPy, pytest, Docker/Kubernetes Jobs, Go 1.26.5 control-plane extensions, PostgreSQL.

## Global Constraints

- Requires completion of both P0 route evidence and P1 control-plane plans.
- Runner and grader identities are separate Worker records and credentials.
- Runner output is evidence only; it cannot set pass/fail, score, failure class, or aggregate status.
- Do not store hidden chain-of-thought. Persist final output, tool calls, protocol events, verifier logs, hashes, and bounded explanations only.
- Coding verification runs with no host mounts outside its assignment directory, read-only root filesystem, no network, non-root UID, fixed CPU/memory/PID/time limits, and an image pinned by digest.
- Capability errors count toward model quality; upstream, Worker, Judge, and infrastructure failures do not.
- Baseline and candidate use the same case, parameters, region, time window, and sample index.
- Bootstrap uses 10,000 paired resamples and a 95% percentile interval with a recorded seed.

---

## File Structure

- `backend/migrations/192_add_radar_grading_statistics.sql`: grading jobs, versioned scores, aggregate snapshots, and review queue.
- `backend/internal/service/evaluation_grading.go`: Go grading lease/submission contracts.
- `backend/internal/repository/evaluation_grading_repo.go`: fenced grader jobs and immutable score versions.
- `backend/internal/handler/internal/radar_grader_handler.go`: grader/statistics endpoints.
- `radar-worker/pyproject.toml`: pinned Python package and test dependencies.
- `radar-worker/src/sub2api_radar/config.py`: environment validation.
- `radar-worker/src/sub2api_radar/models.py`: Pydantic wire contracts.
- `radar-worker/src/sub2api_radar/control_plane.py`: Worker/grader HTTP client.
- `radar-worker/src/sub2api_radar/runner.py`: lease/heartbeat/execution/upload loop.
- `radar-worker/src/sub2api_radar/executors/`: OpenAI, Anthropic, Gemini, SSE, and adapter execution.
- `radar-worker/src/sub2api_radar/graders/`: exact, protocol, tool, Coding, Judge, and safety graders.
- `radar-worker/src/sub2api_radar/statistics/`: pairing, Bootstrap, EWMA, CUSUM, and classification.
- `radar-worker/src/sub2api_radar/security/redaction.py`: artifact secret/PII/internal-domain scanning.
- `radar-worker/tests/`: unit, contract, and fixture tests.
- `radar-worker/Dockerfile`, `radar-worker/deploy/kubernetes/*.yaml`: controlled runtime.

### Task 1: Add Grading and Aggregate Persistence Contracts

**Files:**
- Create: `backend/migrations/192_add_radar_grading_statistics.sql`
- Create: `backend/internal/service/evaluation_grading.go`
- Create: `backend/internal/repository/evaluation_grading_repo.go`
- Test: `backend/internal/repository/evaluation_grading_repo_integration_test.go`
- Create: `backend/internal/handler/internal/radar_grader_handler.go`
- Test: `backend/internal/handler/internal/radar_grader_handler_test.go`
- Modify: `backend/internal/server/routes/radar_worker.go`

**Interfaces:**
- Produces grader lease endpoints `/internal/radar/v1/grading-leases:claim`, `/:id/heartbeat`, `/:id/complete`, and `/:id/fail`.
- Produces statistics endpoints `/internal/radar/v1/analysis-jobs:claim` and `/:id/complete`.
- Produces immutable `ScoreSubmission` and `AggregateSubmission` contracts.

- [ ] **Step 1: Write failing fencing and version tests**

Prove runner tokens cannot claim grading work, a stale grader lease cannot submit, repeated submission returns the same score ID, regrading creates a new version and clears the prior `is_current`, and aggregate submission rejects score IDs from another run.

```go
type ScoreSubmission struct {
    SampleID       uuid.UUID
    GraderID       string
    GraderVersion  string
    Score          decimal.Decimal
    Passed         *bool
    FailureClass   FailureClass
    FailureCode    string
    Explanation    string
    EvidenceHashes []string
}
```

- [ ] **Step 2: Run backend tests and verify red**

Run: `cd backend && go test -tags=integration ./internal/repository -run EvaluationGrading && go test -tags=unit ./internal/handler/internal -run RadarGrader`

Expected: FAIL because migration and grading contracts are absent.

- [ ] **Step 3: Implement versioned score and job tables**

Create `evaluation_grading_jobs`, monthly range-partitioned `evaluation_scores`, `evaluation_score_heads`, `evaluation_analysis_jobs`, monthly range-partitioned `evaluation_aggregate_snapshots`, and `evaluation_manual_reviews`. Enforce score range `0..1`, one current pointer per `(sample_id, grader_id)` through `evaluation_score_heads`, a unique submission idempotency key, and aggregate uniqueness `(run_id, capability_domain, model_route, window, analysis_version, window_start)`. The migration creates the current and next two monthly partitions; the daily scheduler creates the next partition before month end.

- [ ] **Step 4: Implement fenced claims and server-side validation**

Use the same random token hashing and lease-expiry guards as assignment leases. On score submit, load the sample, route evidence, case grader ID/version, and evidence hashes in one transaction. Reject mismatches; set sample/assignment `completed` only after an accepted score. Judge disagreement creates a manual-review row and leaves the sample out of aggregates.

- [ ] **Step 5: Run tests and commit**

Run: `cd backend && go test -tags=unit ./internal/handler/internal -run RadarGrader && go test -tags=integration ./internal/repository -run EvaluationGrading`

Expected: PASS.

```bash
git add backend/migrations/192_add_radar_grading_statistics.sql backend/internal/service/evaluation_grading.go backend/internal/repository/evaluation_grading_repo* backend/internal/handler/internal/radar_grader_handler* backend/internal/server/routes/radar_worker.go
git commit -m "feat(radar): add independent grading contracts"
```

### Task 2: Scaffold a Strict Worker Package and Control-Plane Client

**Files:**
- Create: `radar-worker/pyproject.toml`
- Create: `radar-worker/src/sub2api_radar/__init__.py`
- Create: `radar-worker/src/sub2api_radar/config.py`
- Create: `radar-worker/src/sub2api_radar/models.py`
- Create: `radar-worker/src/sub2api_radar/control_plane.py`
- Create: `radar-worker/tests/test_config.py`
- Create: `radar-worker/tests/test_control_plane.py`

**Interfaces:**
- Produces: `Settings.from_env() -> Settings`.
- Produces: async `ControlPlaneClient.claim_assignment`, `heartbeat`, `submit_evidence`, `complete_assignment`, `fail_assignment`, and equivalent grading/analysis methods.

- [ ] **Step 1: Create the package manifest and failing tests**

Pin runtime dependencies to compatible minor ranges and expose console scripts `radar-runner`, `radar-grader`, and `radar-statistics`. Test missing tokens, non-HTTPS URLs outside `localhost`, lease DTO unknown fields, request timeouts, 409 fencing, 429 backoff, and identical idempotency keys across retries.

```toml
[project]
requires-python = ">=3.12,<3.13"
dependencies = [
  "httpx>=0.28,<0.29", "pydantic>=2.11,<3", "pydantic-settings>=2.9,<3",
  "jsonschema>=4.24,<5", "numpy>=2.2,<3", "scipy>=1.15,<2", "tenacity>=9.1,<10"
]
[project.optional-dependencies]
dev = ["pytest>=8.4,<9", "pytest-asyncio>=1.0,<2", "respx>=0.22,<0.23", "ruff>=0.12,<0.13", "mypy>=1.16,<2"]
```

- [ ] **Step 2: Run tests and verify red**

Run: `cd radar-worker && python3.12 -m pip install -e '.[dev]' && pytest tests/test_config.py tests/test_control_plane.py -q`

Expected: FAIL because modules are not implemented.

- [ ] **Step 3: Implement strict contracts and retry policy**

All Pydantic models use `ConfigDict(extra="forbid", frozen=True)`. Retry only connection errors, 408, 429, and 5xx; honor `Retry-After`; never retry 400/401/403/409. Add `Idempotency-Key` to evidence, completion, and failure calls using the assignment/job ID plus action.

- [ ] **Step 4: Run static and unit checks, then commit**

Run: `cd radar-worker && pytest tests/test_config.py tests/test_control_plane.py -q && ruff check . && mypy src`

Expected: PASS.

```bash
git add radar-worker
git commit -m "feat(radar-worker): add strict control plane client"
```

### Task 3: Implement Lease, Heartbeat, and Crash-Safe Runner Loop

**Files:**
- Create: `radar-worker/src/sub2api_radar/runner.py`
- Create: `radar-worker/src/sub2api_radar/state.py`
- Create: `radar-worker/tests/test_runner.py`
- Create: `radar-worker/tests/test_state.py`

**Interfaces:**
- Produces: `Runner.run_forever(stop: asyncio.Event) -> None`.
- Produces: `Runner.execute_lease(lease: AssignmentLease) -> None`.
- Produces local write-ahead states `claimed`, `executing`, `evidence_ready`, `evidence_accepted`, and `terminal`.

- [ ] **Step 1: Write runner failure-mode tests**

Cover cancellation, heartbeat failure, lease fencing during execution, process restart after local evidence is written, duplicate evidence receipt, upload timeout, and maximum one active execution per configured slot. Use a fake monotonic clock; do not sleep in tests.

- [ ] **Step 2: Run tests and verify red**

Run: `cd radar-worker && pytest tests/test_runner.py tests/test_state.py -q`

Expected: FAIL because runner/state modules do not exist.

- [ ] **Step 3: Implement the crash-safe loop**

Write each local state atomically as JSON to `<state-dir>/<assignment-id>.json` using a temporary file plus `os.replace`. Heartbeat every `min(lease_ttl/3, 30s)` while executing. If fenced, terminate child execution and retain local evidence for diagnostics without uploading. On restart, resend `evidence_ready` with the original idempotency key before claiming new work.

- [ ] **Step 4: Verify and commit**

Run: `cd radar-worker && pytest tests/test_runner.py tests/test_state.py -q && ruff check . && mypy src`

Expected: PASS.

```bash
git add radar-worker/src/sub2api_radar/runner.py radar-worker/src/sub2api_radar/state.py radar-worker/tests
git commit -m "feat(radar-worker): add crash-safe lease runner"
```

### Task 4: Execute Gateway Protocols and Produce Reproducible Evidence

**Files:**
- Create: `radar-worker/src/sub2api_radar/executors/base.py`
- Create: `radar-worker/src/sub2api_radar/executors/openai.py`
- Create: `radar-worker/src/sub2api_radar/executors/anthropic.py`
- Create: `radar-worker/src/sub2api_radar/executors/gemini.py`
- Create: `radar-worker/src/sub2api_radar/executors/sse.py`
- Create: `radar-worker/src/sub2api_radar/executors/adapters.py`
- Create: `radar-worker/tests/fixtures/protocol/`
- Create: `radar-worker/tests/test_executors.py`

**Interfaces:**
- Produces: `Executor.execute(case: CaseSpec, lease: AssignmentLease) -> ExecutionEvidence`.
- Produces: exact event records with monotonic offsets, status, response headers allowlist, final output, tool calls, usage, hashes, and route trace linkage.

- [ ] **Step 1: Add protocol fixture tests**

Test OpenAI Responses and Chat Completions, Anthropic Messages, Gemini generateContent, valid SSE, invalid UTF-8, missing `[DONE]`, duplicated terminal event, truncated JSON, tool-call argument fragments, upstream 429, and a 200 response with malformed protocol.

- [ ] **Step 2: Run tests and verify red**

Run: `cd radar-worker && pytest tests/test_executors.py -q`

Expected: FAIL because executors are absent.

- [ ] **Step 3: Implement bounded execution**

Send the lease-provided gateway token only as `X-Sub2API-Evaluation-Token`; send the evaluation API key through the protocol's normal auth header. Enforce case timeout, maximum response bytes, and maximum SSE event count. Hash the exact request body and raw response bytes before parsing. Store only headers `content-type`, `x-request-id`, `openai-request-id`, `request-id`, `retry-after`, and `server-timing`.

- [ ] **Step 4: Implement external-tool adapters**

Run Promptfoo, EvalScope, lm-eval-harness/OpenCompass, and Garak only through version-pinned adapter images using argument arrays, never `shell=True`. Ship content-addressed adapter fixtures for MMLU, GSM8K, HumanEval, IFEval, and a minimal Garak jailbreak set. Convert tool JSON output to `ExecutionEvidence`; reject output whose adapter version, dataset hash, or model route differs from the lease.

- [ ] **Step 5: Verify and commit**

Run: `cd radar-worker && pytest tests/test_executors.py -q && ruff check . && mypy src`

Expected: PASS.

```bash
git add radar-worker/src/sub2api_radar/executors radar-worker/tests
git commit -m "feat(radar-worker): execute model protocol cases"
```

### Task 5: Add Deterministic, Coding, Judge, and Safety Graders

**Files:**
- Create: `radar-worker/src/sub2api_radar/graders/base.py`
- Create: `radar-worker/src/sub2api_radar/graders/exact.py`
- Create: `radar-worker/src/sub2api_radar/graders/protocol.py`
- Create: `radar-worker/src/sub2api_radar/graders/tool_call.py`
- Create: `radar-worker/src/sub2api_radar/graders/coding.py`
- Create: `radar-worker/src/sub2api_radar/graders/judge.py`
- Create: `radar-worker/src/sub2api_radar/graders/safety.py`
- Create: `radar-worker/src/sub2api_radar/grader.py`
- Create: `radar-worker/src/sub2api_radar/security/redaction.py`
- Create: `radar-worker/tests/test_graders.py`
- Create: `radar-worker/tests/test_redaction.py`

**Interfaces:**
- Produces: `GradeResult(score: Decimal, passed: bool|None, failure_class: FailureClass, failure_code: str, explanation: str, evidence_hashes: tuple[str,...])`.
- Produces: `ArtifactScanner.scan(paths: Sequence[Path]) -> ScanReport`.

- [ ] **Step 1: Write grader and scanner tests**

Cover numeric normalization, Unicode/whitespace normalization, JSON Schema, ordered/unordered tool calls, protocol event invariants, compile/test pass/fail, Docker timeout/OOM/network attempt, swapped Judge answer order, Judge disagreement, jailbreak success, over-refusal, API keys, JWTs, private keys, email/phone patterns, and configured internal domains.

- [ ] **Step 2: Run tests and verify red**

Run: `cd radar-worker && pytest tests/test_graders.py tests/test_redaction.py -q`

Expected: FAIL because graders/scanner are absent.

- [ ] **Step 3: Implement deterministic graders and failure taxonomy**

Return `capability` only when a complete, parseable model output violates the expected answer or deterministic contract. Map 429/timeouts/disconnects to `upstream`, Docker/Worker faults to `infrastructure`, malformed gateway protocol to `protocol`, grader timeout to `judge`, and missing/mismatched hashes to `invalid_evidence`.

- [ ] **Step 4: Implement isolated Coding verification**

Invoke Docker with explicit arguments equivalent to: read-only root, `--network none`, `--cpus`, `--memory`, `--pids-limit`, `--user 65532:65532`, `--cap-drop ALL`, `--security-opt no-new-privileges`, tmpfs `/tmp`, assignment directory mounted read-only, and a writable scratch volume. Verify the image digest before launch and capture bounded stdout/stderr plus exit/OOM/timeout metadata.

- [ ] **Step 5: Implement blinded Judge and safety review**

Randomize A/B order with a recorded seed and remove model/provider/route labels. Run the configured odd number of Judge models; require a configurable agreement ratio, default `2/3`. On disagreement return `passed=None`, `failure_class='judge'`, and request manual review rather than contributing a score.

- [ ] **Step 6: Scan before upload and verify**

Block artifact confirmation when high-confidence secrets or configured internal domains are found. Redact lower-risk PII spans and include only rule IDs/counts in the scan report.

Run: `cd radar-worker && pytest tests/test_graders.py tests/test_redaction.py -q && ruff check . && mypy src`

Expected: PASS.

- [ ] **Step 7: Commit graders**

```bash
git add radar-worker/src/sub2api_radar/graders radar-worker/src/sub2api_radar/grader.py radar-worker/src/sub2api_radar/security radar-worker/tests
git commit -m "feat(radar-worker): grade evidence independently"
```

### Task 6: Compute Paired Regression Statistics and Failure Classification

**Files:**
- Create: `radar-worker/src/sub2api_radar/statistics/pairing.py`
- Create: `radar-worker/src/sub2api_radar/statistics/bootstrap.py`
- Create: `radar-worker/src/sub2api_radar/statistics/ewma.py`
- Create: `radar-worker/src/sub2api_radar/statistics/cusum.py`
- Create: `radar-worker/src/sub2api_radar/statistics/classification.py`
- Create: `radar-worker/src/sub2api_radar/statistics/service.py`
- Create: `radar-worker/tests/test_statistics.py`

**Interfaces:**
- Produces: `analyze(pairs: Sequence[PairedScore], history: Sequence[AggregatePoint], policy: AnalysisPolicy) -> AggregateAnalysis`.
- Produces results with weighted baseline/candidate score, delta in percentage points, 95% CI, effective pair count, invalid counts by class, EWMA, CUSUM, and evidence sufficiency.

- [ ] **Step 1: Add fixed-seed golden tests**

Test no regression, exact -2 and -3 point boundaries, a CI crossing zero, missing pairs, capability failures, upstream failures, Judge disagreement, unequal weights, repeated samples, EWMA small drift, CUSUM sustained drift, and deterministic output for seed `20260725`.

- [ ] **Step 2: Run tests and verify red**

Run: `cd radar-worker && pytest tests/test_statistics.py -q`

Expected: FAIL because statistics modules do not exist.

- [ ] **Step 3: Implement pairing and weighted Bootstrap**

Pair on `(case_id, model_route, sample_index)` and discard pairs where either side lacks a current valid score. Resample pairs with replacement 10,000 times using `numpy.random.Generator(PCG64(seed))`; calculate the weighted candidate-minus-baseline delta for each draw and the 2.5/97.5 percentiles. Report `insufficient_evidence` when effective pairs are below policy minimum or CI width exceeds policy maximum.

- [ ] **Step 4: Implement EWMA, CUSUM, and classification**

EWMA uses configurable `lambda`, default `0.2`. Two-sided CUSUM uses configurable drift `k=0.5` and decision threshold `h=5.0` in standardized units. Classification never converts upstream/infrastructure/Judge/invalid-evidence failures into zero capability scores; it reports them as separate rates.

- [ ] **Step 5: Verify and commit**

Run: `cd radar-worker && pytest tests/test_statistics.py -q && ruff check . && mypy src`

Expected: PASS.

```bash
git add radar-worker/src/sub2api_radar/statistics radar-worker/tests/test_statistics.py
git commit -m "feat(radar-worker): detect paired quality regressions"
```

### Task 7: Package Controlled Deployments and Run Known-Good/Known-Bad E2E

**Files:**
- Create: `radar-worker/Dockerfile`
- Create: `radar-worker/deploy/kubernetes/runner-deployment.yaml`
- Create: `radar-worker/deploy/kubernetes/grader-deployment.yaml`
- Create: `radar-worker/deploy/kubernetes/statistics-cronjob.yaml`
- Create: `radar-worker/deploy/kubernetes/network-policy.yaml`
- Create: `radar-worker/tests/e2e/test_known_regression.py`
- Modify: `.github/workflows/backend-ci.yml`
- Create: `.github/workflows/radar-worker-ci.yml`
- Modify: `docs/model-quality-radar-configuration.md`

**Interfaces:**
- Produces immutable Worker images supporting `runner`, `grader`, and `statistics` commands.
- Produces an E2E proof that injected model/protocol/performance regressions are classified correctly.

- [ ] **Step 1: Write the E2E fixture**

Start a fixed-response upstream with baseline and candidate modes. Candidate mode answers one reasoning case incorrectly, emits one invalid tool argument, truncates one SSE stream, delays one response beyond the P99 threshold, and returns one 429. Assert only the reasoning error counts as capability, tool/SSE as protocol, delay as reliability/performance, and 429 as upstream.

- [ ] **Step 2: Run E2E and verify red**

Run: `cd radar-worker && pytest tests/e2e/test_known_regression.py -q`

Expected: FAIL until images and full loop are wired.

- [ ] **Step 3: Build hardened images and manifests**

Use a multi-stage build, non-root UID 65532, read-only application files, digest-pinned base image, `PYTHONDONTWRITEBYTECODE=1`, health endpoints, termination grace long enough to fail/release a lease, resource requests/limits, topology spread, and NetworkPolicy allowing only control-plane, sub2api gateway, DNS, and approved Judge endpoints.

- [ ] **Step 4: Add CI gates**

Run Python unit tests, Ruff, Mypy, `python -m pip_audit`, the image build, and `docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:0.65.0 image --exit-code 1 --severity HIGH,CRITICAL sub2api-radar-worker:${GITHUB_SHA}` plus the Go grading contract tests. Add `pip-audit>=2.9,<3` to the dev dependency group. Do not download benchmark datasets in unit CI; use content-addressed fixtures.

- [ ] **Step 5: Run full verification**

Run: `cd radar-worker && pytest -q && ruff check . && mypy src`

Run: `cd backend && go test -tags=unit ./...`

Run: `cd backend && go test -tags=integration ./internal/repository ./internal/integration -run 'EvaluationGrading|Radar'`

Expected: all commands PASS.

- [ ] **Step 6: Commit Worker and statistics delivery**

```bash
git add radar-worker .github/workflows docs/model-quality-radar-configuration.md backend
git commit -m "feat(radar): deliver controlled workers and statistics"
```

## Worker and Statistics Acceptance Gate

- Killing a runner, grader, or statistics process cannot duplicate a current score or aggregate.
- A runner cannot submit scores; a grader cannot grade mismatched or unscanned evidence.
- Coding execution is isolated, resource-bounded, digest-pinned, and network-disabled.
- Capability, protocol, upstream, Worker, Judge, infrastructure, and invalid-evidence outcomes remain separate.
- Fixed-seed Bootstrap output is reproducible, and evidence-insufficient cases never claim regression.
- The known-good/known-bad E2E fixture detects and correctly classifies every injected regression.
