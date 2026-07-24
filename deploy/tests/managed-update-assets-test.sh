#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
SERVICE="$REPO_ROOT/deploy/sub2api-deployer.service"
COMPOSE="$REPO_ROOT/deploy/compose.deployer.yml"
LOADER="$REPO_ROOT/deploy/sub2api-managed-upstream.conf"
EXAMPLE="$REPO_ROOT/deploy/sub2api-deployer.example.json"
INSTALLER="$REPO_ROOT/deploy/install-sub2api-deployer.sh"
RELEASE_WORKFLOW="$REPO_ROOT/.github/workflows/release.yml"
RELEASE_SAFETY="$REPO_ROOT/.github/scripts/release-safety.sh"
RELEASE_SAFETY_TEST="$REPO_ROOT/.github/scripts/test-release-safety.sh"
MIGRATION_HISTORY_CHECK="$REPO_ROOT/backend/scripts/check-release-migrations-history.sh"
GORELEASER_CONFIG="$REPO_ROOT/.goreleaser.yaml"
GORELEASER_DOCKERFILE="$REPO_ROOT/Dockerfile.goreleaser"

bash -n "$INSTALLER"

grep -Fq 'ReadWritePaths=/run/sub2api-deployer /var/lib/sub2api-deployer' "$SERVICE"
if grep -Eq 'ReadWritePaths=.*(/opt/sub2api/\.deployer\.env|/etc/nginx/conf\.d/sub2api-managed-upstream\.conf)' "$SERVICE"; then
  echo "systemd must grant writable parent directories, not atomic-write target files" >&2
  exit 1
fi

grep -Fq 'SUB2API_DEPLOYER_SOCKET_GID:?SUB2API_DEPLOYER_SOCKET_GID is required' "$COMPOSE"
grep -Fq 'include /var/lib/sub2api-deployer/nginx/managed-upstream.conf;' "$LOADER"

jq -e '
  .image_state_path == "/var/lib/sub2api-deployer/image.env"
  and .nginx_upstream_path == "/var/lib/sub2api-deployer/nginx/managed-upstream.conf"
  and (.compose_project | strings | length > 0)
  and (.nginx_dump_command | length) > 0
  and (.nginx_probe_url | startswith("http://"))
' "$EXAMPLE" >/dev/null

