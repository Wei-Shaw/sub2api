# Codex Batch Image QA Report

Date: 2026-07-06
Tester: Codex
Baseline commits:

- `8fab636 feat: complete batch image workflow`
- `5553d83 fix: localize antigravity image mapping labels`

## Summary

No blocking issue remains from the Codex-run checks. One frontend regression was found during testing: Antigravity image mapping preset labels displayed English `passthrough` while the existing UI/test expectation used Chinese `透传`. It was fixed in `5553d83`, and the full frontend suite then passed.

## Commands Run

| Area | Command | Result |
|---|---|---|
| Backend service tests | Docker Go 1.26.4: `go test ./internal/service -run "BatchImage|AdminService_.*BatchImage|GroupBatchImage|PricingService.*Batch|UsageBilling" -count=1 -timeout=10m` | Pass |
| Backend repository tests | Docker Go 1.26.4: `go test ./internal/repository -run "BatchImage|UsageBilling|Migrations" -count=1 -timeout=10m` | Pass |
| Backend server tests | Docker Go 1.26.4: `go test ./internal/server/... -run "APIContract|BatchImage|APIKey" -count=1 -timeout=10m` | Pass |
| Frontend typecheck | `pnpm --dir frontend typecheck` | Pass |
| Frontend build | `pnpm --dir frontend build` | Pass |
| Frontend full tests | `pnpm --dir frontend test:run` | Pass: 128 files, 803 tests |
| Local HTTP smoke | See `smoke-summary.txt` | Pass |

## HTTP Smoke Result

Source: `smoke-summary.txt`

| Check | Result |
|---|---|
| Unauthorized batch list | `401 API_KEY_REQUIRED` |
| Model list | `200`, 2 models: `gemini-2.5-flash-image`, `gemini-3.1-flash-image` |
| Insufficient balance submit | `402 BATCH_IMAGE_INSUFFICIENT_BALANCE` |
| Completed batch detail | `200`, status `completed`, success `2`, fail `0`, actual cost `0.134` |
| Completed items | `200`, item count `2` |
| Completed download | `200 application/zip`, 1,602,237 bytes |
| Balance restoration after smoke | Original `1.86600000 / 0.00000000`; final `1.86600000 / 0.00000000` |

## Findings

| Severity | Finding | Status |
|---|---|---|
| P2 | Antigravity batch edit image mapping labels were mixed English/Chinese and failed existing UI expectation. | Fixed in `5553d83`; full frontend tests pass. |
| P3 | Frontend test output contains existing Vue/i18n warnings (`router-link`, `el-tooltip`, localstorage-file, Browserslist stale data). | Non-blocking; suite passes. |

## Billing And Exception Coverage

Covered by automated tests and smoke:

- Balance reserve moves available funds to frozen funds.
- Insufficient balance returns 402 before provider submission.
- Capture rejects actual cost greater than hold.
- Capture below hold releases the remainder.
- Stale pre-provider jobs can be failed and released.
- Completed job download only returns successful outputs.

## Residual Risks

- Real provider failure combinations should still be tested with controlled fake/fixture provider outputs: malformed output JSONL, missing image bytes, provider cancelled after partial success, and delayed output indexing.
- Concurrent cancel vs settlement needs a dedicated integration test with simultaneous requests to prove row-lock behavior under load, not only unit/static coverage.
- Settlement billing failure retry currently needs a clearer bounded retry or operator handoff story; Claude independently flagged this too.

## Recommendation

Proceed to broader review with Claude and/or manual exploratory testing. Before production enablement, add one integration test for cancel/settle concurrency and one for persistent settlement billing failure recovery.

