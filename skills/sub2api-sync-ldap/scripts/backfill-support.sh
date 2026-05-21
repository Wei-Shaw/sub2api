#!/usr/bin/env bash

set -euo pipefail

usage() {
    cat <<'EOF'
Usage:
  bash backfill-support.sh [--release-branch <branch>] [--support-branch <branch>]

Defaults:
  --release-branch main
  --support-branch feature/ldap-support
EOF
}

RELEASE_BRANCH="main"
SUPPORT_BRANCH="feature/ldap-support"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --release-branch)
            RELEASE_BRANCH="${2:-}"
            [[ -n "$RELEASE_BRANCH" ]] || { echo "ERROR: --release-branch requires value."; exit 1; }
            shift 2
            ;;
        --support-branch)
            SUPPORT_BRANCH="${2:-}"
            [[ -n "$SUPPORT_BRANCH" ]] || { echo "ERROR: --support-branch requires value."; exit 1; }
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

if [[ -n "$(git status --porcelain)" ]]; then
    echo "ERROR: worktree is dirty. Commit or stash changes first."
    exit 1
fi

if ! git show-ref --verify --quiet "refs/heads/${RELEASE_BRANCH}"; then
    echo "ERROR: release branch not found: ${RELEASE_BRANCH}"
    exit 1
fi

RELEASE_SHA="$(git rev-parse "${RELEASE_BRANCH}")"
CURRENT_BRANCH="$(git branch --show-current || true)"

if ! git show-ref --verify --quiet "refs/heads/${SUPPORT_BRANCH}"; then
    git branch "${SUPPORT_BRANCH}" "${RELEASE_BRANCH}"
    echo "Backfill: created ${SUPPORT_BRANCH} at ${RELEASE_SHA}"
    exit 0
fi

SUPPORT_SHA="$(git rev-parse "${SUPPORT_BRANCH}")"
if [[ "$SUPPORT_SHA" == "$RELEASE_SHA" ]]; then
    echo "Backfill: no change (${SUPPORT_BRANCH} already at ${RELEASE_BRANCH})."
    exit 0
fi

if git merge-base --is-ancestor "${SUPPORT_BRANCH}" "${RELEASE_BRANCH}"; then
    git branch -f "${SUPPORT_BRANCH}" "${RELEASE_BRANCH}" >/dev/null
    echo "Backfill: fast-forward ${SUPPORT_BRANCH} -> ${RELEASE_SHA}"
    exit 0
fi

echo "Backfill: ${SUPPORT_BRANCH} diverged, merge release into support."
git switch "${SUPPORT_BRANCH}" >/dev/null
git merge --no-ff "${RELEASE_BRANCH}" -m "chore(ldap): backfill support from release"
if [[ -n "$CURRENT_BRANCH" ]]; then
    git switch "$CURRENT_BRANCH" >/dev/null
fi
echo "Backfill: merged ${RELEASE_BRANCH} into ${SUPPORT_BRANCH}."
