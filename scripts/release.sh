#!/bin/bash
#
# scripts/release.sh
# Build sub2api binary locally (Mac) and publish as a GitHub Release.
#
# Published assets (must stay in sync with deploy/install-custom.sh fast upgrade):
#   - sub2api-linux-amd64.tar.gz  — tarball contains executable "server"
#   - checksums.txt
# CI GoReleaser (.goreleaser.yaml) instead publishes:
#   - sub2api_${VERSION}_linux_amd64.tar.gz — contains "sub2api"; install-custom tries both names.
#
# Usage:
#   ./scripts/release.sh v1.2.3       # explicit version
#   ./scripts/release.sh              # read backend/cmd/server/VERSION
#
# Requirements: go >= 1.26, pnpm, gh (GitHub CLI logged in), tar, shasum
#

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
print_success() { echo -e "${GREEN}[ OK ]${NC} $*"; }
print_warning() { echo -e "${YELLOW}[WARN]${NC} $*"; }
print_error()   { echo -e "${RED}[ERR ]${NC} $*" >&2; }

# Locate repo root (directory containing backend/, frontend/, scripts/)
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# GitHub repo this release targets. Pin explicitly because `gh` defaults to
# the upstream repo when this clone is a fork, which would cause collisions
# with upstream's release tags.
FORK_REPO="qiangweihewu/sub2api"

# -----------------------------------------------------------------------------
# Version parsing
# -----------------------------------------------------------------------------
resolve_version() {
    local arg="${1:-}"
    if [ -n "$arg" ]; then
        echo "$arg"
        return
    fi
    local version_file="$REPO_ROOT/backend/cmd/server/VERSION"
    if [ ! -f "$version_file" ]; then
        print_error "No version given and $version_file does not exist"
        exit 1
    fi
    echo "v$(tr -d '\r\n' < "$version_file")"
}

validate_version() {
    local v="$1"
    if ! [[ "$v" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        print_error "Invalid version format: '$v' (expected vX.Y.Z)"
        exit 1
    fi
}

check_tag_unused() {
    local v="$1"
    if gh release view "$v" --repo "$FORK_REPO" >/dev/null 2>&1; then
        print_error "Release $v already exists on GitHub. Pick a different version."
        exit 1
    fi
}

# -----------------------------------------------------------------------------
# Dependencies
# -----------------------------------------------------------------------------
check_deps() {
    local missing=()
    for cmd in go pnpm gh tar shasum git; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing+=("$cmd")
        fi
    done
    if [ "${#missing[@]}" -gt 0 ]; then
        print_error "Missing required tools: ${missing[*]}"
        exit 1
    fi
    if ! gh auth status >/dev/null 2>&1; then
        print_error "gh CLI not authenticated. Run: gh auth login"
        exit 1
    fi
}

# -----------------------------------------------------------------------------
# Build steps
# -----------------------------------------------------------------------------
clean_dist() {
    rm -rf "$REPO_ROOT/dist"
    mkdir -p "$REPO_ROOT/dist"
}

build_frontend() {
    print_info "Building frontend (vite build)..."
    (
        cd "$REPO_ROOT/frontend"
        pnpm install --frozen-lockfile
        pnpm run build
    )
    # Output goes to $REPO_ROOT/backend/internal/web/dist per vite.config.ts
    if [ ! -f "$REPO_ROOT/backend/internal/web/dist/index.html" ]; then
        print_error "Frontend build did not produce backend/internal/web/dist/index.html"
        exit 1
    fi
    print_success "Frontend built"
}

build_backend() {
    local version_no_v="$1"
    print_info "Building backend (linux/amd64, -tags embed)..."
    (
        cd "$REPO_ROOT/backend"
        local commit date
        commit=$(git -C "$REPO_ROOT" rev-parse --short HEAD)
        date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
          go build -tags embed \
          -ldflags="-s -w -X main.Version=${version_no_v} -X main.BuildType=release -X main.Commit=${commit} -X main.Date=${date}" \
          -trimpath \
          -o "$REPO_ROOT/dist/server" \
          ./cmd/server
    )
    if [ ! -f "$REPO_ROOT/dist/server" ]; then
        print_error "Backend build did not produce dist/server"
        exit 1
    fi
    print_success "Backend built: $(du -h "$REPO_ROOT/dist/server" | awk '{print $1}')"
}

package_and_checksum() {
    print_info "Packaging tarball..."
    (
        cd "$REPO_ROOT/dist"
        tar -czf sub2api-linux-amd64.tar.gz server
        # shasum -a 256 is portable on Mac; its output matches Linux sha256sum's format
        shasum -a 256 sub2api-linux-amd64.tar.gz > checksums.txt
        print_success "Packaged: $(ls -lh sub2api-linux-amd64.tar.gz | awk '{print $5}')"
        print_info "Checksums:"
        cat checksums.txt
    )
}

publish_release() {
    local version="$1"
    print_info "Creating GitHub release $version on $FORK_REPO..."
    (
        cd "$REPO_ROOT"
        gh release create "$version" \
          --repo "$FORK_REPO" \
          --title "$version" \
          --generate-notes \
          "dist/sub2api-linux-amd64.tar.gz" \
          "dist/checksums.txt"
    )
    print_success "Released $version"
    echo ""
    echo "  To upgrade a production server, run there:"
    echo "  curl -sSL https://raw.githubusercontent.com/qiangweihewu/sub2api/i18n-seo/deploy/install-custom.sh | sudo bash -s -- upgrade"
    echo ""
}

main() {
    local version
    version=$(resolve_version "${1:-}")
    validate_version "$version"
    check_deps
    check_tag_unused "$version"
    print_info "Releasing version: $version"
    clean_dist
    build_frontend
    build_backend "${version#v}"
    package_and_checksum
    publish_release "$version"
}

# Only run main if executed directly (allows sourcing for tests)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
