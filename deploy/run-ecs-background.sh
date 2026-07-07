#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="${SERVICE_NAME:-sub2api-ecs}"
WORK_DIR="${WORK_DIR:-/root/workspace/sub2api}"
RUN_SCRIPT="${RUN_SCRIPT:-${WORK_DIR}/deploy/run-ecs.sh}"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

log() { echo "[run-ecs-background] $*"; }

require_root() {
    if [ "$(id -u)" != "0" ]; then
        echo "Must run as root. Use: sudo bash $0 $*" >&2
        exit 1
    fi
}

require_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        echo "ERROR: systemctl not found. This script is intended for systemd-based ECS images." >&2
        exit 1
    fi
}

require_run_script() {
    if [ ! -f "${RUN_SCRIPT}" ]; then
        echo "ERROR: ${RUN_SCRIPT} not found." >&2
        echo "Upload deploy/run-ecs.sh to ${WORK_DIR}/deploy/ first." >&2
        exit 1
    fi
    chmod +x "${RUN_SCRIPT}"
}

install_service() {
    require_run_script

    cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=Sub2API ECS all-in-one service
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
WorkingDirectory=${WORK_DIR}
ExecStart=/usr/bin/env bash ${RUN_SCRIPT}
Restart=always
RestartSec=5
KillSignal=SIGTERM
TimeoutStopSec=90
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}.service" >/dev/null
    log "Installed and enabled ${SERVICE_NAME}.service"
}

start_service() {
    install_service
    systemctl restart "${SERVICE_NAME}.service"
    systemctl --no-pager --full status "${SERVICE_NAME}.service" || true
}

stop_service() {
    systemctl stop "${SERVICE_NAME}.service"
    log "Stopped ${SERVICE_NAME}.service"
}

restart_service() {
    install_service
    systemctl restart "${SERVICE_NAME}.service"
    systemctl --no-pager --full status "${SERVICE_NAME}.service" || true
}

status_service() {
    systemctl --no-pager --full status "${SERVICE_NAME}.service"
}

logs_service() {
    journalctl -u "${SERVICE_NAME}.service" -f
}

uninstall_service() {
    systemctl disable --now "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
    rm -f "${SERVICE_FILE}"
    systemctl daemon-reload
    log "Uninstalled ${SERVICE_NAME}.service"
}

usage() {
    cat <<EOF
Usage: sudo bash ${0##*/} [start|stop|restart|status|logs|install|uninstall]

Default command: start

Environment overrides:
  SERVICE_NAME=${SERVICE_NAME}
  WORK_DIR=${WORK_DIR}
  RUN_SCRIPT=${RUN_SCRIPT}
EOF
}

main() {
    require_root "$@"
    require_systemd

    case "${1:-start}" in
        start) start_service ;;
        stop) stop_service ;;
        restart) restart_service ;;
        status) status_service ;;
        logs) logs_service ;;
        install) install_service ;;
        uninstall) uninstall_service ;;
        -h|--help|help) usage ;;
        *)
            usage >&2
            exit 2
            ;;
    esac
}

main "$@"
