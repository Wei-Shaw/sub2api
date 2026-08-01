#!/usr/bin/env bash
# Switch local e2e stack from mock → real Cloudflare WARP (sing-box + wgcf profiles).
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)"
WG_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORKDIR="${ROOT}/.local-warp-e2e"
PROFDIR="${WORKDIR}/warp-profiles"
API_PORT=18080
WG_PORT=19798
WG_TOKEN="warp-e2e-token"
ADMIN_EMAIL="admin@warp-e2e.local"
ADMIN_PASSWORD="WarpE2ePass123!"
export PATH="${HOME}/.local/bin:${HOME}/go/bin:${PATH}"

log() { echo -e "\n==> $*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

command -v sing-box >/dev/null || die "sing-box not found (install to ~/.local/bin)"
command -v wgcf >/dev/null || die "wgcf not found (install to ~/.local/bin)"
[[ -x "${WORKDIR}/sub2api" ]] || die "missing ${WORKDIR}/sub2api — run local-full-e2e.sh first"
[[ -f "${WORKDIR}/config.yaml" ]] || die "missing ${WORKDIR}/config.yaml"

mkdir -p "$PROFDIR" "$WORKDIR/logs" "$WORKDIR/warp-gateway-data-real"

log "1) ensure 3 wgcf WARP profiles"
for i in 1 2 3; do
  d="${PROFDIR}/acc${i}"
  mkdir -p "$d"
  if [[ ! -f "${d}/wgcf-profile.conf" ]]; then
    (
      cd "$d"
      if [[ ! -f wgcf-account.toml ]]; then
        printf 'y\n' | wgcf register 2>/dev/null || yes | wgcf register || true
      fi
      wgcf generate
    )
  fi
  [[ -f "${d}/wgcf-profile.conf" ]] || die "failed to generate profile for acc${i}"
done

log "2) build warp-gateway"
(cd "$WG_ROOT" && GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" go build -o bin/warp-gateway ./cmd/warp-gateway)

log "3) stop previous warp-gateway (mock or real)"
# Prefer pid file; fall back to port.
if [[ -f "${WORKDIR}/wg.pid" ]]; then
  kill "$(cat "${WORKDIR}/wg.pid")" 2>/dev/null || true
  rm -f "${WORKDIR}/wg.pid"
fi
# Do not kill self via broad pkill patterns that match this script.
for pid in $(ss -lntp 2>/dev/null | awk '/:19798/ {print}' | grep -oP 'pid=\K[0-9]+' || true); do
  kill "$pid" 2>/dev/null || true
done
sleep 0.5

log "4) start warp-gateway (sing-box runtime)"
# Fresh instance store for real WARP (keep mock data intact under warp-gateway-data/)
export WARP_GATEWAY_PROBE_URL="${WARP_GATEWAY_PROBE_URL:-https://1.1.1.1/cdn-cgi/trace}"
export WARP_GATEWAY_SING_BOX="${WARP_GATEWAY_SING_BOX:-$(command -v sing-box)}"
nohup "$WG_ROOT/bin/warp-gateway" \
  -listen "127.0.0.1:${WG_PORT}" \
  -data-dir "$WORKDIR/warp-gateway-data-real" \
  -runtime sing-box \
  -token "$WG_TOKEN" \
  >"$WORKDIR/logs/warp-gateway-real.log" 2>&1 &
echo $! >"$WORKDIR/wg.pid"
sleep 0.8
curl -fsS "http://127.0.0.1:${WG_PORT}/healthz" | grep -q ok || {
  tail -50 "$WORKDIR/logs/warp-gateway-real.log" || true
  die "warp-gateway healthz failed"
}

log "5) create 3 real WARP instances from wgcf profiles"
ROOT_DIR="$ROOT" python3 - <<'PY'
import json, re, urllib.request, pathlib, os
root = pathlib.Path(os.environ["ROOT_DIR"])
prof = root / ".local-warp-e2e" / "warp-profiles"
token = "warp-e2e-token"
base = "http://127.0.0.1:19798"

def parse_wgcf(path):
    text = path.read_text()
    def grab(k):
        m = re.search(rf"^{k}\s*=\s*(.+)$", text, re.M)
        return m.group(1).strip() if m else ""
    addrs = [a.strip() for a in grab("Address").split(",") if a.strip()]
    dns = [a.strip() for a in grab("DNS").split(",") if a.strip()]
    mtu = int(grab("MTU") or "1280")
    return {
        "private_key": grab("PrivateKey"),
        "address": addrs,
        "dns": dns[:2] or ["1.1.1.1"],
        "mtu": mtu,
        "peers": [{
            "public_key": grab("PublicKey"),
            "endpoint": grab("Endpoint") or "engage.cloudflareclient.com:2408",
            "allowed_ips": [a.strip() for a in grab("AllowedIPs").split(",") if a.strip()] or ["0.0.0.0/0", "::/0"],
        }],
    }

def api(method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        base + path, data=data, method=method,
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)

# delete existing instances
cur = api("GET", "/v1/instances")
for inst in cur.get("instances") or []:
    try:
        api("DELETE", f"/v1/instances/{inst['id']}")
        print("deleted", inst.get("name"), inst["id"])
    except Exception as e:
        print("delete fail", inst.get("id"), e)

created = []
for i in range(1, 4):
    profile = parse_wgcf(prof / f"acc{i}" / "wgcf-profile.conf")
    assert profile["private_key"], f"missing key acc{i}"
    body = {
        "name": f"real-{i:02d}",
        "listen_port": 41000 + i,  # 41001-41003
        "profile": profile,
        "auto_start": True,
    }
    inst = api("POST", "/v1/instances", body)
    print("created", inst.get("name"), inst.get("status"), inst.get("listen_port"), inst.get("last_error", ""))
    created.append(inst)

# health all
import time
time.sleep(3)
health = api("POST", "/v1/health/all", {})
print(json.dumps(health, indent=2)[:2000])
snap = api("GET", "/v1/pools/snapshot")
print("snapshot instances:", len(snap.get("instances") or snap.get("running") or []))
for inst in (api("GET", "/v1/instances").get("instances") or []):
    print(f"  {inst['name']} status={inst['status']} exit={inst.get('exit_ip')} colo={inst.get('exit_colo')} err={inst.get('last_error')}")
PY

log "6) SOCKS smoke (expect warp=on)"
for port in 41001 41002 41003; do
  out=$(curl -sS --connect-timeout 10 --max-time 15 --socks5-hostname "127.0.0.1:${port}" \
    https://1.1.1.1/cdn-cgi/trace || true)
  echo "--- port ${port} ---"
  echo "$out" | head -15
  echo "$out" | grep -q 'warp=on' || echo "WARN: port ${port} not warp=on yet"
done

log "7) login sub2api + sync proxies"
LOGIN=$(curl -sf -X POST "http://127.0.0.1:${API_PORT}/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")
AT=$(printf '%s' "$LOGIN" | python3 -c 'import sys,json; print(json.load(sys.stdin)["data"]["access_token"])')
SYNC=$(curl -sf -X POST "http://127.0.0.1:${API_PORT}/api/v1/admin/warp/sync" \
  -H "Authorization: Bearer $AT" -H 'Content-Type: application/json' \
  -d '{"group_name":"warp-pool"}')
echo "$SYNC" | python3 -m json.tool | head -80

HS=$(curl -sf -X POST "http://127.0.0.1:${API_PORT}/api/v1/admin/warp/health-sync" \
  -H "Authorization: Bearer $AT" -H 'Content-Type: application/json' -d '{}')
echo "$HS" | python3 -c '
import sys,json
d=json.load(sys.stdin)["data"]
snap=d.get("snapshot") or {}
for i in snap.get("instances") or []:
  print(i.get("name"), i.get("status"), i.get("exit_ip"), i.get("exit_colo"), i.get("last_error",""))
'

log "REAL_WARP_OK"
echo "UI: http://127.0.0.1:${API_PORT}/admin/warp"
echo "gateway log: ${WORKDIR}/logs/warp-gateway-real.log"
echo "profiles: ${PROFDIR}/acc{1,2,3}/wgcf-profile.conf"
