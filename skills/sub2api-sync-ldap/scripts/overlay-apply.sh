#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ ! -f backend/internal/service/auth_service.go ]]; then
    echo "ERROR: run this script in sub2api repository root."
    exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
    echo "ERROR: worktree is dirty. Commit or stash changes first."
    exit 1
fi

PATCH_BRANCH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --patch-branch)
            PATCH_BRANCH="${2:-}"
            [[ -n "$PATCH_BRANCH" ]] || { echo "ERROR: --patch-branch requires value."; exit 1; }
            shift 2
            ;;
        *)
            echo "ERROR: unknown argument: $1"
            exit 1
            ;;
    esac
done

if [[ -z "$PATCH_BRANCH" ]]; then
    if git show-ref --verify --quiet refs/heads/feature/ldap-patch; then
        PATCH_BRANCH="feature/ldap-patch"
    else
        PATCH_BRANCH="feature/ldap-support"
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
git branch -f upstream-mirror upstream/main >/dev/null

echo "Fetch origin/main (optional)..."
git fetch origin +main:refs/remotes/origin/main >/dev/null 2>&1 || true

UPSTREAM_SHA="$(git rev-parse upstream-mirror)"
PATCH_SHA="$(git rev-parse "${PATCH_BRANCH}")"

if git show-ref --verify --quiet refs/remotes/origin/main; then
    ORIGIN_RELEASE="origin/main"
    ORIGIN_P1="$(git rev-parse "${ORIGIN_RELEASE}^1" 2>/dev/null || true)"
    ORIGIN_P2="$(git rev-parse "${ORIGIN_RELEASE}^2" 2>/dev/null || true)"
    if [[ "$ORIGIN_P1" == "$UPSTREAM_SHA" && "$ORIGIN_P2" == "$PATCH_SHA" ]]; then
        echo "OK: origin/main already matches current upstream+patch."
        git switch -C main "$ORIGIN_RELEASE" >/dev/null
        exit 0
    fi
fi

echo "Create main from upstream-mirror..."
git switch -C main upstream-mirror >/dev/null

if git merge-base --is-ancestor "$PATCH_BRANCH" HEAD; then
    echo "OK: patch branch is already contained in upstream-mirror; no merge needed."
    exit 0
fi

if git merge-base --is-ancestor HEAD "$PATCH_BRANCH"; then
    echo "OK: patch branch already contains upstream base; fast-forward release to patch."
    git switch -C main "$PATCH_BRANCH" >/dev/null
    exit 0
fi

echo "Merge patch branch: ${PATCH_BRANCH}"
if git merge --no-ff "$PATCH_BRANCH" -m "Merge LDAP patch (${PATCH_BRANCH}) into release"; then
    echo "OK: overlay merge completed."
else
    echo "WARN: merge conflicts detected on main, try known auto-resolutions."
    if bash "$SCRIPT_DIR/auto-resolve-wire-conflicts.sh"; then
        if [[ -z "$(git diff --name-only --diff-filter=U)" ]]; then
            git commit --no-edit >/dev/null
            echo "OK: overlay merge completed with auto-resolved known conflicts."
            exit 0
        fi
    fi
    echo "ERROR: merge conflicts detected on main."
    echo "Resolve conflicts, commit, then continue generated-repair.sh -> contract-gate.sh -> backfill-support.sh."
    exit 1
fi
