#!/bin/bash
#
# Sub2API Custom Fork - One-click Deploy Script
# Usage:
#   curl -sSL https://raw.githubusercontent.com/qiangweihewu/sub2api/main/deploy/install-custom.sh | sudo bash
#   curl -sSL https://raw.githubusercontent.com/qiangweihewu/sub2api/i18n-seo/deploy/install-custom.sh | sudo bash
#
# Which branch to clone/pull is chosen in this order:
#   1) Environment GITHUB_DEPLOY_BRANCH (use sudo -E or sudo env ... bash if needed)
#   2) Flags before subcommands: --deploy-branch main | i18n-seo
#   3) Existing install at INSTALL_DIR: current git branch of that checkout
#   4) DEFAULT_GITHUB_DEPLOY_BRANCH below (this copy on GitHub branch main → main; branch i18n-seo → i18n-seo)
#
# Note: with "curl ... | bash" the script cannot see the URL you curled; (4) matches whichever raw path you used.
#
# This script:
#   1. Installs Docker & Docker Compose if missing
#   2. Clones the repo and builds a custom Docker image
#   3. Generates secure secrets (.env)
#   4. Starts all services (sub2api + PostgreSQL + Redis)
#

set -e

# =============================================================================
# Configuration
# =============================================================================
GITHUB_REPO="qiangweihewu/sub2api"
# Last-resort branch when env / flags / existing checkout don't set it (see apply_default_deploy_branch).
DEFAULT_GITHUB_DEPLOY_BRANCH="main"
INSTALL_DIR="/opt/sub2api"
IMAGE_NAME="sub2api-custom"
COMPOSE_PROJECT="sub2api"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

print_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[ OK ]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARN]${NC} $1"; }
print_error()   { echo -e "${RED}[ERR ]${NC} $1"; }

# Generate random secret
generate_secret() { openssl rand -hex 32; }

# Check if /dev/tty is available for interactive prompts
is_interactive() { [ -e /dev/tty ] && [ -r /dev/tty ] && [ -w /dev/tty ]; }

# Validate port number
validate_port() {
    local port="$1"
    [[ "$port" =~ ^[0-9]+$ ]] && [ "$port" -ge 1 ] && [ "$port" -le 65535 ]
}

# =============================================================================
# Pre-flight Checks
# =============================================================================
check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        print_error "Please run as root (use sudo)"
        exit 1
    fi
}

check_os() {
    if [ ! -f /etc/os-release ]; then
        print_error "Unsupported OS (no /etc/os-release found)"
        exit 1
    fi
    . /etc/os-release
    print_info "Detected OS: $PRETTY_NAME"
}

# =============================================================================
# Install Docker if Missing
# =============================================================================
install_docker() {
    if command -v docker &>/dev/null; then
        print_success "Docker already installed: $(docker --version)"
        return
    fi

    print_info "Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    systemctl enable docker
    systemctl start docker
    print_success "Docker installed: $(docker --version)"
}

install_docker_compose() {
    # Docker Compose V2 is bundled as a Docker plugin
    if docker compose version &>/dev/null; then
        print_success "Docker Compose already available: $(docker compose version --short)"
        return
    fi

    print_info "Installing Docker Compose plugin..."
    apt-get update -qq && apt-get install -y -qq docker-compose-plugin 2>/dev/null \
        || yum install -y docker-compose-plugin 2>/dev/null \
        || {
            # Manual install as fallback
            local compose_version
            compose_version=$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
            curl -SL "https://github.com/docker/compose/releases/download/${compose_version}/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
            chmod +x /usr/local/bin/docker-compose
            ln -sf /usr/local/bin/docker-compose /usr/libexec/docker/cli-plugins/docker-compose 2>/dev/null || true
        }
    print_success "Docker Compose installed"
}

# =============================================================================
# Ensure Sufficient Memory (add swap if needed)
# =============================================================================
ensure_memory() {
    local total_mem_kb
    total_mem_kb=$(grep MemTotal /proc/meminfo 2>/dev/null | awk '{print $2}' || echo "0")
    local total_swap_kb
    total_swap_kb=$(grep SwapTotal /proc/meminfo 2>/dev/null | awk '{print $2}' || echo "0")
    local total_available=$(( (total_mem_kb + total_swap_kb) / 1024 ))

    print_info "System memory: $((total_mem_kb / 1024))MB RAM + $((total_swap_kb / 1024))MB swap"

    if [ "$total_available" -lt 3000 ]; then
        print_warning "Less than 3GB total memory. Docker build needs more."
        if [ "$total_swap_kb" -lt 2097152 ]; then
            print_info "Creating 2GB swap file for build..."
            if [ ! -f /swapfile ]; then
                dd if=/dev/zero of=/swapfile bs=1M count=2048 status=progress
                chmod 600 /swapfile
                mkswap /swapfile
            fi
            swapon /swapfile 2>/dev/null || true
            # Persist across reboots
            if ! grep -q '/swapfile' /etc/fstab 2>/dev/null; then
                echo '/swapfile none swap sw 0 0' >> /etc/fstab
            fi
            print_success "Swap enabled: $(swapon --show --noheadings | awk '{print $3}')"
        else
            print_info "Swap already sufficient"
        fi
    else
        print_success "Memory sufficient for build"
    fi
}

