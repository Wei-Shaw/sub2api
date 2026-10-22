#!/usr/bin/env bash
# self-heal-watch.sh — Sub2API self-healing monitor for Docker + NAS environments
# Usage: ./self-heal-watch.sh [OPTIONS]
#   -h HOST        NAS SSH host (default: 192.168.10.20)
#   -u USER        SSH user (default: l890852)
#   -k KEY         SSH private key (default: ~/.ssh/id_ed25519_nas)
#   -d COMPOSE_DIR Docker compose directory on remote (default: /volume1/docker/sub2api)
#   -p PORT        Web UI port (default: 3014)
#   -P PROXY_URL   Proxy URL for API calls (e.g. http://192.168.10.20:7890)
#   -i INTERVAL    Check interval in seconds (default: 3600)
#   --no-restart   Only alert, do not restart containers
#   --dry-run      Print actions without executing

set -euo pipefail

HOST="${SELF_HEAL_HOST:-192.168.10.20}"
USER="${SELF_HEAL_USER:-l890852}"
KEY="${SELF_HEAL_KEY:-$HOME/.ssh/id_ed25519_nas}"
COMPOSE_DIR="${SELF_HEAL_COMPOSE_DIR:-/volume1/docker/sub2api}"
PORT="${SELF_HEAL_PORT:-3014}"
PROXY_URL="${SELF_HEAL_PROXY_URL:-}"
INTERVAL="${SELF_HEAL_INTERVAL:-3600}"
DRY_RUN=false
NO_RESTART=false

while [[ $# -gt 0 ]]; do
  case $1 in
    -h) HOST="$2"; shift 2 ;;
    -u) USER="$2"; shift 2 ;;
    -k) KEY="$2"; shift 2 ;;
    -d) COMPOSE_DIR="$2"; shift 2 ;;
    -p) PORT="$2"; shift 2 ;;
    -P) PROXY_URL="$2"; shift 2 ;;
    -i) INTERVAL="$2"; shift 2 ;;
    --no-restart) NO_RESTART=true; shift ;;
    --dry-run) DRY_RUN=true; shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

SSH="ssh -i "$KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10"
SSH_CMD="$USER@$HOST"
WEB_URL="http://${HOST}:${PORT}"

ERROR_PATTERNS=(
  "no available accounts"
  "unsupported_country_region_territory"
  "account_select_failed"
  "token_refresh.retry_attempt_failed"
  "OAuth refresh attempt timed out"
  "account OAuth credentials permanently rejected"
  "proxyconnect tcp: dial tcp 127.0.0.1:7890"
)

log() { echo "[$(date '+%Y-%m-%dT%H:%M:%S%z')] $*"; }

check_http() {
  local url=$1
  local proxy_arg=()
  [[ -n "$PROXY_URL" ]] && proxy_arg=(-x "$PROXY_URL")
  curl -s -o /dev/null -w "%{http_code}" --max-time 10 "${proxy_arg[@]}" "$url" 2>/dev/null || echo "000"
}

restart_containers() {
  log "Restarting sub2api containers..."
  if $DRY_RUN; then
    log "[DRY-RUN] Would run: cd $COMPOSE_DIR && sudo docker compose restart"
    return 0
  fi
  $SSH "$SSH_CMD" "cd $COMPOSE_DIR && sudo docker compose restart" 2>/dev/null
  sleep 5
}

heal() {
  log "Attempting self-heal..."
  restart_containers
  sleep 10
  local http_code; http_code=$(check_http "${WEB_URL}/health")
  if [[ "$http_code" == "200" ]]; then
    log "Heal successful — /health returned 200"
    return 0
  else
    log "Heal failed — /health still returning $http_code"
    return 1
  fi
}

run_check() {
  log "=== Starting sub2api health check ==="

  # 1. Container status
  local container_status; container_status=$($SSH "$SSH_CMD" \
    "cd $COMPOSE_DIR && sudo docker compose ps --format json 2>/dev/null" 2>/dev/null || echo "")

  if [[ -z "$container_status" ]]; then
    log "WARN: Could not get container status from $COMPOSE_DIR"
  else
    local unhealthy; unhealthy=$(echo "$container_status" | \
      jq -r 'select(.State != "running" and .State != "Up") | .Name' 2>/dev/null || echo "")
    if [[ -n "$unhealthy" ]]; then
      log "WARN: Unhealthy containers: $unhealthy"
    else
      log "Containers: OK"
    fi
  fi

  # 2. HTTP health checks
  local base_code; base_code=$(check_http "$WEB_URL/")
  local health_code; health_code=$(check_http "${WEB_URL}/health")
  local models_code; models_code=$(check_http "${WEB_URL}/v1/models")

  log "/ -> $base_code | /health -> $health_code | /v1/models -> $models_code"

  # 3. Log scan for error patterns
  local log_errors=""
  for pattern in "${ERROR_PATTERNS[@]}"; do
    local found; found=$($SSH "$SSH_CMD" \
      "cd $COMPOSE_DIR && sudo docker compose logs --tail=100 2>/dev/null | grep -i '$pattern' | tail -3" 2>/dev/null || true)
    if [[ -n "$found" ]]; then
      log_errors+="  [$pattern] found in logs: $(echo "$found" | tr '\n' '|')\n"
    fi
  done

  if [[ -n "$log_errors" ]]; then
    log "ERROR patterns detected:\n$log_errors"
  else
    log "Logs: clean, no error patterns"
  fi

  # 4. Proxy env check
  if [[ -n "$PROXY_URL" ]]; then
    local proxy_check; proxy_check=$($SSH "$SSH_CMD" \
      "cd $COMPOSE_DIR && sudo docker compose exec -T sub2api env | grep -E '^(HTTP_PROXY|HTTPS_PROXY|ALL_PROXY|UPDATE_PROXY_URL)=' 2>/dev/null" || true)
    if echo "$proxy_check" | grep -qv "$HOST"; then
      log "WARN: Proxy env may still point to localhost instead of $HOST"
    else
      log "Proxy env: OK ($PROXY_URL)"
    fi
  fi

  # 5. Decide heal
  local needs_heal=false
  if [[ "$health_code" != "200" ]]; then
    needs_heal=true
    log "Reason to heal: /health returned $health_code"
  fi
  if [[ -n "$log_errors" ]]; then
    needs_heal=true
    log "Reason to heal: error patterns in logs"
  fi

  if $needs_heal; then
    if $NO_RESTART; then
      log "Alert: sub2api needs attention (no-restart mode, not healing)"
      return 1
    fi
    heal && return 0 || return 1
  fi

  log "Result: HEALTHY"
  return 0
}

# Run once or loop
if [[ "${SELF_HEAL_ONCE:-false}" == "true" ]]; then
  run_check
else
  while true; do
    run_check || true
    log "Sleeping ${INTERVAL}s before next check..."
    sleep "$INTERVAL"
  done
fi
