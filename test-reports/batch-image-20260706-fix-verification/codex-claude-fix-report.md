# Batch Image Fix Verification Report

Date: 2026-07-06
Branch: feature/batch-image-foundation

## Scope

This pass fixes the remaining QA findings from the batch image reports:

- Add a bounded settlement billing retry path for `SETTLEMENT_BILLING_FAILED`.
- Prevent jobs from staying in `settling` with frozen balance forever after repeated billing failures.
- Clarify cancel billing copy: only images indexed as successful are billed, and the remaining hold is released.
- Ask Claude Code to re-review the fix after Codex validation.

## Codex Changes

- `BatchImageSettlementService` now uses `batchImageSettlementMaxRetries = 5`.
- `SetBatchImageJobSettlementFailed` atomically increments and returns `retry_count` with `RETURNING retry_count`.
- When capture billing fails and reaches the retry limit, settlement releases the frozen hold and transitions the job to `failed` with `SETTLEMENT_BILLING_RETRY_EXHAUSTED`.
- The worker pipeline re-reads the job after settlement billing errors and acknowledges terminal jobs instead of requeueing forever.
- A transition-failure regression test verifies that release retry is idempotent when release succeeds but the failed-state transition fails.
- User-facing cancel copy and the copyable skill instructions now say indexed successful images are billed and the remaining hold is released.

## Codex Verification

| Check | Result |
|---|---|
| `go test -tags unit ./internal/service -run 'BatchImage(Settlement\|Pipeline\|Public\|Processor\|BillingRecovery)' -count=1 -timeout=8m` | Pass |
| `pnpm --dir frontend typecheck` | Pass |
| `pnpm --dir frontend build` | Pass, with existing Vite chunk/Browserslist warnings |
| `go test -tags integration ./internal/repository -run '^TestBatchImageRepository_SetBatchImageJobSettlementFailed$'` | Compiled; skipped inside Docker because Docker socket is unavailable to testcontainers |

## Claude Code Verification

Claude Code model used: `sonnet --safe-mode --effort low`.

First review result:

- No P1/P2 blocker found in the bounded retry and cancel-copy fix.
- Flagged one residual risk: if release succeeds but transition to `failed` fails, the next run could call release again; requested confirmation of idempotency.

Codex follow-up:

- Added `TestBatchImageSettlementRetryExhaustedReleaseIsIdempotentAfterTransitionFailure`.
- Confirmed the real `UsageBillingRepository` calls `claimUsageBillingRequest` before `ReleaseBatchImageBalance`; duplicate `BatchImageReleaseRequestID(batchID)` returns `Applied:false`.

Second Claude review result:

- Confirmed the new test closes the prior risk.
- No remaining P1/P2 issue.
- Remaining P3: repository integration should be run in an environment with Docker socket/testcontainers available.

## Residual Risk

- Repository integration was not fully executed in the Docker-based Go test container because testcontainers could not access Docker. The SQL change is small and compiled, but should be run once in an environment where repository integration tests can start containers.
