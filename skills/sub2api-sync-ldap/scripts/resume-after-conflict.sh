#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
    cat <<'EOF'
Usage:
  bash resume-after-conflict.sh [--release-branch <branch>] [--backfill-branch <branch>] [--publish] [--no-publish] [--full-test] [--skip-deploy-sanity]

Options:
  --release-branch <name>   Checked-out release branch to validate and publish (default: main).
  --backfill-branch <name>  Target branch for backfill (default: feature/ldap-support).
  --publish                 Compatibility alias; publish is already the default.
  --no-publish              Keep the recovery local only.
  --full-test               Run full backend test suites in contract gate stage.
  --skip-deploy-sanity      Skip deploy consistency checks.
  -h, --help                Show this help.
EOF
}

RELEASE_BRANCH="main"
BACKFILL_BRANCH="feature/ldap-support"
PUBLISH=1
FULL_TEST=0
DO_DEPLOY_SANITY=1

commit_generated_artifacts() {
    if [[ -z "$(git status --porcelain)" ]]; then
        echo "OK: no generated sync artifacts to commit."
        return 0
    fi

    git add -A
    git commit -m "chore(ldap): regenerate sync artifacts" >/dev/null
    echo "OK: committed generated sync artifacts."
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --release-branch)
            RELEASE_BRANCH="${2:-}"
            [[ -n "$RELEASE_BRANCH" ]] || { echo "ERROR: --release-branch requires value."; exit 1; }
            shift 2
            ;;
        --backfill-branch)
            BACKFILL_BRANCH="${2:-}"
            [[ -n "$BACKFILL_BRANCH" ]] || { echo "ERROR: --backfill-branch requires value."; exit 1; }
            shift 2
            ;;
        --publish)
            PUBLISH=1
            shift
            ;;
        --no-publish)
            PUBLISH=0
            shift
            ;;
        --full-test)
            FULL_TEST=1
            shift
            ;;
        --skip-deploy-sanity)
            DO_DEPLOY_SANITY=0
            shift
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

if [[ ! -d backend || ! -f backend/internal/service/auth_service.go ]]; then
    echo "ERROR: run this script in sub2api repository root."
    exit 1
fi

CURRENT_BRANCH="$(git branch --show-current || true)"
if [[ "$CURRENT_BRANCH" != "$RELEASE_BRANCH" ]]; then
    echo "ERROR: expected checked-out release branch ${RELEASE_BRANCH}, current branch is ${CURRENT_BRANCH:-<detached>}."
    exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
    echo "ERROR: worktree is dirty. Commit the conflict resolution before resuming."
    exit 1
fi

TOTAL_STEPS=3
if [[ "$DO_DEPLOY_SANITY" -eq 1 ]]; then
    TOTAL_STEPS=$((TOTAL_STEPS + 1))
fi
if [[ "$PUBLISH" -eq 1 ]]; then
    TOTAL_STEPS=$((TOTAL_STEPS + 1))
fi
STEP=1

echo "[${STEP}/${TOTAL_STEPS}] generated repair"
bash "$SCRIPT_DIR/generated-repair.sh"
STEP=$((STEP + 1))

echo "[${STEP}/${TOTAL_STEPS}] contract gate"
if [[ "$FULL_TEST" -eq 1 ]]; then
    LDAP_SYNC_FULL_TESTS=1 bash "$SCRIPT_DIR/contract-gate.sh"
else
    bash "$SCRIPT_DIR/contract-gate.sh"
fi
STEP=$((STEP + 1))

if [[ "$DO_DEPLOY_SANITY" -eq 1 ]]; then
    echo "[${STEP}/${TOTAL_STEPS}] deploy sanity"
    bash "$SCRIPT_DIR/deploy-sanity.sh"
    STEP=$((STEP + 1))
fi

echo "[resume] finalize generated artifacts"
commit_generated_artifacts

echo "[${STEP}/${TOTAL_STEPS}] backfill patch source branch (${BACKFILL_BRANCH})"
bash "$SCRIPT_DIR/backfill-support.sh" --release-branch "$RELEASE_BRANCH" --support-branch "$BACKFILL_BRANCH"
STEP=$((STEP + 1))

if [[ "$PUBLISH" -eq 1 ]]; then
    echo "[${STEP}/${TOTAL_STEPS}] publish release branches"
    bash "$SCRIPT_DIR/publish-release.sh" --release-branch "$RELEASE_BRANCH" --also-branch "$BACKFILL_BRANCH"
fi

echo "DONE: conflict recovery workflow completed."