# =============================================================================
# Clone & Build
# =============================================================================
apply_default_deploy_branch() {
    if [ -n "${GITHUB_DEPLOY_BRANCH:-}" ]; then
        return
    fi
    if [ -d "$INSTALL_DIR/.git" ]; then
        local detected
        detected="$(git -C "$INSTALL_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
        if [ -n "$detected" ] && [ "$detected" != "HEAD" ]; then
            GITHUB_DEPLOY_BRANCH="$detected"
            print_info "Deploy branch (from existing $INSTALL_DIR checkout): $GITHUB_DEPLOY_BRANCH"
            return
        fi
    fi
    GITHUB_DEPLOY_BRANCH="$DEFAULT_GITHUB_DEPLOY_BRANCH"
}

sync_repo_to_deploy_branch() {
    git fetch origin
    if ! git rev-parse --verify "origin/$GITHUB_DEPLOY_BRANCH" >/dev/null 2>&1; then
        print_error "Remote branch origin/$GITHUB_DEPLOY_BRANCH not found. Check GITHUB_REPO / GITHUB_DEPLOY_BRANCH."
        exit 1
    fi
    if git show-ref --verify --quiet "refs/heads/$GITHUB_DEPLOY_BRANCH"; then
        git checkout "$GITHUB_DEPLOY_BRANCH"
    else
        git checkout -b "$GITHUB_DEPLOY_BRANCH" "origin/$GITHUB_DEPLOY_BRANCH"
    fi
    git pull origin "$GITHUB_DEPLOY_BRANCH"
}

clone_repo() {
    if [ -d "$INSTALL_DIR/.git" ]; then
        print_info "Repo already exists, syncing branch $GITHUB_DEPLOY_BRANCH ..."
        cd "$INSTALL_DIR"
        sync_repo_to_deploy_branch
    else
        print_info "Cloning repo from github.com/$GITHUB_REPO (branch $GITHUB_DEPLOY_BRANCH) ..."
        rm -rf "$INSTALL_DIR"
        git clone -b "$GITHUB_DEPLOY_BRANCH" --single-branch "https://github.com/${GITHUB_REPO}.git" "$INSTALL_DIR"
        cd "$INSTALL_DIR"
    fi
    print_success "Source code ready at $INSTALL_DIR"
}

build_image() {
    cd "$INSTALL_DIR"
    print_info "Building Docker image (this may take a few minutes)..."
    docker build -t "$IMAGE_NAME:latest" .
    print_success "Docker image built: $IMAGE_NAME:latest"
}

# =============================================================================
# Configure Environment
# =============================================================================
configure_env() {
    local env_file="$INSTALL_DIR/deploy/.env"
    local compose_dir="$INSTALL_DIR/deploy"

    # If .env already exists, ask before overwriting
    if [ -f "$env_file" ]; then
        print_warning ".env already exists at $env_file"
        if is_interactive; then
            read -p "Overwrite? Existing secrets will be lost. (y/N): " -r < /dev/tty
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                print_info "Keeping existing .env"
                return
            fi
        else
            print_info "Keeping existing .env (non-interactive mode)"
            return
        fi
    fi

    # Collect settings interactively or use defaults
    local server_port="8080"
    local admin_email="admin@sub2api.local"
    local admin_password=""
    local tz="UTC"

    if is_interactive; then
        echo ""
        echo -e "${CYAN}=============================================="
        echo "  Server Configuration"
        echo "==============================================${NC}"
        echo ""

        read -p "Server port [8080]: " input_port < /dev/tty
        if [ -n "$input_port" ] && validate_port "$input_port"; then
            server_port="$input_port"
        fi

        read -p "Admin email [admin@sub2api.local]: " input_email < /dev/tty
        if [ -n "$input_email" ]; then
            admin_email="$input_email"
        fi

        read -p "Admin password (empty = auto-generate, shown in logs): " input_pw < /dev/tty
        if [ -n "$input_pw" ]; then
            admin_password="$input_pw"
        fi

        read -p "Timezone [UTC]: " input_tz < /dev/tty
        if [ -n "$input_tz" ]; then
            tz="$input_tz"
        fi

        echo ""
    fi

    # Generate secrets
    local pg_password jwt_secret totp_key
    pg_password=$(generate_secret)
    jwt_secret=$(generate_secret)
    totp_key=$(generate_secret)

    # Write .env
    cat > "$env_file" << EOF
# =============================================================================
# Sub2API Custom Deploy - Auto-generated $(date -u +%Y-%m-%dT%H:%M:%SZ)
# =============================================================================

# Server
BIND_HOST=0.0.0.0
SERVER_PORT=${server_port}
SERVER_MODE=release
RUN_MODE=standard
TZ=${tz}

# PostgreSQL
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=${pg_password}
POSTGRES_DB=sub2api

# Redis (no password for internal network)
REDIS_PASSWORD=

# Admin
ADMIN_EMAIL=${admin_email}
ADMIN_PASSWORD=${admin_password}

# Security (auto-generated, do NOT lose these)
JWT_SECRET=${jwt_secret}
TOTP_ENCRYPTION_KEY=${totp_key}
EOF

    chmod 600 "$env_file"

    print_success "Environment configured at $env_file"
    echo ""
    echo -e "  ${YELLOW}IMPORTANT: Save these credentials securely${NC}"
    echo "  PostgreSQL password:  ${pg_password}"
    echo "  JWT secret:           ${jwt_secret:0:16}..."
    echo "  TOTP key:             ${totp_key:0:16}..."
    echo ""
}

