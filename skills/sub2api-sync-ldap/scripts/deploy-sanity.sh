#!/usr/bin/env bash

set -euo pipefail

if [[ ! -f deploy/docker-compose.yml ]]; then
    echo "ERROR: run this script in sub2api repository root."
    exit 1
fi

fail() {
    echo "ERROR: $1"
    exit 1
}

find_rg() {
    local candidate
    for candidate in "$(command -v rg 2>/dev/null || true)" /usr/bin/rg /usr/local/bin/rg; do
        [[ -n "$candidate" && -x "$candidate" ]] || continue
        if "$candidate" --version >/dev/null 2>&1; then
            echo "$candidate"
            return 0
        fi
    done
    return 1
}

RG_BIN="$(find_rg || true)"

contains() {
    local pattern="$1"
    local file="$2"

    if [[ -n "$RG_BIN" ]]; then
        "$RG_BIN" -q "$pattern" "$file"
        return $?
    fi

    grep -Eq "$pattern" "$file"
}

check_compose_file() {
    local file="$1"
    [[ -f "$file" ]] || fail "missing file: $file"

    contains "DATA_DIR=/app/data" "$file" || fail "$file missing DATA_DIR=/app/data"
    contains 'healthcheck:' "$file" || fail "$file missing healthcheck block"
    contains 'CMD-SHELL' "$file" || fail "$file healthcheck is not CMD-SHELL"
    contains 'wget -q -T 5 -O /dev/null http://localhost:8080/health' "$file" || fail "$file healthcheck missing wget probe"
    contains 'curl -fsS http://localhost:8080/health' "$file" || fail "$file healthcheck missing curl fallback"
}

echo "Sanity: compose files"
check_compose_file "deploy/docker-compose.yml"
check_compose_file "deploy/docker-compose.local.yml"
check_compose_file "deploy/docker-compose.standalone.yml"

echo "Sanity: dockerfiles"
contains 'HEALTHCHECK .*' Dockerfile || fail "Dockerfile missing HEALTHCHECK"
contains 'wget -q -T 5 -O /dev/null http://localhost:\$\{SERVER_PORT:-8080\}/health' Dockerfile || fail "Dockerfile healthcheck not using wget"
contains 'wget -q -T 5 -O /dev/null http://localhost:\$\{SERVER_PORT:-8080\}/health' deploy/Dockerfile || fail "deploy/Dockerfile healthcheck not using wget"

echo "Sanity: setup docker data dir logic"
contains 'if isContainerRuntime\(\)' backend/internal/setup/setup.go || fail "setup.go missing container runtime branch"
contains 'is not writable in container runtime' backend/internal/setup/setup.go || fail "setup.go missing clear /app/data permission error"

echo "Sanity: deploy script hardening"
contains 'init_directory_permissions\(\)' deploy/docker-deploy.sh || fail "docker-deploy.sh missing init_directory_permissions"
contains 'chown -R 1000:1000 data' deploy/docker-deploy.sh || fail "docker-deploy.sh missing data ownership fix"
contains 'generate_admin_password\(\)' deploy/docker-deploy.sh || fail "docker-deploy.sh missing admin password generation"
contains '\^ADMIN_PASSWORD=' deploy/docker-deploy.sh || fail "docker-deploy.sh missing ADMIN_PASSWORD write-back"
contains 'run_self_check_snapshot\(\)' deploy/docker-deploy.sh || fail "docker-deploy.sh missing self-check snapshot"
contains 'codeload.github.com/big-dimple/sub2api/tar.gz/refs/heads/main' deploy/docker-deploy.sh || fail "docker-deploy.sh missing fork snapshot bootstrap"
contains 'docker build -t weishaw/sub2api:latest' deploy/docker-deploy.sh || fail "docker-deploy.sh missing local LDAP image build"
contains 'docker compose -f "\$\{COMPOSE_FILE\}" up -d' deploy/docker-deploy.sh || fail "docker-deploy.sh missing auto-start compose step"
[[ -f deploy/upgrade_main.sh ]] || fail "missing deploy/upgrade_main.sh"
[[ -f deploy/upgrade_ldap_prod.sh ]] || fail "missing deploy/upgrade_ldap_prod.sh compatibility wrapper"
contains 'git clone --quiet --depth 1 --branch' deploy/upgrade_main.sh || fail "upgrade_main.sh missing remote branch clone logic"
contains 'codeload.github.com' deploy/upgrade_main.sh || fail "upgrade_main.sh missing tarball fallback"
! contains 'refs/remotes/origin/' deploy/upgrade_main.sh || fail "upgrade_main.sh should not rely on local origin refs"
contains 'exec bash "\$TARGET_SCRIPT" "\$@"' deploy/upgrade_ldap_prod.sh || fail "upgrade_ldap_prod.sh not delegating to upgrade_main.sh"

echo "Sanity: deploy docs"
[[ -f deploy/README_LDAP_ENTERPRISE.md ]] || fail "missing deploy/README_LDAP_ENTERPRISE.md"
contains 'curl -fsSL https://raw.githubusercontent.com/big-dimple/sub2api/main/deploy/docker-deploy.sh \| bash' deploy/README_LDAP_ENTERPRISE.md || fail "deploy/README_LDAP_ENTERPRISE.md missing one-line fresh deploy command"
contains 'curl -fsSLo upgrade_main.sh https://raw.githubusercontent.com/big-dimple/sub2api/main/deploy/upgrade_main.sh && bash upgrade_main.sh' deploy/README_LDAP_ENTERPRISE.md || fail "deploy/README_LDAP_ENTERPRISE.md missing one-line upgrade command"
! contains 'upgrade_ldap_prod.sh' deploy/README_LDAP_ENTERPRISE.md || fail "deploy/README_LDAP_ENTERPRISE.md should not mention deprecated upgrade_ldap_prod.sh"

echo "OK: deploy sanity checks passed."
