#!/usr/bin/env bash
set -Eeuo pipefail

systemctl is-active --quiet x-ui
if systemctl is-active --quiet haproxy; then
    echo 'HAProxy is already active; refusing to repeat the switch' >&2
    exit 1
fi
haproxy -c -f /etc/haproxy/haproxy.cfg

backup="$(mktemp /root/x-ui-before-astrogate-443.XXXXXX.db)"
chmod 600 "$backup"
backup_ready=0

rollback() {
    trap - ERR
    if [[ "$backup_ready" != 1 ]]; then
        echo '443 switch stopped before changing the listener' >&2
        return
    fi
    systemctl stop haproxy || true
    systemctl disable haproxy >/dev/null 2>&1 || true
    systemctl stop x-ui || true
    if [[ "$backup_ready" == 1 ]]; then
        cp "$backup" /etc/x-ui/x-ui.db
    fi
    systemctl start x-ui || true
    echo "443 switch failed; restored the x-ui database from $backup" >&2
}
trap rollback ERR

python3 - "$backup" <<'PY'
import sqlite3
import sys

source = sqlite3.connect('/etc/x-ui/x-ui.db', timeout=15)
destination = sqlite3.connect(sys.argv[1])
source.backup(destination)
destination.close()

row = source.execute('SELECT id, protocol FROM inbounds WHERE id = 3 AND port = 443').fetchone()
if row != (3, 'vless'):
    raise SystemExit('Expected x-ui VLESS inbound 3 on port 443; refusing to switch')
source.execute('UPDATE inbounds SET port = 1443 WHERE id = 3 AND port = 443')
source.commit()
source.close()
PY
backup_ready=1

systemctl restart x-ui
for attempt in $(seq 1 15); do
    if ss -lnt | grep -q ':1443 '; then
        break
    fi
    sleep 1
done
ss -lnt | grep -q ':1443 '

systemctl enable --now haproxy
systemctl is-active --quiet haproxy
ss -lnt | grep -q ':443 '
curl --fail --silent --show-error --max-time 15 \
    --resolve dicksuck.astrogate.sbs:443:127.0.0.1 \
    https://dicksuck.astrogate.sbs/ -o /dev/null
ufw allow 1443/tcp >/dev/null

trap - ERR
echo "443 switch succeeded; x-ui backup: $backup"