# =============================================================================
# Create Compose Override (use custom image instead of weishaw/sub2api)
# =============================================================================
create_compose_override() {
    local override_file="$INSTALL_DIR/deploy/docker-compose.override.yml"

    cat > "$override_file" << EOF
# Auto-generated: use locally built image instead of upstream
services:
  sub2api:
    image: ${IMAGE_NAME}:latest
EOF

    print_success "Compose override created (using local image)"
}

# ensure_compose_override is the idempotent wrapper used by install/upgrade/rollback.
# - missing file   → create fresh
# - already pins our image → leave alone (preserves user-added env/volumes)
# - exists but missing image pin → back up + recreate, warn user to merge back
# Without this, an upgrade after a stray `git checkout` can wipe the override and
# silently fall back to the upstream image.
ensure_compose_override() {
    local override_file="$INSTALL_DIR/deploy/docker-compose.override.yml"

    if [ ! -f "$override_file" ]; then
        print_info "Compose override missing — creating..."
        create_compose_override
        return
    fi

    if grep -qE "image:[[:space:]]+${IMAGE_NAME}:" "$override_file"; then
        return
    fi

    local backup="${override_file}.bak.$(date +%Y%m%d-%H%M%S)"
    print_warning "docker-compose.override.yml exists but doesn't pin '${IMAGE_NAME}:latest'."
    print_warning "Without that pin, the upstream image would be used. Fixing now."
    cp "$override_file" "$backup"
    print_warning "Existing override backed up to: $backup"
    create_compose_override
    print_warning "Recreated baseline override. If the backup contained custom"
    print_warning "environment/volumes/networks, merge them back into:"
    print_warning "  $override_file"
    print_warning "then run: docker compose -p ${COMPOSE_PROJECT} up -d sub2api"
}

# =============================================================================
# Start Services
# =============================================================================
start_services() {
    cd "$INSTALL_DIR/deploy"

    print_info "Starting services..."
    docker compose -p "$COMPOSE_PROJECT" up -d

    # Wait for health check
    print_info "Waiting for services to become healthy..."
    local retries=0
    while [ $retries -lt 30 ]; do
        if docker compose -p "$COMPOSE_PROJECT" ps --format json 2>/dev/null | grep -q '"Health":"healthy"'; then
            break
        fi
        sleep 2
        retries=$((retries + 1))
    done

    print_success "All services started"
}

# =============================================================================
# Upgrade
# =============================================================================
do_upgrade() {
    cd "$INSTALL_DIR"

    print_info "Pulling latest code (branch $GITHUB_DEPLOY_BRANCH)..."
    sync_repo_to_deploy_branch

    print_info "Rebuilding Docker image..."
    ensure_memory
    docker build -t "$IMAGE_NAME:latest" .

    cd "$INSTALL_DIR/deploy"
    ensure_compose_override
    print_info "Restarting services..."
    docker compose -p "$COMPOSE_PROJECT" up -d

    print_success "Upgrade complete!"
}

