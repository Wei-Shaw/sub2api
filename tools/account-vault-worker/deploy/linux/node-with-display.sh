#!/bin/sh
set -eu

# Set these paths for the account that owns the Linux desktop session.
export HOME=/home/lxq
export PLAYWRIGHT_BROWSERS_PATH=/opt/astrogate-browser
export XDG_RUNTIME_DIR=/run/user/1000
export DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus
export DISPLAY=:0

for auth in /run/user/1000/.mutter-Xwaylandauth.*; do
  if [ -f "$auth" ]; then
    export XAUTHORITY="$auth"
    break
  fi
done

if [ -z "${XAUTHORITY:-}" ]; then
  echo 'No active GNOME display for account vault browser' >&2
  exit 1
fi

node=/home/lxq/.local/opt/node-v24.19.0-linux-x64/bin/node
if [ -x /opt/microsoft/msedge/msedge ]; then
  export ACCOUNT_VAULT_BROWSER_CHANNEL=msedge
fi

# A backend hardened with NoNewPrivileges cannot launch Edge's sandbox.
# Start the browser helper through this user's service manager. --pipe keeps
# the one-use worker ticket on stdin, never in a command argument or file.
if grep -Eq '^NoNewPrivs:[[:space:]]*1$' /proc/self/status; then
  exec /usr/bin/systemd-run --user --pipe --wait --quiet --collect \
    --working-directory="$(pwd -P)" \
    --setenv="HOME=$HOME" \
    --setenv="LANG=${LANG:-C.UTF-8}" \
    --setenv="DISPLAY=$DISPLAY" \
    --setenv="XAUTHORITY=$XAUTHORITY" \
    --setenv="XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR" \
    --setenv="DBUS_SESSION_BUS_ADDRESS=$DBUS_SESSION_BUS_ADDRESS" \
    --setenv="PLAYWRIGHT_BROWSERS_PATH=$PLAYWRIGHT_BROWSERS_PATH" \
    --setenv="ACCOUNT_VAULT_BROWSER_CHANNEL=${ACCOUNT_VAULT_BROWSER_CHANNEL:-}" \
    --setenv="ACCOUNT_VAULT_BROWSER_PROXY=${ACCOUNT_VAULT_BROWSER_PROXY:-}" \
    "$node" "$@"
fi

exec "$node" "$@"