grep -Fq -- '--nginx-probe-url' "$INSTALLER"
grep -Fq '.status == "ok"' "$INSTALLER"
grep -Fq '.job_running == false' "$INSTALLER"
grep -Fq 'validate_idle_state_file "$OLD_STATE_FILE"' "$INSTALLER"
grep -Fq 'STATE_ACTIVE_CONTAINER_ID' "$INSTALLER"
grep -Fq '"$STATE_ACTIVE_CONTAINER_ID" != "$INSPECTED_CONTAINER_ID"' "$INSTALLER"
grep -Fq 'Existing deployment marker path $OLD_RUNTIME_MARKER cannot be changed' "$INSTALLER"
grep -Fq 'Existing deployer socket path $OLD_SOCKET_PATH cannot be changed' "$INSTALLER"
grep -Fq 'install -m 0600 "$TEMP_DIR/state.json" "$STATE_FILE"' "$INSTALLER"
grep -Fq 'backup_directory_metadata "$directory"' "$INSTALLER"
grep -Fq 'ROLLBACK_INCOMPLETE=1' "$INSTALLER"
grep -Fq 'jq -n \' "$INSTALLER"
grep -Fq 'systemctl restart sub2api-deployer.service' "$INSTALLER"
grep -Fq 'rollback_install' "$INSTALLER"
grep -Fq 'acquire_root_lock "$INSTALL_LOCK_FILE" INSTALL_LOCK_FD' "$INSTALLER"
grep -Fq 'INSTALL_LOCK_FILE="/run/sub2api-deployer-install.lock"' "$INSTALLER"
grep -Fq 'acquire_root_lock "$(dirname -- "$OLD_STATE_FILE")/deployer.lock" STATE_LOCK_FD' "$INSTALLER"
grep -Fq 'compose_preflight "$TEMP_DIR/config.json"' "$INSTALLER"
grep -Fq 'load_single_loopback_port "$INSPECTED_CONTAINER_ID"' "$INSTALLER"
grep -Fq 'release-safety.sh previous-release-json \' "$RELEASE_WORKFLOW"
grep -Fq 'release-safety.sh validate "$RELEASE_TAG" "$PREVIOUS_RELEASE_TAG" refs/remotes/origin/main' "$RELEASE_WORKFLOW"
grep -Fq 'release_commit: ${{ steps.release_ref.outputs.commit }}' "$RELEASE_WORKFLOW"
grep -Fq 'release_tag_object: ${{ steps.release_ref.outputs.tag_object }}' "$RELEASE_WORKFLOW"
grep -Fq 'previous_release_commit: ${{ steps.release_ref.outputs.previous_commit }}' "$RELEASE_WORKFLOW"
grep -Fq 'previous_release_tag_object: ${{ steps.release_ref.outputs.previous_tag_object }}' "$RELEASE_WORKFLOW"
grep -Fq 'ref: ${{ needs.release-gate.outputs.release_commit }}' "$RELEASE_WORKFLOW"
grep -Fq 'MIGRATION_BASELINE_COMMIT: 0572569c6e0187fd02655bec2d9439e30d9edc04' "$RELEASE_WORKFLOW"
if [[ $(grep -Fc 'check-release-migrations-history.sh \' "$RELEASE_WORKFLOW") -lt 2 ]]; then
  echo "migration history must be checked both before builds and in the final preflight" >&2
  exit 1
fi
test -x "$MIGRATION_HISTORY_CHECK"
grep -Fq "git tag --merged \"\$release_commit\" --list 'v*-ts.*'" "$MIGRATION_HISTORY_CHECK"
grep -Fq 'RELEASE_TAG_RULESET_ID: ${{ vars.RELEASE_TAG_RULESET_ID }}' "$RELEASE_WORKFLOW"
if [[ $(grep -Fc 'GH_TOKEN: ${{ github.token }}' "$RELEASE_WORKFLOW") -lt 2 ]]; then
  echo "every release ruleset API check must authenticate GitHub CLI" >&2
  exit 1
fi
grep -Fq 'release-safety.sh verify-ruleset-json' "$RELEASE_WORKFLOW"
grep -Fq 'VALIDATED_RELEASE_TAG_OBJECT: ${{ needs.release-gate.outputs.release_tag_object }}' "$RELEASE_WORKFLOW"
grep -Fq 'VALIDATED_PREVIOUS_RELEASE_TAG_OBJECT: ${{ needs.release-gate.outputs.previous_release_tag_object }}' "$RELEASE_WORKFLOW"
grep -Fq '"$CURRENT_PREVIOUS_RELEASE_TAG" != "$VALIDATED_PREVIOUS_RELEASE_TAG"' "$RELEASE_WORKFLOW"
grep -Fq '"$VALIDATED_RELEASE_TAG_OBJECT" \' "$RELEASE_WORKFLOW"
grep -Fq 'refs/remotes/origin/main' "$RELEASE_WORKFLOW"
grep -Fq -- '--prune-tags' "$RELEASE_SAFETY"
grep -Fq '"$tag_object" == "$expected_tag_object"' "$RELEASE_SAFETY"
grep -Fq 'git merge-base --is-ancestor "$tag_commit" "$main_commit"' "$RELEASE_SAFETY"
grep -Fq 'latest reachable fork tag' "$RELEASE_SAFETY_TEST"
grep -Fq 'GORELEASER_CURRENT_TAG: ${{ github.event.inputs.tag || github.ref_name }}' "$RELEASE_WORKFLOW"
grep -Fq 'goreleaser-image-digest dist/artifacts.json "$GHCR_IMAGE"' "$RELEASE_WORKFLOW"
grep -Fq 'release-safety.sh publish-release-with-latest \' "$RELEASE_WORKFLOW"
grep -Fq 'gh release edit "$release_tag" --draft=false --latest' "$RELEASE_SAFETY"
grep -Fq 'trap rollback_latest_tags EXIT' "$RELEASE_SAFETY"
if grep -Fq 'VERSION_MAJOR' "$RELEASE_WORKFLOW" || grep -Fq 'VERSION_MINOR' "$RELEASE_WORKFLOW"; then
  echo "release workflow must only promote latest, not major/minor mutable tags" >&2
  exit 1
fi
grep -Fq 'path: ${{ runner.temp }}/deployer-dist/' "$RELEASE_WORKFLOW"
grep -Fq '"$RUNNER_TEMP/deployer-dist/"*' "$RELEASE_WORKFLOW"
if [[ $(grep -Fc 'ARG TARGETPLATFORM' "$GORELEASER_DOCKERFILE") -ne 2 ]]; then
  echo "GoReleaser Dockerfile must redeclare TARGETPLATFORM in the final stage" >&2
  exit 1
fi
if grep -Eq '^      - (latest|"\{\{ \.Major)' "$GORELEASER_CONFIG"; then
  echo "GoReleaser must not publish mutable image tags before completion verification" >&2
  exit 1
fi
if grep -Fq 'previous-tag' "$RELEASE_WORKFLOW" || grep -Fq 'v0.1.162-ts.3' "$RELEASE_WORKFLOW"; then
  echo "release migration checks must use the fixed immutable commit baseline" >&2
  exit 1
fi
if grep -Fq 'MANAGED_UPDATE_SCHEMA_BASELINE' "$RELEASE_WORKFLOW"; then
  echo "release migration baseline must not be mutable through repository variables" >&2
  exit 1
fi
if grep -Fq 'enable --now sub2api-deployer.service' "$INSTALLER"; then
  echo "installer must explicitly restart an already-running deployer" >&2
  exit 1
fi
if grep -Fq 'sed "s#/opt/sub2api#' "$INSTALLER"; then
  echo "installer must install the path-independent systemd unit without a dead substitution" >&2
  exit 1
fi

container_validation_line=$(grep -n 'validate_container_name "--container" "$CONTAINER_NAME"' "$INSTALLER" | cut -d: -f1)
container_docker_line=$(grep -n 'load_compose_identity "$CONTAINER_NAME"' "$INSTALLER" | head -1 | cut -d: -f1)
if [[ -z "$container_validation_line" || -z "$container_docker_line" || "$container_validation_line" -ge "$container_docker_line" ]]; then
  echo "the requested container name must be validated before it is passed to Docker" >&2
  exit 1
fi

final_health_block=$(sed -n '/^DEPLOYER_READY=0$/,/Deployer health check failed after restart/p' "$INSTALLER")
for predicate in \
  '.job_running == false' \
  '.active_container == $container' \
  '.active_port == $port' \
  '.active_version == $version'; do
  if ! grep -Fq "$predicate" <<<"$final_health_block"; then
    echo "final deployer health verification must enforce $predicate" >&2
    exit 1
  fi
done

quiesce_line=$(grep -n 'systemctl stop sub2api-deployer.service' "$INSTALLER" | tail -1 | cut -d: -f1)
install_line=$(grep -n 'install -m 0755 "$TEMP_DIR/sub2api-deployer" "$INSTALLED_BINARY"' "$INSTALLER" | cut -d: -f1)
if [[ -z "$quiesce_line" || -z "$install_line" || "$quiesce_line" -ge "$install_line" ]]; then
  echo "an existing deployer must be stopped before managed files are replaced" >&2
  exit 1
fi

"$REPO_ROOT/deploy/tests/install-sub2api-deployer-test.sh"

echo "managed update deployment asset tests passed"
