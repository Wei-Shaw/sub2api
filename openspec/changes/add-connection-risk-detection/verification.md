# Connection Risk — Verification

## Build

```bash
cd backend && go build -o /dev/null ./cmd/server/
```

## Backend tests

```bash
cd backend
go test ./internal/service/ -run 'ConnectionRisk|Percentile|FlagPrecedence|GetConnectionRisk|HashUserAgent|Severity|MaskAPIKey|ClearEvent' -count=1
go test -tags=unit ./internal/repository/ -run 'ConnectionSignal' -count=1
go test -tags=unit ./internal/service/ -run 'AdminUpdate' -count=1
go test ./internal/pkg/ip/ ./internal/server/middleware/ ./internal/server/routes/ ./cmd/server/ -count=1 -timeout 60s
```

## Frontend tests

```bash
cd frontend
pnpm exec vitest run src/features/connection-risk
```

## Manual smoke (after enable)

1. `GET /api/v1/admin/connection-risk/config`
2. `PUT` config with `enabled=true`, `emit_enabled=true` (YAML master must be on)
3. Generate multi-IP traffic on one API key
4. `GET /api/v1/admin/connection-risk/events?status=open`
5. Ack / resolve / whitelist from UI

## Live E2E (2026-07-29, sub2api-dev @ 127.0.0.1:18080)

Environment: hot-swapped newly built binary into `sub2api-dev`; migration `connection_risk_events` applied on restart; YAML `connection_risk.enabled=true`.

| Step | Result |
|------|--------|
| Health | `{"status":"ok"}` |
| Admin login | OK |
| `GET/PUT /admin/connection-risk/config` | 200; enabled+emit+worker |
| `GET /admin/connection-risk/runtime` | `yaml_enabled=true`, flags effective |
| Create API key + top-up balance + clear `apikey:auth:*` cache | Required (auth cache kept balance=0 until flush) |
| 22× `/v1/models` with 11 distinct client IPs + 5 UAs | 22/22 HTTP 200 |
| Emit metrics | `emit_ok=23`, `emit_error=0` |
| Redis `cr:*` | 14 keys (active, HLL, uas:1h ZSET, evidence, owner, prefix…) |
| Worker | ticks advanced; produced **1 open event** |
| Event content | severity=`medium`, score=`45`, rules=`R1,R2,R3_abs`, key prefix masked |
| Ack → Resolve | status `acknowledged` → `resolved` (200) |

### Notes from E2E

1. **API key auth cache** must be invalidated after direct SQL balance updates, or gateway keeps returning `INSUFFICIENT_BALANCE` and never reaches emit middleware.
2. Balance check runs **inside** `apiKeyAuth` **before** `postAuthSignal`; failed auth ⇒ no signals (by design).
3. Worker first tick is delayed (~5s start + interval); with `worker_interval_seconds=60`, event appeared within ~1 min after traffic.