# =============================================================================
# Rollback (swap to the previous image tag)
# =============================================================================
do_rollback() {
    if ! docker image inspect "${IMAGE_NAME}:previous" >/dev/null 2>&1; then
        print_error "No ${IMAGE_NAME}:previous image found. Cannot rollback."
        print_info "To install a specific prior version, run:"
        print_info "  VERSION=vX.Y.Z curl -sSL <install-custom-url> | sudo -E bash -s -- upgrade"
        exit 1
    fi

    local failed_tag
    failed_tag="${IMAGE_NAME}:failed-$(date +%Y%m%d-%H%M%S)"
    print_info "Archiving current ${IMAGE_NAME}:latest as $failed_tag ..."
    docker tag "${IMAGE_NAME}:latest" "$failed_tag" 2>/dev/null || true

    print_info "Swapping ${IMAGE_NAME}:previous → ${IMAGE_NAME}:latest ..."
    docker tag "${IMAGE_NAME}:previous" "${IMAGE_NAME}:latest"

    cd "$INSTALL_DIR/deploy"
    ensure_compose_override
    print_info "Restarting sub2api container..."
    docker compose -p "$COMPOSE_PROJECT" up -d sub2api

    print_info "Waiting for health check (max 60s)..."
    local retries=0
    while [ $retries -lt 30 ]; do
        local status
        status=$(docker inspect --format '{{.State.Health.Status}}' sub2api 2>/dev/null || echo "starting")
        if [ "$status" = "healthy" ]; then
            print_success "Rollback succeeded. Current version:"
            docker inspect --format '  {{ index .Config.Labels "sub2api.version" }}' sub2api 2>/dev/null || echo "  (no label)"
            return 0
        fi
        sleep 2
        retries=$((retries + 1))
    done

    print_error "Rollback container failed health check after 60s."
    print_error "Check logs: docker compose -p ${COMPOSE_PROJECT} logs sub2api"
    exit 1
}

# =============================================================================
# Fast Upgrade (pull prebuilt binary from GitHub Release, skip source build)
# =============================================================================

# Configurable via environment:
#   VERSION=vX.Y.Z            # target version (default: latest release)
#   FORCE=1                   # re-upgrade even if already on target version
#   NO_RELEASE_FALLBACK=1     # fail instead of falling back to source build

_upgrade_fast_preflight() {
    # jq for GitHub API parsing
    if ! command -v jq >/dev/null 2>&1; then
        print_info "Installing jq (required for upgrade)..."
        apt-get update -qq && apt-get install -y -qq jq 2>/dev/null \
            || yum install -y jq 2>/dev/null \
            || { print_error "Failed to install jq. Install it manually and retry."; exit 1; }
    fi

    # Docker must be available (checked before docker-dir disk checks)
    if ! command -v docker >/dev/null 2>&1; then
        print_error "Docker not installed. Run the full installer first (no args)."
        exit 1
    fi

    # Installation must exist (source-build fallback needs the git tree)
    if [ ! -d "$INSTALL_DIR/.git" ]; then
        print_error "Sub2API not installed. Run the full installer first (no args)."
        exit 1
    fi

    # Architecture check
    local arch
    arch=$(uname -m)
    if [ "$arch" != "x86_64" ]; then
        print_warning "Architecture $arch is not supported by prebuilt releases (only x86_64/amd64)."
        print_info "Falling back to source build..."
        do_upgrade
        exit $?
    fi

    # Disk check: need at least 1GB free in /tmp and DockerRootDir.
    # Do NOT hardcode /var/lib/docker because many hosts customize data-root.
    local free_tmp free_docker docker_root
    free_tmp=$(df -Pm /tmp 2>/dev/null | awk 'NR==2 {print $4}')
    docker_root=$(docker info -f '{{.DockerRootDir}}' 2>/dev/null || true)
    if [ -n "$docker_root" ]; then
        free_docker=$(df -Pm "$docker_root" 2>/dev/null | awk 'NR==2 {print $4}')
    else
        free_docker=""
    fi
    if [ "${free_tmp:-0}" -lt 1024 ] || [ "${free_docker:-0}" -lt 1024 ]; then
        print_error "Insufficient disk space. Need >1GB free in /tmp and DockerRootDir."
        print_error "  /tmp free: ${free_tmp:-?}MB, DockerRootDir(${docker_root:-unknown}) free: ${free_docker:-?}MB"
        exit 1
    fi
}

