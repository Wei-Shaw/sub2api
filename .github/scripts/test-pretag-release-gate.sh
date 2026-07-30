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
require_literal "$PRETAG_WORKFLOW" 'WORKFLOW_SHA: ${{ github.workflow_sha }}'
require_literal "$PRETAG_WORKFLOW" '[[ "$WORKFLOW_SHA" == "$TARGET_SHA" ]]'
require_literal "$PRETAG_WORKFLOW" 'ref: ${{ env.TARGET_SHA }}'
require_literal "$PRETAG_WORKFLOW" 'git ls-remote --exit-code --tags origin'
require_literal "$PRETAG_WORKFLOW" 'run: make test-frontend-release'
require_literal "$PRETAG_WORKFLOW" 'backend/scripts/check-release-migrations-history.sh'
require_literal "$PRETAG_WORKFLOW" 'go mod tidy'
require_literal "$PRETAG_WORKFLOW" 'args: release --snapshot --clean --skip=publish --skip=docker'
require_literal "$PRETAG_WORKFLOW" 'deploy/prepare-release-candidate.sh \'
require_literal "$PRETAG_WORKFLOW" 'push-by-digest=true'
require_literal "$PRETAG_WORKFLOW" '-f event=push -f head_sha="$TARGET_SHA" -f per_page=20'
require_literal "$PRETAG_WORKFLOW" 'source_run_id=$REUSE_RUN_ID'
require_literal "$PRETAG_WORKFLOW" 'deploy/finalize-release-candidate.sh'
require_literal "$PRETAG_WORKFLOW" 'deploy/verify-release-candidate.sh'
require_literal "$PRETAG_WORKFLOW" 'deploy/verify-control-plane-image.sh'
require_literal "$PRETAG_WORKFLOW" "-run '^TestRealDockerControlPlaneStaging$'"
require_literal "$PRETAG_WORKFLOW" 'name: ${{ steps.identity.outputs.artifact_name }}'
require_literal "$PRETAG_WORKFLOW" 'if: steps.reuse.outputs.source_run_id != github.run_id'
require_literal "$PRETAG_WORKFLOW" 'Create or verify immutable candidate tag'
if grep -Fq -- 'assert-image-absent "$CANDIDATE_IMAGE"' "$PRETAG_WORKFLOW"; then
  fail "$PRETAG_WORKFLOW must not create the candidate tag before its recoverable artifact exists"
fi
candidate_upload_line=$(grep -nF 'name: ${{ steps.identity.outputs.artifact_name }}' "$PRETAG_WORKFLOW" | tail -n 1 | cut -d: -f1)
candidate_tag_line=$(grep -nF 'Create or verify immutable candidate tag' "$PRETAG_WORKFLOW" | cut -d: -f1)
if [[ -z "$candidate_upload_line" || -z "$candidate_tag_line" || "$candidate_upload_line" -ge "$candidate_tag_line" ]]; then
  fail "$PRETAG_WORKFLOW must upload the recoverable candidate artifact before creating its immutable tag"
fi
require_literal "$PRETAG_WORKFLOW" 'name: Release Preflight'
require_literal "$PRETAG_WORKFLOW" 'if: always()'
require_literal "$PROMOTE_WORKFLOW" 'name: Promote Release'
require_literal "$PROMOTE_WORKFLOW" 'ref: ${{ inputs.main_sha }}'
require_literal "$PROMOTE_WORKFLOW" '[[ "$WORKFLOW_SHA" == "$EXPECTED_SHA" ]]'
require_literal "$PROMOTE_WORKFLOW" 'verify_workflow backend-ci.yml "Backend CI"'
require_literal "$PROMOTE_WORKFLOW" 'verify_workflow release-preflight.yml "Release Preflight"'
require_literal "$PROMOTE_WORKFLOW" 'name: release-candidate-${{ inputs.main_sha }}'
require_literal "$PROMOTE_WORKFLOW" 'deploy/verify-release-candidate.sh \'
require_literal "$PROMOTE_WORKFLOW" 'assert-image-digest-matches \'
require_literal "$PROMOTE_WORKFLOW" 'RELEASE_TAG_DEPLOY_KEY: ${{ secrets.RELEASE_TAG_DEPLOY_KEY }}'
require_literal "$PROMOTE_WORKFLOW" 'RELEASE_TAG_CREATION_RULESET_ID: ${{ vars.RELEASE_TAG_CREATION_RULESET_ID }}'
require_literal "$PROMOTE_WORKFLOW" 'release-promotion-state.sh \'
require_literal "$PROMOTE_WORKFLOW" 'verify-rulesets-runtime-json \'
require_literal "$PROMOTE_WORKFLOW" 'git push "git@github.com:${GITHUB_REPOSITORY}.git"'
require_literal "$PROMOTE_WORKFLOW" "if: steps.verify.outputs.tag_exists != 'true'"
require_literal "$PROMOTE_WORKFLOW" 'require-current-main "$EXPECTED_SHA" refs/remotes/origin/main'
require_literal "$PROMOTE_WORKFLOW" 'git tag -a "$RELEASE_TAG"'
require_literal "$PROMOTE_WORKFLOW" 'uses: ./.github/workflows/release.yml'
require_literal "$PROMOTE_WORKFLOW" 'secrets: inherit'
require_literal "$RELEASE_WORKFLOW" 'workflow_call:'
require_literal "$RELEASE_WORKFLOW" '[[ "$WORKFLOW_SHA" == "$INPUT_CANDIDATE_COMMIT" ]]'
require_literal "$RELEASE_WORKFLOW" 'ref: ${{ inputs.tag }}'
require_literal "$RELEASE_WORKFLOW" 'name: release-candidate-${{ inputs.candidate_commit }}'
require_literal "$RELEASE_WORKFLOW" 'deploy/verify-release-candidate.sh'
require_literal "$RELEASE_WORKFLOW" 'deploy/verify-control-plane-image.sh'
require_literal "$RELEASE_WORKFLOW" 'Promote candidate digest to immutable version tags'
require_literal "$RELEASE_WORKFLOW" 'Bind tag identity and publish completion ledger'
require_literal "$RELEASE_WORKFLOW" 'RELEASE_TAG_CREATION_RULESET_ID: ${{ vars.RELEASE_TAG_CREATION_RULESET_ID }}'
require_literal "$RELEASE_WORKFLOW" 'verify-rulesets-runtime-json \'
if grep -Eq -- '(goreleaser-action|pnpm run|go build|docker buildx build)' "$RELEASE_WORKFLOW"; then
  fail "$RELEASE_WORKFLOW must publish the audited candidate without rebuilding it"
fi
if grep -Eq -- '^[[:space:]]+(workflow_dispatch|push):' "$RELEASE_WORKFLOW"; then
  fail "$RELEASE_WORKFLOW must only be callable through the controlled promotion workflow"
fi

echo "Pre-tag release gate contract passed"
