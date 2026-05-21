#!/usr/bin/env bash

set -euo pipefail

usage() {
    cat <<'EOF'
Usage:
  bash upstream-preflight.sh [--patch-branch <branch>] [--change-threshold <percent>]

Options:
  --patch-branch <name>     Use specific patch branch (default auto detect).
  --change-threshold <n>    Override the drift threshold percentage (default: 40).
  -h, --help                Show this help.
EOF
}

if [[ ! -f backend/internal/service/auth_service.go ]]; then
    echo "ERROR: run this script in sub2api repository root."
    exit 1
fi

PATCH_BRANCH=""
THRESHOLD="${LDAP_SYNC_CHANGE_THRESHOLD:-40}"
while [[ $# -gt 0 ]]; do
    case "$1" in
        --patch-branch)
            PATCH_BRANCH="${2:-}"
            [[ -n "$PATCH_BRANCH" ]] || { echo "ERROR: --patch-branch requires value."; exit 1; }
            shift 2
            ;;
        --change-threshold)
            THRESHOLD="${2:-}"
            [[ -n "$THRESHOLD" ]] || { echo "ERROR: --change-threshold requires value."; exit 1; }
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "ERROR: unknown argument: $1"
            usage
            exit 1
            ;;
    esac
done

if ! [[ "$THRESHOLD" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    echo "ERROR: --change-threshold must be a number, got: ${THRESHOLD}"
    exit 1
fi

if [[ -z "$PATCH_BRANCH" ]]; then
    if git show-ref --verify --quiet refs/heads/feature/ldap-patch; then
        PATCH_BRANCH="feature/ldap-patch"
    elif git show-ref --verify --quiet refs/heads/feature/ldap-support; then
        PATCH_BRANCH="feature/ldap-support"
    else
        echo "ERROR: no patch branch found (feature/ldap-patch or feature/ldap-support)."
        exit 1
    fi
fi

if ! git show-ref --verify --quiet "refs/heads/${PATCH_BRANCH}"; then
    echo "ERROR: patch branch not found: ${PATCH_BRANCH}"
    exit 1
fi

if ! git remote | grep -qx upstream; then
    git remote add upstream https://github.com/Wei-Shaw/sub2api.git
fi

echo "Fetch upstream/main + tags..."
if ! git fetch upstream main --tags --quiet; then
    echo "ERROR: failed to fetch upstream/main."
    exit 1
fi

UPSTREAM_FILE="$(git show upstream/main:backend/internal/service/auth_service.go 2>/dev/null || true)"
if [[ -z "$UPSTREAM_FILE" ]]; then
    echo "ERROR: cannot read upstream auth_service.go."
    exit 1
fi

if ! grep -q "type AuthService struct" <<<"$UPSTREAM_FILE"; then
    echo "ERROR: upstream AuthService not found; large refactor detected."
    exit 2
fi

NUMSTAT="$(git diff --numstat "${PATCH_BRANCH}...upstream/main" -- \
    backend/internal/service/auth_service.go \
    frontend/src/views/admin/SettingsView.vue || true)"

ADDED=0
DELETED=0
if [[ -n "$NUMSTAT" ]]; then
    ADDED="$(awk '{a+=$1} END {print a+0}' <<<"$NUMSTAT")"
    DELETED="$(awk '{d+=$2} END {print d+0}' <<<"$NUMSTAT")"
fi
CHANGED=$((ADDED + DELETED))

AUTH_LINES="$(git show "${PATCH_BRANCH}:backend/internal/service/auth_service.go" | wc -l | tr -d ' ')"
SETTINGS_LINES="$(git show "${PATCH_BRANCH}:frontend/src/views/admin/SettingsView.vue" | wc -l | tr -d ' ')"
BASE_LINES=$((AUTH_LINES + SETTINGS_LINES))
if [[ "$BASE_LINES" -le 0 ]]; then
    BASE_LINES=1
fi

CHANGE_PERCENT="$(awk -v c="$CHANGED" -v b="$BASE_LINES" 'BEGIN { printf "%.2f", (c*100.0)/b }')"
echo "Preflight: +${ADDED} / -${DELETED}, total=${CHANGED}, ratio=${CHANGE_PERCENT}% (vs ${PATCH_BRANCH})"

EXCEED="$(awk -v p="$CHANGE_PERCENT" -v t="$THRESHOLD" 'BEGIN { if (p > t) print "yes"; else print "no" }')"
if [[ "$EXCEED" == "yes" ]]; then
    echo "ERROR: change ratio ${CHANGE_PERCENT}% > ${THRESHOLD}% threshold."
    echo "Hint: review upstream drift, then retry with --change-threshold <percent> only if the extra churn is expected."
    exit 2
fi

echo "OK: preflight passed."
