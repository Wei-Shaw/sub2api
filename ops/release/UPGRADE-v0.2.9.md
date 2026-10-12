# BUAGA v0.2.9 upgrade

Baseline: production `1dbbef7c391a4c6932d02f5509fb531d5a58f84f`, verified live on port 18207 with seven-day video previews and DeepSeek native cache usage.

Merge stable upstream v0.2.9 (`4c00df2e0183e2c70b7fa8ba45914205e36aad0c`). Preserve the custom model-list display editor rather than restoring the upstream allowlist UI. Keep canonical-model-aware transport failure handling while accepting client cancellation detection. Keep obsolete upstream Compose files deleted; the live containers and their configuration are inherited from the verified custom runtime.

Keep the existing unset image-token pricing semantics and their regression tests. The upstream change to inherit image prices when a channel leaves them empty is intentionally excluded to avoid silently changing existing customer charges. Explicit prices, group/user multipliers, long-context and peak/off-peak settings are not edited. Accept the upstream account-statistics long-context correction and pass the new argument through the custom fixed-quantity video cost snapshot (zero tokens; existing video prices unchanged).

No SQL migration files change. Before starting the candidate, verify the full migration directory matches the live release, take an online database backup, and capture hashes of pricing, groups, multipliers and settings. Compare those hashes before switching and after deployment; never restore the production database for an application rollback.

Release only through a reviewed production tag and `package.py` / `git archive`. Required validation: all 416 customization guards and guard tests, backend build/full unit and focused race, frontend typecheck/lint/full unit/build. Deploy on 18208, retain 18207 for in-flight requests and rollback, publish hashed frontend assets additively, gracefully reload Nginx and rebind the price monitor with authenticated read validation. Sample health across the switch.

Operational evidence belongs in `/home/serveradmin/releases/buaga-v029-custom-20260928`; the workspace deployment report records the final results and release identifiers.

The broad focused race run exposed existing parallel tests concurrently calling Gin's global `SetMode` in unchanged bridge-test files. Run this selection with `-parallel=1` to isolate test-global state while retaining the race detector and concurrency inside each test; retain both logs. No application race was inferred from that test-harness collision.
