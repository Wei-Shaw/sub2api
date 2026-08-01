#!/usr/bin/env bash
# Phase 0–3 local end-to-end smoke against warp-gateway mock runtime.
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BIN="${ROOT}/bin/warp-gateway"
DATA="${ROOT}/.e2e-data-$$"
LISTEN="127.0.0.1:19799"
TOKEN="e2e-token"
export WARP_GATEWAY_RUNTIME=mock
export WARP_GATEWAY_DATA_DIR="$DATA"
export WARP_GATEWAY_LISTEN="$LISTEN"
export WARP_GATEWAY_TOKEN="$TOKEN"

cleanup() {
  if [[ -n "${PID:-}" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "$DATA"
}
trap cleanup EXIT

mkdir -p "$DATA" "${ROOT}/bin"
echo "==> build warp-gateway"
(cd "$ROOT" && go build -o "$BIN" ./cmd/warp-gateway)

echo "==> start gateway on $LISTEN"
"$BIN" -listen "$LISTEN" -data-dir "$DATA" -runtime mock -token "$TOKEN" &
PID=$!
sleep 0.4

auth=(-H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")

echo "==> healthz"
curl -fsS "http://${LISTEN}/healthz" | grep -q '"status":"ok"'

echo "==> create pool (Phase 3)"
POOL_JSON=$(curl -fsS -X POST "http://${LISTEN}/v1/pools" "${auth[@]}" \
  -d '{"name_prefix":"e2e","count":3}')
echo "$POOL_JSON" | grep -q '"count":3'

echo "==> snapshot"
SNAP=$(curl -fsS "http://${LISTEN}/v1/pools/snapshot" -H "Authorization: Bearer $TOKEN")
echo "$SNAP" | grep -q 'socks5h://'
echo "$SNAP" | grep -q '"healthy_count"'

echo "==> health all + duplicate IP alerts path"
curl -fsS -X POST "http://${LISTEN}/v1/health/all" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{}' \
  | grep -q 'snapshot'

echo "==> metrics"
curl -fsS "http://${LISTEN}/metrics" -H "Authorization: Bearer $TOKEN" | grep -q warp_instances_total

echo "==> E2E OK (Phase 0–3 control plane mock)"
