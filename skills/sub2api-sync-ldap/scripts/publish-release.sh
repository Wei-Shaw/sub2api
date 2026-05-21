#!/usr/bin/env bash

set -euo pipefail

usage() {
    cat <<'EOF'
Usage:
  bash publish-release.sh [--release-branch <branch>] [--also-branch <branch>]
EOF
}

RELEASE_BRANCH="main"
ALSO_BRANCH=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --release-branch)
            RELEASE_BRANCH="${2:-}"
            [[ -n "$RELEASE_BRANCH" ]] || { echo "ERROR: --release-branch requires value."; exit 1; }
            shift 2
            ;;
        --also-branch)
            ALSO_BRANCH="${2:-}"
            [[ -n "$ALSO_BRANCH" ]] || { echo "ERROR: --also-branch requires value."; exit 1; }
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

if [[ ! -f backend/internal/service/auth_service.go ]]; then
    echo "ERROR: run this script in sub2api repository root."
    exit 1
fi

push_one_branch() {
    local branch="$1"

    if ! git show-ref --verify --quiet "refs/heads/${branch}"; then
        echo "ERROR: local branch not found: ${branch}"
        exit 1
    fi

    git fetch origin "+${branch}:refs/remotes/origin/${branch}" >/dev/null 2>&1 || true

    local local_sha
    local_sha="$(git rev-parse "${branch}")"
    if ! git show-ref --verify --quiet "refs/remotes/origin/${branch}"; then
        echo "Publish(${branch}): remote branch does not exist, push new."
        git push origin "${branch}"
        return 0
    fi

    local remote_sha
    remote_sha="$(git rev-parse "origin/${branch}")"
    if [[ "$local_sha" == "$remote_sha" ]]; then
        echo "Publish(${branch}): no changes. Skip push."
        return 0
    fi

    if git merge-base --is-ancestor "origin/${branch}" "${branch}"; then
        echo "Publish(${branch}): fast-forward push."
        git push origin "${branch}"
    else
        echo "Publish(${branch}): non-fast-forward update, use --force-with-lease."
        git push --force-with-lease origin "${branch}"
    fi
}

push_one_branch "$RELEASE_BRANCH"
if [[ -n "$ALSO_BRANCH" && "$ALSO_BRANCH" != "$RELEASE_BRANCH" ]]; then
    push_one_branch "$ALSO_BRANCH"
fi
