# Channel Monitor V2 Safe Defaults & Gentle Backfill

**Date:** 2026-08-08
**Status:** Approved for implementation
**Branch:** `fix/channel-monitor-v2-ops-ui-blockers` (onto channel-monitor-v2)

## Problem

1. **H2** — Migration 195 and code defaults set `channel_monitor_mode=v2`, so every upgrade without an existing key auto-opts into passive V2, stops V1 probes, and may leave users on an empty V2 view.
2. **M1** — First-enable bootstrap forces 5s ticks and ~24h `RecomputeRange` chunks until `now-30d`, hammering the primary DB.
3. Error dedup SQL matches `request_id` without a `created_at` bound, scanning `ops_error_logs` history via `candidate_ids`.

## Decisions

| Topic | Decision |
|-------|----------|
| Default mode | **v1** (keep active probes). V2 is explicit opt-in. |
| Existing `channel_monitor_mode=v2` rows | **Unchanged** (`ON CONFLICT DO NOTHING` / no force rewrite). |
| Switch to v2 | V1 runner `fire()` and `RunCheck` require `ActiveProbesAllowed()` → probes stop. |
| Backfill | Unified hard gates + adaptive soft gates (same rules, different observed load). |
| Optional profile knob | **Not in this change** (YAGNI). |

## Mode defaults

- Migration `195_channel_monitor_mode.sql`: insert `'v1'` when key missing.
- If 195 already applied with old checksum, add migration checksum compatibility (do not force rewrite applied DBs to v1).
- Code: `defaultChannelMonitorMode = v1`; empty/invalid normalize → v1.
- Frontend Settings form default and public feature flag fallback → v1 when missing/invalid.
- Nil settings on V1 `RunCheck` path remains **fail-closed** (no probes) for test safety — independent of product default.

## Gentle backfill (low-resource phases)

### Hard gates (all servers)

- Tick interval = `refresh_interval` (60 or 300s). **No 5s bootstrap override.**
- Each tick: (1) recent overlap (~10m), then (2) **at most one** historical chunk while product bootstrap incomplete / retention walk ongoing.
- Single leader lock; single short transaction budget (~55s).
- Chunk ceiling by depth (product phases 90m → 1d → 7d → 30d → silent 90d).

### Soft gates (adaptive)

- Initial historical chunk: **1h** (first seed still **2h** for 90m UI).
- Min chunk: **15m**; max by depth: ≤1d → **2h**, ≤7d → **4h**, older → **6h**.
- Success + fast: grow slowly (×1.5) up to depth max.
- Failure/timeout: halve chunk; exponential backoff on wait (cap 10m).
- Progressive UI: show whatever is covered; bootstrap banner vs 30d product window.

### Error dedup

- Bound the `request_id IN candidate_ids` branch with
  `created_at >= $1 - INTERVAL '90 minutes' AND created_at < $2`.

## Out of scope

- Force existing v2 → v1.
- Separate backfill workers / read replicas.
- Admin `backfill_profile` setting (may add later).
