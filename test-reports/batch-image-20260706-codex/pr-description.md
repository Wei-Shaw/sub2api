# PR Description Draft: Batch Image Generation MVP

## Summary

This PR adds an opt-in batch image generation MVP for Gemini image models through Sub2API.

Main capabilities:

- Public async batch image API under `/v1/images/batches*`.
- Provider support for Vertex-managed Gemini batch jobs and Gemini API batch jobs.
- Upstream account support is limited to Gemini `service_account` accounts for the Vertex provider and Gemini `apikey` accounts for the Gemini API provider.
- Redis-backed worker queue, delayed requeue, stale active recovery, and per-job locks.
- PostgreSQL job/item state, provider refs kept internal, and proxied item/ZIP downloads.
- Balance hold, capture, release, partial-failure settlement, and idempotent billing request ids.
- Frontend user batch image guide and gated navigation entry.
- Feature gates through global `BATCH_IMAGE_ENABLED`, Gemini-only group eligibility, image-generation enablement, and group-level `allow_batch_image_generation`.

The feature is intentionally not GA by default. It should be enabled first through feature flag and group opt-in only.

## Docs Included

- `docs/BATCH_IMAGE_MVP.md`: API, lifecycle, billing, provider notes, config, official Google enablement, and operations checklist.
- `test-reports/batch-image-20260706-codex/test-case.md`: QA case matrix.
- `test-reports/batch-image-20260706-codex/codex-report.md`: Codex test report.
- `test-reports/batch-image-20260706-codex/claude-report.md`: Claude Code review report plus 2026-07-07 follow-up addendum.
- `test-reports/batch-image-20260706-codex/smoke-summary.txt`: local HTTP smoke result.

## Validation

Automated/local validation recorded in the test reports:

- Backend batch image service/repository/server tests: pass.
- Frontend typecheck/build/full tests: pass.
- Local HTTP smoke: unauthenticated access, model listing, insufficient balance, completed status/items/download, and balance restoration.
- Settlement tests cover successful-image-only charging, zero-success completion, already-settled idempotency, billing crash idempotency, cost-over-hold rejection, pricing snapshot, bounded settlement retry, retry exhaustion release, and billing request ids.

Online validation recorded on 2026-07-07:

- One-image Vertex success: hold `0.0804`, actual `0.0737`, release `0.0067`, final `frozen_balance=0`.
- Immediate cancel after submit: hold released, charged `0`, no capture usage log.
- Two-item partial failure: one success, one failure, charged one image only, `errors.json` contains failed item, final `frozen_balance=0`.
- Gemini API-key provider path: provider selectable/callable; test key had no prepayment, so successful generation was not continued; failed submit released hold and charged `0`.

## Remaining Non-Blocking Gaps

- No high-concurrency online stress test was run because it would create unnecessary provider cost and production pressure.
- Gemini API-key upstream success still needs one paid/prepaid low-cost image test when such a key is available.
- Other Gemini login/account types were not tested and are not selected by the current providers unless they can expose equivalent service-account or API-key credentials through the same provider flow.
- A future integration test can exercise simultaneous cancel vs settlement under load, although Redis per-job locks, PostgreSQL row locks, and billing idempotency are already present.
- Optional object-storage download offload could be added later: store completed outputs in GCS/S3/R2 and issue short-lived signed links so large image/ZIP downloads do not consume Sub2API server bandwidth. This should remain opt-in because it adds storage credentials, lifecycle cleanup, signed-link expiry, and access-audit requirements.

## Rollout Recommendation

Merge/review behind flags only:

- Keep `BATCH_IMAGE_ENABLED=false` by default.
- Enable only for selected Gemini groups after `allow_image_generation=true`, then set `allow_batch_image_generation=true`; non-Gemini groups are intentionally not eligible for this switch.
- Start with one controlled group and monitor job state, provider errors, hold/capture/release events, and download volume before broader enablement.
