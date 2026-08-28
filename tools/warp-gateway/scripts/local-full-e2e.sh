#!/usr/bin/env bash
# Full local deploy + integration of warp-gateway (mock) + sub2api + postgres + redis.
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)"
WG_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORKDIR="${ROOT}/.local-warp-e2e"
PG_PORT=15432
REDIS_PORT=16379
API_PORT=18080
WG_PORT=19798
ADMIN_EMAIL="admin@warp-e2e.local"
ADMIN_PASSWORD="WarpE2ePass123!"
JWT_SECRET="$(openssl rand -hex 32)"
TOTP_KEY="$(openssl rand -hex 32)"
WG_TOKEN="warp-e2e-token"
WG_PROFILE_KEY="$(openssl rand -hex 32)"
PG_PASS="warp_e2e_pg"
CONTAINER_PG="sub2api-warp-e2e-pg"
CONTAINER_REDIS="sub2api-warp-e2e-redis"

log() { echo -e "\n==> $*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

cleanup() {
  set +e
  if [[ -n "${API_PID:-}" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null
    wait "$API_PID" 2>/dev/null
  fi
  if [[ -n "${WG_PID:-}" ]] && kill -0 "$WG_PID" 2>/dev/null; then
    kill "$WG_PID" 2>/dev/null
    wait "$WG_PID" 2>/dev/null
  fi
  # leave containers running for inspection unless CLEANUP_ALL=1
  if [[ "${CLEANUP_ALL:-0}" == "1" ]]; then
    docker rm -f "$CONTAINER_PG" "$CONTAINER_REDIS" 2>/dev/null
    rm -rf "$WORKDIR"
  fi
}
trap cleanup EXIT

mkdir -p "$WORKDIR/data" "$WORKDIR/logs" "$WG_ROOT/bin"

log "1) PostgreSQL + Redis (docker)"
if ! docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_PG"; then
  docker run -d --name "$CONTAINER_PG" \
    -e POSTGRES_USER=sub2api \
    -e POSTGRES_PASSWORD="$PG_PASS" \
    -e POSTGRES_DB=sub2api \
    -p "127.0.0.1:${PG_PORT}:5432" \
    postgres:16-alpine >/dev/null
else
  docker start "$CONTAINER_PG" >/dev/null
fi
if ! docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_REDIS"; then
  docker run -d --name "$CONTAINER_REDIS" \
    -p "127.0.0.1:${REDIS_PORT}:6379" \
    redis:7-alpine >/dev/null
else
  docker start "$CONTAINER_REDIS" >/dev/null
fi

log "wait for postgres"
for i in $(seq 1 60); do
  if docker exec "$CONTAINER_PG" pg_isready -U sub2api >/dev/null 2>&1; then
    break
  fi
  sleep 1
  [[ $i -eq 60 ]] && die "postgres not ready"
done
log "wait for redis"
for i in $(seq 1 30); do
  if docker exec "$CONTAINER_REDIS" redis-cli ping 2>/dev/null | grep -q PONG; then
    break
  fi
  sleep 1
  [[ $i -eq 30 ]] && die "redis not ready"
done

log "2) build warp-gateway"
(cd "$WG_ROOT" && GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" go build -o bin/warp-gateway ./cmd/warp-gateway)

log "3) start warp-gateway (mock)"
export WARP_GATEWAY_PROFILE_KEY="$WG_PROFILE_KEY"
"$WG_ROOT/bin/warp-gateway" \
  -listen "127.0.0.1:${WG_PORT}" \
  -data-dir "$WORKDIR/warp-gateway-data" \
  -runtime mock \
  -token "$WG_TOKEN" \
  >"$WORKDIR/logs/warp-gateway.log" 2>&1 &
WG_PID=$!
for i in $(seq 1 30); do
  if curl -fsS -m 2 "http://127.0.0.1:${WG_PORT}/healthz" 2>/dev/null | grep -q ok; then
    break
  fi
  if ! kill -0 "$WG_PID" 2>/dev/null; then
    echo "--- warp-gateway log ---"
    cat "$WORKDIR/logs/warp-gateway.log" || true
    die "warp-gateway process died"
  fi
  sleep 1
  if [[ $i -eq 30 ]]; then
    cat "$WORKDIR/logs/warp-gateway.log" || true
    die "warp-gateway healthz timeout"
  fi
done

log "4) write sub2api config"
# Remove prior install marker for clean AUTO_SETUP if present
rm -f "$WORKDIR/data/.installed" 2>/dev/null || true

cat >"$WORKDIR/config.yaml" <<EOF
server:
  host: "127.0.0.1"
  port: ${API_PORT}
  mode: "release"
database:
  host: "127.0.0.1"
  port: ${PG_PORT}
  user: "sub2api"
  password: "${PG_PASS}"
  dbname: "sub2api"
  sslmode: "disable"
redis:
  host: "127.0.0.1"
  port: ${REDIS_PORT}
  password: ""
  db: 0
jwt:
  secret: "${JWT_SECRET}"
  expire_hour: 24
totp:
  encryption_key: "${TOTP_KEY}"
default:
  user_concurrency: 5
timezone: "Asia/Shanghai"
log:
  level: "info"
  format: "console"
  env: "local-e2e"
  output:
    to_stdout: true
    to_file: true
    file_path: "${WORKDIR}/logs/sub2api.log"
warp:
  enabled: true
  gateway:
    base_url: "http://127.0.0.1:${WG_PORT}"
    token: "${WG_TOKEN}"
    timeout_ms: 5000
    reconcile_interval_sec: 30
  auto_detach_unhealthy: true
  alert_duplicate_exit_ip: true
  default_group_name: "warp-pool"
EOF

log "5) build sub2api binary"
(cd "$ROOT/backend" && GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" go build -o "$WORKDIR/sub2api" ./cmd/server)

log "6) start sub2api with AUTO_SETUP"
export AUTO_SETUP=true
export ADMIN_EMAIL="$ADMIN_EMAIL"
export ADMIN_PASSWORD="$ADMIN_PASSWORD"
export JWT_SECRET="$JWT_SECRET"
export DATABASE_HOST=127.0.0.1
export DATABASE_PORT=$PG_PORT
export DATABASE_USER=sub2api
export DATABASE_PASSWORD="$PG_PASS"
export DATABASE_DBNAME=sub2api
export DATABASE_SSLMODE=disable
export REDIS_HOST=127.0.0.1
export REDIS_PORT=$REDIS_PORT
export SERVER_HOST=127.0.0.1
export SERVER_PORT=$API_PORT
export TOTP_ENCRYPTION_KEY="$TOTP_KEY"
export DATA_DIR="$WORKDIR/data"
export CONFIG_FILE="$WORKDIR/data/config.yaml"
# Warp via env (works even if AUTO_SETUP rewrites yaml incompletely)
export WARP_ENABLED=true
export WARP_GATEWAY_BASE_URL="http://127.0.0.1:${WG_PORT}"
export WARP_GATEWAY_TOKEN="$WG_TOKEN"
export WARP_GATEWAY_TIMEOUT_MS=5000
export WARP_GATEWAY_RECONCILE_INTERVAL_SEC=30
export WARP_AUTO_DETACH_UNHEALTHY=true
export WARP_ALERT_DUPLICATE_EXIT_IP=true
export WARP_DEFAULT_GROUP_NAME=warp-pool

mkdir -p "$WORKDIR/data"
# CRITICAL: do NOT place config.yaml before AUTO_SETUP — NeedsSetup() is false when config exists.
rm -f "$WORKDIR/data/config.yaml" "$WORKDIR/data/.installed"

cd "$WORKDIR"
./sub2api >"$WORKDIR/logs/sub2api.stdout" 2>&1 &
API_PID=$!
cd "$ROOT"

log "wait for sub2api /health"
for i in $(seq 1 90); do
  if curl -fsS -m 2 "http://127.0.0.1:${API_PORT}/health" 2>/dev/null | grep -q ok; then
    echo "sub2api ready (${i}s)"
    break
  fi
  if ! kill -0 "$API_PID" 2>/dev/null; then
    echo "--- sub2api log ---"
    tail -80 "$WORKDIR/logs/sub2api.stdout" || true
    die "sub2api process died"
  fi
  sleep 1
  [[ $i -eq 90 ]] && { tail -100 "$WORKDIR/logs/sub2api.stdout"; die "sub2api health timeout"; }
done

# Merge warp section into generated config (AUTO_SETUP writes config without warp)
if [[ -f "$WORKDIR/data/config.yaml" ]] && ! grep -q '^warp:' "$WORKDIR/data/config.yaml"; then
  log "append warp config + restart so worker picks up enabled=true"
  cat >>"$WORKDIR/data/config.yaml" <<EOF

warp:
  enabled: true
  gateway:
    base_url: "http://127.0.0.1:${WG_PORT}"
    token: "${WG_TOKEN}"
    timeout_ms: 5000
    reconcile_interval_sec: 30
  auto_detach_unhealthy: true
  alert_duplicate_exit_ip: true
  default_group_name: "warp-pool"
EOF
  kill "$API_PID" 2>/dev/null || true
  wait "$API_PID" 2>/dev/null || true
  cd "$WORKDIR"
  # already installed; do not re-run AUTO_SETUP
  unset AUTO_SETUP
  export AUTO_SETUP=false
  ./sub2api >"$WORKDIR/logs/sub2api.stdout" 2>&1 &
  API_PID=$!
  cd "$ROOT"
  for i in $(seq 1 60); do
    curl -fsS -m 2 "http://127.0.0.1:${API_PORT}/health" 2>/dev/null | grep -q ok && break
    sleep 1
    [[ $i -eq 60 ]] && die "restart health timeout"
  done
fi

log "7) admin login"
LOGIN_JSON=$(curl -fsS -X POST "http://127.0.0.1:${API_PORT}/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}" || true)
# envelope: {code, data:{access_token}}
TOKEN=$(python3 - <<PY
import json,sys
raw='''$LOGIN_JSON'''
try:
  o=json.loads(raw)
except Exception as e:
  print('', end='')
  sys.exit(0)
data=o.get('data') or o
print(data.get('access_token') or data.get('token') or '', end='')
PY
)
if [[ -z "$TOKEN" ]]; then
  echo "login response: $LOGIN_JSON"
  # try create via setup install if needed
  die "admin login failed"
fi
AUTH=(-H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")

log "7b) accept admin compliance (required for admin APIs)"
curl -fsS -X POST "http://127.0.0.1:${API_PORT}/api/v1/admin/compliance/accept" "${AUTH[@]}" \
  -d '{"phrase":"I have read, understood, and agree to the Sub2API Deployment and Operation Compliance Commitment","language":"en"}' \
  | tee "$WORKDIR/logs/compliance.json" >/dev/null

log "8) warp status"
curl -fsS "http://127.0.0.1:${API_PORT}/api/v1/admin/warp/status" -H "Authorization: Bearer $TOKEN" | tee "$WORKDIR/logs/warp-status.json"
python3 - <<PY
import json
o=json.load(open("$WORKDIR/logs/warp-status.json"))
d=o.get("data") or o
assert d.get("enabled") is True, d
print("warp enabled OK")
PY

log "9) create pool + auto sync to DB"
curl -fsS -X POST "http://127.0.0.1:${API_PORT}/api/v1/admin/warp/pools" "${AUTH[@]}" \
  -d '{"name_prefix":"e2e","count":3,"group_name":"warp-pool"}' | tee "$WORKDIR/logs/warp-pool.json"
python3 - <<PY
import json
o=json.load(open("$WORKDIR/logs/warp-pool.json"))
d=o.get("data") or o
created=d.get("created_proxies") or []
members=d.get("member_ids") or []
group=d.get("group") or {}
group_name=group.get("name") or group.get("Name")
group_id=group.get("id") or group.get("ID")
print(f"created={len(created)} members={len(members)} group={group_name} id={group_id}")
assert len(created) >= 1 or len(members) >= 1, d
assert group_name == "warp-pool" or group_id, d
print("pool sync OK")
PY

log "10) list proxies (should include warp-e2e-*)"
curl -fsS "http://127.0.0.1:${API_PORT}/api/v1/admin/proxies?page=1&page_size=50&search=warp" "${AUTH[@]}" \
  | tee "$WORKDIR/logs/proxies.json" >/dev/null
python3 - <<PY
import json
o=json.load(open("$WORKDIR/logs/proxies.json"))
d=o.get("data") or o
items=d.get("items") or d.get("Items") or d
if isinstance(items, dict):
  items=items.get("items") or []
print("proxy items sample:", len(items) if isinstance(items, list) else type(items))
if isinstance(items, list):
  names=[x.get("name") or x.get("Name") or "" for x in items]
  print("names:", names[:10])
  assert any("warp" in (n or "").lower() or "e2e" in (n or "").lower() for n in names), names
print("proxies OK")
PY

log "11) health-sync + rotate one instance"
INST_ID=$(python3 - <<PY
import json
o=json.load(open("$WORKDIR/logs/warp-pool.json"))
d=o.get("data") or o
snap=d.get("snapshot") or {}
insts=snap.get("instances") or []
print(insts[0]["id"] if insts else "", end="")
PY
)
if [[ -n "$INST_ID" ]]; then
  curl -fsS -X POST "http://127.0.0.1:${API_PORT}/api/v1/admin/warp/instances/${INST_ID}/rotate" "${AUTH[@]}" \
    -d '{"group_name":"warp-pool"}' | tee "$WORKDIR/logs/warp-rotate.json" >/dev/null
  echo "rotate OK for $INST_ID"
fi
curl -fsS -X POST "http://127.0.0.1:${API_PORT}/api/v1/admin/warp/health-sync" "${AUTH[@]}" \
  -d '{"group_name":"warp-pool"}' | tee "$WORKDIR/logs/warp-health-sync.json" >/dev/null
echo "health-sync OK"

log "12) gateway snapshot still healthy"
curl -fsS "http://127.0.0.1:${WG_PORT}/v1/pools/snapshot" \
  -H "Authorization: Bearer ${WG_TOKEN}" | tee "$WORKDIR/logs/gw-snapshot.json" >/dev/null
python3 - <<PY
import json
o=json.load(open("$WORKDIR/logs/gw-snapshot.json"))
print("gateway total", o.get("total_count"), "healthy", o.get("healthy_count"))
assert o.get("total_count",0) >= 3
print("gateway snapshot OK")
PY

log "FULL E2E PASSED"
echo "----------------------------------------"
echo "sub2api:        http://127.0.0.1:${API_PORT}"
echo "warp-gateway:   http://127.0.0.1:${WG_PORT}"
echo "admin:          ${ADMIN_EMAIL} / ${ADMIN_PASSWORD}"
echo "workdir:        ${WORKDIR}"
echo "logs:           ${WORKDIR}/logs/"
echo "containers:     ${CONTAINER_PG}, ${CONTAINER_REDIS}"
echo "To tear down:   CLEANUP_ALL=1 $0  (or docker rm -f ${CONTAINER_PG} ${CONTAINER_REDIS})"
echo "----------------------------------------"
# keep services running for manual inspection unless CLEANUP_ALL=1
if [[ "${CLEANUP_ALL:-0}" != "1" ]]; then
  trap - EXIT
  echo "Services left running (API_PID=$API_PID WG_PID=$WG_PID)."
fi
