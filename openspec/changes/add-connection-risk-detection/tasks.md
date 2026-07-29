# Connection Risk Detection — Implementation Tasks

## Status: Backend A1–C + Frontend A2 implemented

### Done

- [x] PR-01 Config + cached settings + flag precedence
- [x] PR-02 Redis signal cache + emitter (always-on / evidence)
- [x] PR-03 Full gateway surface middleware mount
- [x] PR-04 `connection_risk_events` migration + raw SQL repo
- [x] PR-05 Rules R1–R7 pure functions (+ R3 baseline Phase B)
- [x] PR-06 Worker + LeaderLock + policy + metrics + wire
- [x] PR-07 R7 session binding hooks (jwt + admin)
- [x] PR-08 Admin REST API
- [x] PR-09 Frontend `/admin/connection-risk` UI + sidebar + i18n
- [x] PR-10 Soft throttle + AdminUpdate whitelist + baseline snapshots
- [x] PR-11 Auto-disable flags + retention cleanup
- [x] miniredis unit tests for signal cache sliding window
- [x] Unit tests for settings, rules, AdminUpdate, policy helpers

### Operator enablement checklist

1. Apply migration `192_connection_risk_events.sql` (embedded; runs on deploy)
2. Set YAML `connection_risk.enabled: true`
3. Admin UI → 异常连接 → enable `enabled` + later `emit_enabled`
4. Confirm Worker via Runtime tab (`worker_ticks` increasing)
5. Keep `auto_disable_enabled` false until validated

### Optional follow-ups (non-blocking)

- [ ] Email notify on high+ events (settings.notify_email hook)
- [ ] OpsCleanupService overlay for retention instead of worker tick
- [ ] Richer frontend charts / geo map
