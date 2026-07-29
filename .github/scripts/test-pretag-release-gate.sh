#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
CI_WORKFLOW="$REPO_ROOT/.github/workflows/backend-ci.yml"
PRETAG_WORKFLOW="$REPO_ROOT/.github/workflows/release-preflight.yml"
PROMOTE_WORKFLOW="$REPO_ROOT/.github/workflows/promote-release.yml"
RELEASE_WORKFLOW="$REPO_ROOT/.github/workflows/release.yml"

fail() {
  echo "$*" >&2
  exit 1
}

require_literal() {
  local file="$1"
  local expected="$2"
  grep -Fq -- "$expected" "$file" || fail "$file is missing required release preflight contract: $expected"
}

DRY_RUN=$(make -n -C "$REPO_ROOT" test-frontend-release)
for expected in \
  'pnpm --dir frontend run lint:check' \
  'pnpm --dir frontend run test:run' \
  'pnpm --dir frontend run build'; do
  [[ "$DRY_RUN" == *"$expected"* ]] || fail "test-frontend-release is missing: $expected"
done

require_literal "$CI_WORKFLOW" 'bash .github/scripts/test-pretag-release-gate.sh'
require_literal "$PRETAG_WORKFLOW" 'name: Release Preflight'
require_literal "$PRETAG_WORKFLOW" 'TARGET_SHA: ${{ github.event.pull_request.head.sha || github.sha }}'
require_literal "$PRETAG_WORKFLOW" 'ref: ${{ env.TARGET_SHA }}'
require_literal "$PRETAG_WORKFLOW" 'git ls-remote --exit-code --tags origin'
require_literal "$PRETAG_WORKFLOW" 'run: make test-frontend-release'
require_literal "$PRETAG_WORKFLOW" 'backend/scripts/check-release-migrations-history.sh'
require_literal "$PRETAG_WORKFLOW" 'go mod tidy'
require_literal "$PRETAG_WORKFLOW" 'package-sub2api-deployer-bundles.sh'
require_literal "$PRETAG_WORKFLOW" 'name: Release Preflight'
require_literal "$PRETAG_WORKFLOW" 'if: always()'
require_literal "$PROMOTE_WORKFLOW" 'name: Promote Release'
require_literal "$PROMOTE_WORKFLOW" 'ref: ${{ inputs.main_sha }}'
require_literal "$PROMOTE_WORKFLOW" 'verify_workflow backend-ci.yml "Backend CI"'
require_literal "$PROMOTE_WORKFLOW" 'verify_workflow release-preflight.yml "Release Preflight"'
require_literal "$PROMOTE_WORKFLOW" 'RELEASE_TAG_DEPLOY_KEY: ${{ secrets.RELEASE_TAG_DEPLOY_KEY }}'
require_literal "$PROMOTE_WORKFLOW" 'RELEASE_TAG_CREATION_RULESET_ID: ${{ vars.RELEASE_TAG_CREATION_RULESET_ID }}'
require_literal "$PROMOTE_WORKFLOW" 'release-promotion-state.sh \'
require_literal "$PROMOTE_WORKFLOW" 'verify-rulesets-json \'
require_literal "$PROMOTE_WORKFLOW" 'git push "git@github.com:${GITHUB_REPOSITORY}.git"'
require_literal "$PROMOTE_WORKFLOW" "if: steps.verify.outputs.tag_exists != 'true'"
require_literal "$PROMOTE_WORKFLOW" 'require-current-main "$EXPECTED_SHA" refs/remotes/origin/main'
require_literal "$PROMOTE_WORKFLOW" 'git tag -a "$RELEASE_TAG"'
require_literal "$PROMOTE_WORKFLOW" 'uses: ./.github/workflows/release.yml'
require_literal "$PROMOTE_WORKFLOW" 'secrets: inherit'
require_literal "$RELEASE_WORKFLOW" 'bash .github/scripts/test-pretag-release-gate.sh'
require_literal "$RELEASE_WORKFLOW" 'run: pnpm run test:run'
require_literal "$RELEASE_WORKFLOW" 'run: pnpm run build'
require_literal "$RELEASE_WORKFLOW" 'workflow_call:'
require_literal "$RELEASE_WORKFLOW" 'ref: ${{ inputs.tag }}'
require_literal "$RELEASE_WORKFLOW" 'RELEASE_TAG_CREATION_RULESET_ID: ${{ vars.RELEASE_TAG_CREATION_RULESET_ID }}'
require_literal "$RELEASE_WORKFLOW" 'verify-rulesets-json \'
if grep -Eq -- '^[[:space:]]+(workflow_dispatch|push):' "$RELEASE_WORKFLOW"; then
  fail "$RELEASE_WORKFLOW must only be callable through the controlled promotion workflow"
fi

echo "Pre-tag release gate contract passed"
