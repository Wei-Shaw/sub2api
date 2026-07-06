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

## Access Control And Visibility

The batch image feature has two independent gates:

- Global runtime gate: `BATCH_IMAGE_ENABLED` controls whether `/v1/images/batches*` is available at all. If disabled, the backend returns `404 BATCH_IMAGE_DISABLED` regardless of group settings. This value is loaded at application startup, so changing the server environment requires restarting/redeploying the app container.
- Group/API-key gate: `groups.allow_batch_image_generation` controls whether a user's API key may use the feature. If the global gate is enabled but the API key's group is not allowed, the backend returns `403 BATCH_IMAGE_GROUP_DISABLED`.

Frontend visibility follows the same group/API-key gate for user-facing entry points:

- Sidebar `/batch-image` entry is shown only when the current user has at least one active Gemini API key whose group has `allow_batch_image_generation=true`.
- User dashboard quick action is hidden under the same condition.
- Admin dashboard's shortcut to the user-facing batch image page is also hidden under the same current-user API-key condition; admin group configuration remains available under group management.
- The frontend check pages through active keys in batches of 100 and stops as soon as it finds an allowed key. The result is cached in a shared composable for sidebar/dashboard reuse, and API errors fail closed by hiding the entry.

This frontend hiding is only a UX affordance. Backend authorization remains the source of truth, so direct API calls without an allowed group still fail.

Quick action origin:

- `UserDashboardQuickActions.vue` is an upstream dashboard component. The batch image button was added by the custom batch image work to fit into the existing quick action surface.
- The admin dashboard quick action block and the batch image shortcut inside it were added by the custom batch image work.
- The sidebar batch image module entry was added by the custom batch image work.

## Residual Risks

- Real provider failure combinations should still be tested with controlled fake/fixture provider outputs: malformed output JSONL, missing image bytes, provider cancelled after partial success, and delayed output indexing.
- Concurrent cancel vs settlement still benefits from a dedicated integration test with simultaneous requests to prove row-lock behavior under load, not only unit/static coverage.
- Google/Gemini API-key upstream success was not run because the available test key had no prepayment. The provider was verified as selectable/callable, and failed submit released hold.
- Online high-concurrency stress was intentionally skipped to avoid unnecessary provider cost; Redis per-job locks, database row locks, and billing request idempotency cover the core correctness path in code.

## Recommendation

Proceed to upstream review behind `BATCH_IMAGE_ENABLED` and `allow_batch_image_generation`. Before broad GA, add or run a dedicated cancel/settle concurrency integration test and a paid one-image API-key upstream success test with a properly prepaid Google key.