# Returns via globals: TARGET_VERSION, CURRENT_VERSION
# Exits (via fallback or error) if target cannot be determined.
_upgrade_fast_resolve_versions() {
    # Target: env VERSION wins, else GitHub "latest"
    if [ -n "${VERSION:-}" ]; then
        TARGET_VERSION="$VERSION"
    else
        print_info "Fetching latest release tag from GitHub..."
        local api_response
        if ! api_response=$(curl -fsSL --max-time 30 \
            "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null); then
            print_warning "GitHub API unreachable or no releases yet."
            if [ "${NO_RELEASE_FALLBACK:-0}" = "1" ]; then
                print_error "NO_RELEASE_FALLBACK=1 set; not falling back to source build."
                exit 1
            fi
            print_info "Falling back to source build..."
            do_upgrade
            exit $?
        fi
        TARGET_VERSION=$(echo "$api_response" | jq -r '.tag_name // empty')
        if [ -z "$TARGET_VERSION" ] || [ "$TARGET_VERSION" = "null" ]; then
            print_warning "No release found in GitHub API response."
            if [ "${NO_RELEASE_FALLBACK:-0}" = "1" ]; then
                print_error "NO_RELEASE_FALLBACK=1 set; not falling back."
                exit 1
            fi
            print_info "Falling back to source build..."
            do_upgrade
            exit $?
        fi
    fi

    # Current: read sub2api container's image label
    CURRENT_VERSION=$(docker inspect --format '{{ index .Config.Labels "sub2api.version" }}' sub2api 2>/dev/null || echo "")
    if [ -z "$CURRENT_VERSION" ]; then
        CURRENT_VERSION="(unknown — no label on current image)"
    fi

    print_info "Current version: $CURRENT_VERSION"
    print_info "Target version:  $TARGET_VERSION"

    # Idempotency
    if [ "$CURRENT_VERSION" = "$TARGET_VERSION" ] && [ "${FORCE:-0}" != "1" ]; then
        print_success "Already on $TARGET_VERSION. Pass FORCE=1 to re-run."
        exit 0
    fi
}

# Consumes: TARGET_VERSION
# Produces via globals: STAGE_DIR (contains 'server' + 'Dockerfile')
# Falls back to do_upgrade on download failure (unless NO_RELEASE_FALLBACK=1).
# Aborts on checksum failure (does NOT fall back — signals tampering or corruption).
_upgrade_fast_download() {
    STAGE_DIR=$(mktemp -d -t sub2api-upgrade-XXXXXX)
    local version_num="${TARGET_VERSION#v}"
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/${TARGET_VERSION}"
    local checksums_url="${base_url}/checksums.txt"
    # Two release paths must both work:
    # - GitHub Actions + .goreleaser.yaml → sub2api_${ver}_linux_amd64.tar.gz (binary "sub2api")
    # - scripts/release.sh (local)        → sub2api-linux-amd64.tar.gz (binary "server")
    #
    # Mixed releases can expose BOTH tarballs on GitHub while checksums.txt only lists one
    # (e.g. local release overwrites checksums). Always download checksums first, then pick
    # a tarball name that appears in that file; otherwise fall back to trying downloads.

    print_info "Downloading checksums.txt ..."
    local attempt=0
    while [ $attempt -lt 3 ]; do
        rm -f "$STAGE_DIR/checksums.txt"
        if curl -fsSL --max-time 60 -o "$STAGE_DIR/checksums.txt" "$checksums_url" \
           && [ -s "$STAGE_DIR/checksums.txt" ]; then
            break
        fi
        attempt=$((attempt + 1))
        if [ $attempt -lt 3 ]; then
            print_warning "checksums.txt download failed (attempt $attempt/3), retrying in 5s..."
            sleep 5
        fi
    done

    if [ ! -s "$STAGE_DIR/checksums.txt" ]; then
        rm -rf "$STAGE_DIR"
        print_warning "Failed to download checksums.txt."
        if [ "${NO_RELEASE_FALLBACK:-0}" = "1" ]; then
            print_error "NO_RELEASE_FALLBACK=1 set; not falling back."
            exit 1
        fi
        print_info "Falling back to source build..."
        do_upgrade
        exit $?
    fi

    local asset=""
    local cand
    for cand in "sub2api_${version_num}_linux_amd64.tar.gz" "sub2api-linux-amd64.tar.gz"; do
        if grep -qF "$cand" "$STAGE_DIR/checksums.txt"; then
            asset="$cand"
            print_info "Checksums lists asset: $cand"
            break
        fi
    done

    if [ -z "$asset" ]; then
        print_warning "No known tarball name found in checksums.txt; trying downloads in preference order."
    fi

    _upgrade_fast_curl_tarball() {
        local name="$1"
        local tries=0
        while [ $tries -lt 3 ]; do
            rm -f "$STAGE_DIR/$name"
            if curl -fsSL --max-time 300 -o "$STAGE_DIR/$name" "${base_url}/${name}" \
               && [ -s "$STAGE_DIR/$name" ]; then
                return 0
            fi
            tries=$((tries + 1))
            if [ $tries -lt 3 ]; then
                print_warning "Download $name failed (attempt $tries/3), retrying in 5s..."
                sleep 5
            fi
        done
        return 1
    }

    if [ -n "$asset" ]; then
        print_info "Downloading $asset ..."
        if ! _upgrade_fast_curl_tarball "$asset"; then
            asset=""
        fi
    fi

    if [ -z "$asset" ]; then
        for cand in "sub2api_${version_num}_linux_amd64.tar.gz" "sub2api-linux-amd64.tar.gz"; do
            print_info "Trying release asset: $cand ..."
            if _upgrade_fast_curl_tarball "$cand"; then
                asset="$cand"
                break
            fi
            print_warning "Could not download $cand (missing on release or network error)."
        done
    fi

    if [ -z "$asset" ] || [ ! -s "$STAGE_DIR/$asset" ] || [ ! -s "$STAGE_DIR/checksums.txt" ]; then
        rm -rf "$STAGE_DIR"
        print_warning "Failed to download release assets after trying all known filenames."
        if [ "${NO_RELEASE_FALLBACK:-0}" = "1" ]; then
            print_error "NO_RELEASE_FALLBACK=1 set; not falling back."
            exit 1
        fi
        print_info "Falling back to source build..."
        do_upgrade
        exit $?
    fi

    print_info "Using asset: $asset"
    print_info "Verifying sha256..."
    # GoReleaser uses GNU-style lines: "hash  name" or "hash *name"; piping to sha256sum -c
    # fails if grep does not match the asterisk form. Compare digests explicitly.
    (
        cd "$STAGE_DIR" || exit 1
        _ck_line="$(grep -F "$asset" checksums.txt | head -1 | tr -d '\r')"
        if [ -z "$_ck_line" ]; then
            print_error "No checksum line containing '$asset' in checksums.txt."
            exit 1
        fi
        _expected="$(printf '%s\n' "$_ck_line" | awk '{print $1}')"
        _actual="$(sha256sum "$asset" | awk '{print $1}')"
        if [ "$_expected" != "$_actual" ]; then
            print_error "Checksum mismatch for $asset."
            print_error "Expected (from release): $_expected"
            print_error "Actual (downloaded):    $_actual"
            exit 1
        fi
    ) || {
        rm -rf "$STAGE_DIR"
        print_error "Checksum verification failed. Possible tampering or corruption."
        print_error "Not falling back — please investigate."
        exit 1
    }
    print_success "Checksum OK"

    print_info "Extracting binary..."
    tar -xzf "$STAGE_DIR/$asset" -C "$STAGE_DIR"
    # GoReleaser binary name is sub2api; Dockerfile.runtime expects build context file "server"
    if [ -x "$STAGE_DIR/sub2api" ]; then
        mv "$STAGE_DIR/sub2api" "$STAGE_DIR/server"
    fi
    if [ ! -x "$STAGE_DIR/server" ]; then
        rm -rf "$STAGE_DIR"
        print_error "Extracted tarball did not contain executable 'sub2api' (or 'server')."
        exit 1
    fi

    # Stage the runtime Dockerfile alongside the binary
    if [ ! -f "$INSTALL_DIR/deploy/Dockerfile.runtime" ]; then
        rm -rf "$STAGE_DIR"
        print_error "$INSTALL_DIR/deploy/Dockerfile.runtime not found. Ensure code is up to date:"
        print_error "  cd $INSTALL_DIR && git fetch origin && git checkout $GITHUB_DEPLOY_BRANCH && git pull origin $GITHUB_DEPLOY_BRANCH"
        exit 1
    fi
    cp "$INSTALL_DIR/deploy/Dockerfile.runtime" "$STAGE_DIR/Dockerfile"

    print_success "Staged at $STAGE_DIR"
}

# Consumes: TARGET_VERSION, STAGE_DIR
# Builds new image, tags previous, restarts via compose, health-checks, auto-rollback on failure.
_upgrade_fast_switch() {
    # Tag current latest as previous (best-effort — first upgrade may not have a latest)
    if docker image inspect "${IMAGE_NAME}:latest" >/dev/null 2>&1; then
        print_info "Tagging current ${IMAGE_NAME}:latest → ${IMAGE_NAME}:previous"
        docker tag "${IMAGE_NAME}:latest" "${IMAGE_NAME}:previous"
    fi

    # Best-effort pg_dump backup before swapping. Never blocks upgrade.
    _upgrade_fast_backup_database

    print_info "Building runtime image from staged binary..."
    docker build \
        --label "sub2api.version=${TARGET_VERSION}" \
        -t "${IMAGE_NAME}:latest" \
        -t "${IMAGE_NAME}:${TARGET_VERSION}" \
        "$STAGE_DIR"

    print_info "Restarting sub2api container (PG and Redis unaffected)..."
    cd "$INSTALL_DIR/deploy"
    ensure_compose_override
    docker compose -p "$COMPOSE_PROJECT" up -d sub2api

    print_info "Waiting for health check (max 60s)..."
    local retries=0
    local healthy=0
    while [ $retries -lt 30 ]; do
        local status
        status=$(docker inspect --format '{{.State.Health.Status}}' sub2api 2>/dev/null || echo "starting")
        if [ "$status" = "healthy" ]; then
            healthy=1
            break
        fi
        sleep 2
        retries=$((retries + 1))
    done

    if [ $healthy -eq 1 ]; then
        print_success "Upgrade to $TARGET_VERSION succeeded."
        rm -rf "$STAGE_DIR"
        return 0
    fi

    # Auto-rollback
    print_error "New container failed health check after 60s. Rolling back..."
    if docker image inspect "${IMAGE_NAME}:previous" >/dev/null 2>&1; then
        docker tag "${IMAGE_NAME}:previous" "${IMAGE_NAME}:latest"
        docker compose -p "$COMPOSE_PROJECT" up -d sub2api
        print_error "Rollback complete. Check logs: docker compose -p ${COMPOSE_PROJECT} logs sub2api"
    else
        print_error "No :previous image to rollback to. Container is in failed state."
        print_error "Check logs: docker compose -p ${COMPOSE_PROJECT} logs sub2api"
    fi
    print_error ""
    print_error "If the logs above contain 'migration <file> checksum mismatch', upstream"
    print_error "likely rewrote one or more historical migration files. Fix with:"
    print_error "  sudo bash $INSTALL_DIR/deploy/fix-migration-checksums.sh          # dry-run report"
    print_error "  sudo bash $INSTALL_DIR/deploy/fix-migration-checksums.sh --apply  # write fixes"
    print_error "then retry the upgrade."
    rm -rf "$STAGE_DIR"
    exit 1
}

# _upgrade_fast_backup_database snapshots PostgreSQL via pg_dump before the
# image swap so we have a last-known-good state if a migration goes sideways.
# Failures are non-fatal: print a warning and continue.
_upgrade_fast_backup_database() {
    if ! docker ps --format '{{.Names}}' | grep -q '^sub2api-postgres$'; then
        print_warning "sub2api-postgres not running; skipping pre-upgrade backup."
        return 0
    fi
    local backup_dir="$INSTALL_DIR/backups"
    local backup_file="$backup_dir/pre-${TARGET_VERSION}-$(date +%Y%m%d-%H%M%S).sql.gz"
    local pg_user pg_db
    pg_user=$(grep '^POSTGRES_USER=' "$INSTALL_DIR/deploy/.env" 2>/dev/null | cut -d= -f2 | tr -d '"')
    pg_db=$(grep '^POSTGRES_DB=' "$INSTALL_DIR/deploy/.env" 2>/dev/null | cut -d= -f2 | tr -d '"')
    pg_user=${pg_user:-sub2api}
    pg_db=${pg_db:-sub2api}

    mkdir -p "$backup_dir"
    print_info "Backing up PostgreSQL to $backup_file ..."
    if docker exec "sub2api-postgres" pg_dump -U "$pg_user" "$pg_db" 2>/dev/null | gzip > "$backup_file"; then
        local size
        size=$(du -h "$backup_file" 2>/dev/null | cut -f1)
        print_success "Backup saved ($size). Retain or clean up manually from $backup_dir."
    else
        print_warning "pg_dump failed; proceeding anyway. Investigate if the upgrade misbehaves."
        rm -f "$backup_file"
    fi
}

do_upgrade_fast() {
    _upgrade_fast_preflight
    _upgrade_fast_resolve_versions
    _upgrade_fast_download
    _upgrade_fast_switch
}

# =============================================================================
# Uninstall
# =============================================================================
do_uninstall() {
    print_warning "This will stop and remove all Sub2API containers and data."
    if is_interactive; then
        read -p "Are you sure? (y/N): " -r < /dev/tty
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_info "Cancelled."
            exit 0
        fi
    fi

    cd "$INSTALL_DIR/deploy" 2>/dev/null && \
        docker compose -p "$COMPOSE_PROJECT" down -v 2>/dev/null || true

    docker rmi "$IMAGE_NAME:latest" 2>/dev/null || true

    if is_interactive; then
        read -p "Also remove source code and config ($INSTALL_DIR)? (y/N): " -r < /dev/tty
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            rm -rf "$INSTALL_DIR"
            print_success "All files removed"
        else
            print_info "Source code kept at $INSTALL_DIR"
        fi
    fi

    print_success "Sub2API uninstalled"
}

# =============================================================================
# Print Completion
# =============================================================================
print_completion() {
    local server_port
    server_port=$(grep '^SERVER_PORT=' "$INSTALL_DIR/deploy/.env" 2>/dev/null | cut -d= -f2 || echo "8080")

    # Try to get public IP
    local display_ip
    display_ip=$(curl -s --connect-timeout 5 --max-time 10 https://ipinfo.io/ip 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}' || echo "YOUR_SERVER_IP")

    echo ""
    echo -e "${GREEN}=============================================="
    echo "  Sub2API deployed successfully!"
    echo "==============================================${NC}"
    echo ""
    echo "  Access:  http://${display_ip}:${server_port}"
    echo ""
    echo -e "  ${CYAN}Useful Commands:${NC}"
    echo "  cd $INSTALL_DIR/deploy"
    echo ""
    echo "  View logs:      docker compose -p sub2api logs -f sub2api"
    echo "  Restart:        docker compose -p sub2api restart"
    echo "  Stop:           docker compose -p sub2api down"
    echo "  Upgrade (fast):    curl -sSL https://raw.githubusercontent.com/${GITHUB_REPO}/${GITHUB_DEPLOY_BRANCH}/deploy/install-custom.sh | sudo bash -s -- upgrade"
    echo "  Upgrade (legacy):  curl -sSL https://raw.githubusercontent.com/${GITHUB_REPO}/${GITHUB_DEPLOY_BRANCH}/deploy/install-custom.sh | sudo bash -s -- upgrade-from-source"
    echo "  Rollback:          curl -sSL https://raw.githubusercontent.com/${GITHUB_REPO}/${GITHUB_DEPLOY_BRANCH}/deploy/install-custom.sh | sudo bash -s -- rollback"
    echo ""
    echo -e "  ${YELLOW}Reverse Proxy:${NC}"
    echo "  Point your Nginx/Caddy to http://127.0.0.1:${server_port}"
    echo ""
}

# =============================================================================
# Main
# =============================================================================
main() {
    echo ""
    echo -e "${CYAN}=============================================="
    echo "  Sub2API Custom Fork - Deploy Script"
    echo "==============================================${NC}"
    echo ""

    local -a _deploy_args=()
    while [ $# -gt 0 ]; do
        case "$1" in
            --deploy-branch=*)
                GITHUB_DEPLOY_BRANCH="${1#*=}"
                shift
                ;;
            --deploy-branch)
                if [ -z "${2:-}" ]; then
                    print_error "--deploy-branch requires a value (e.g. main or i18n-seo)"
                    exit 1
                fi
                GITHUB_DEPLOY_BRANCH="$2"
                shift 2
                ;;
            *)
                _deploy_args+=("$1")
                shift
                ;;
        esac
    done
    set -- "${_deploy_args[@]}"

    apply_default_deploy_branch

    case "${1:-}" in
        upgrade|update)
            check_root
            if [ ! -d "$INSTALL_DIR/.git" ]; then
                print_error "Sub2API not installed. Run without arguments to install first."
                exit 1
            fi
            # Ensure we have the latest install-custom.sh and Dockerfile.runtime
            cd "$INSTALL_DIR"
            print_info "Syncing code (for Dockerfile.runtime and script updates)..."
            local _hash_before _hash_after
            _hash_before="$(sha256sum "$INSTALL_DIR/deploy/install-custom.sh" 2>/dev/null | awk '{print $1}')"
            sync_repo_to_deploy_branch
            _hash_after="$(sha256sum "$INSTALL_DIR/deploy/install-custom.sh" 2>/dev/null | awk '{print $1}')"
            # Re-exec when:
            #  - we were piped from curl|bash (BASH_SOURCE empty / different from disk copy), or
            #  - git pull changed install-custom.sh on disk while we still hold stale function
            #    definitions in memory (running locally with `bash deploy/install-custom.sh upgrade`).
            # Without the hash check, a local-form upgrade would silently use yesterday's logic.
            local _pulled _running
            _pulled="$(readlink -f "$INSTALL_DIR/deploy/install-custom.sh" 2>/dev/null || echo "$INSTALL_DIR/deploy/install-custom.sh")"
            _running="$(readlink -f "${BASH_SOURCE[0]:-}" 2>/dev/null || true)"
            if [ -z "$_running" ] || [ "$_running" != "$_pulled" ] || [ "$_hash_before" != "$_hash_after" ]; then
                if [ -n "$_hash_before" ] && [ "$_hash_before" != "$_hash_after" ]; then
                    print_info "install-custom.sh updated by git pull — re-executing with fresh logic..."
                fi
                exec bash "$INSTALL_DIR/deploy/install-custom.sh" "$@"
            fi
            do_upgrade_fast
            exit 0
            ;;
        upgrade-from-source)
            check_root
            if [ ! -d "$INSTALL_DIR/.git" ]; then
                print_error "Sub2API not installed. Run without arguments to install first."
                exit 1
            fi
            do_upgrade
            exit 0
            ;;
        rollback)
            check_root
            do_rollback
            exit 0
            ;;
        uninstall|remove)
            check_root
            do_uninstall
            exit 0
            ;;
        --help|-h)
            echo "Usage: $0 [--deploy-branch <name>] [command]"
            echo ""
            echo "Deploy branch (clone/pull): env GITHUB_DEPLOY_BRANCH, or --deploy-branch, or"
            echo "existing checkout under $INSTALL_DIR, else baked default for this script."
            echo ""
            echo "Commands:"
            echo "  (none)                Install Sub2API (clone, build, start)"
            echo "  upgrade               Fast upgrade: pull prebuilt binary from latest GitHub Release"
            echo "                        Env: VERSION=vX.Y.Z  target specific version"
            echo "                             FORCE=1         re-run even if already on target"
            echo "                             NO_RELEASE_FALLBACK=1  fail instead of source-build fallback"
            echo "  upgrade-from-source   Legacy: git pull + docker build (slow, uses lots of RAM)"
            echo "  rollback              Revert to ${IMAGE_NAME}:previous image"
            echo "  uninstall             Stop and remove everything"
            echo ""
            exit 0
            ;;
    esac

    # Default: Full install
    check_root
    check_os
    install_docker
    install_docker_compose
    ensure_memory
    clone_repo
    build_image
    configure_env
    ensure_compose_override
    start_services
    print_completion
}

main "$@"
