#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
SERVICE="$REPO_ROOT/deploy/sub2api-deployer.service"
COMPOSE="$REPO_ROOT/deploy/compose.deployer.yml"
LOADER="$REPO_ROOT/deploy/sub2api-managed-upstream.conf"
EXAMPLE="$REPO_ROOT/deploy/sub2api-deployer.example.json"
TMPFILES="$REPO_ROOT/deploy/sub2api-deployer-tmpfiles.conf"
INSTALLER="$REPO_ROOT/deploy/install-sub2api-deployer.sh"
PACKAGER="$REPO_ROOT/deploy/package-sub2api-deployer-bundles.sh"
CONTROL_PLANE_PREPARER="$REPO_ROOT/deploy/prepare-control-plane-artifacts.sh"
CANDIDATE_PREPARER="$REPO_ROOT/deploy/prepare-release-candidate.sh"
CANDIDATE_FINALIZER="$REPO_ROOT/deploy/finalize-release-candidate.sh"
CANDIDATE_VERIFIER="$REPO_ROOT/deploy/verify-release-candidate.sh"
IMAGE_VERIFIER="$REPO_ROOT/deploy/verify-control-plane-image.sh"
BUNDLE_README="$REPO_ROOT/deploy/DEPLOYER_BUNDLE_README.md"
UPGRADE_HELPER="$REPO_ROOT/deploy/sub2api-deployer-upgrade.sh"
RELEASE_WORKFLOW="$REPO_ROOT/.github/workflows/release.yml"
PREFLIGHT_WORKFLOW="$REPO_ROOT/.github/workflows/release-preflight.yml"
PROMOTE_WORKFLOW="$REPO_ROOT/.github/workflows/promote-release.yml"
RELEASE_SAFETY="$REPO_ROOT/.github/scripts/release-safety.sh"
RELEASE_SAFETY_TEST="$REPO_ROOT/.github/scripts/test-release-safety.sh"
MIGRATION_HISTORY_CHECK="$REPO_ROOT/backend/scripts/check-release-migrations-history.sh"
GORELEASER_CONFIG="$REPO_ROOT/.goreleaser.yaml"
GORELEASER_DOCKERFILE="$REPO_ROOT/Dockerfile.goreleaser"

bash -n "$INSTALLER"
bash -n "$PACKAGER"
bash -n "$CONTROL_PLANE_PREPARER"
bash -n "$CANDIDATE_PREPARER"
bash -n "$CANDIDATE_FINALIZER"
bash -n "$CANDIDATE_VERIFIER"
bash -n "$IMAGE_VERIFIER"
bash -n "$UPGRADE_HELPER"
bash -n "$REPO_ROOT/deploy/tests/sub2api-deployer-upgrade-test.sh"

grep -Fq 'ReadWritePaths=/run/sub2api-deployer /var/lib/sub2api-deployer' "$SERVICE"
grep -Fq 'RuntimeDirectoryPreserve=yes' "$SERVICE"
grep -Fq 'd /run/sub2api-deployer 0755 root root -' "$TMPFILES"
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
grep -Fq 'CONTAINER_SOCKET_DIRECTORY_INODE=$(docker exec "$INSPECTED_CONTAINER_ID" stat -c' "$INSTALLER"
grep -Fq 'APPLICATION_UID=$(docker exec "$INSPECTED_CONTAINER_ID" sh -ceu' "$INSTALLER"
grep -Fq 'docker exec --user "$APPLICATION_UID" "$INSPECTED_CONTAINER_ID" sh -ceu' "$INSTALLER"
grep -Fq 'package-sub2api-deployer-bundles.sh" "$CANDIDATE_DIR/release-assets"' "$CANDIDATE_PREPARER"
grep -Fq 'control_plane_upgrade_ready == true' "$BUNDLE_README"
grep -Fq '.active_container_id == $id' "$UPGRADE_HELPER"
grep -Fq 'write_status failed' "$UPGRADE_HELPER"
grep -Fq 'sub2api-deployer-upgrade.timer' "$INSTALLER"
grep -Fq 'systemctl enable --now sub2api-deployer-upgrade.timer' "$INSTALLER"
if grep -Eq '(^|[[:space:]])docker([[:space:]]|$)' "$UPGRADE_HELPER"; then
  echo "stable activator must not call Docker" >&2
  exit 1
fi
grep -Fq 'release-safety.sh previous-release-json \' "$RELEASE_WORKFLOW"
grep -Fq 'release-safety.sh validate \' "$RELEASE_WORKFLOW"
grep -Fq 'release_commit: ${{ steps.release_ref.outputs.commit }}' "$RELEASE_WORKFLOW"
grep -Fq 'release_tag_object: ${{ steps.release_ref.outputs.tag_object }}' "$RELEASE_WORKFLOW"
grep -Fq 'previous_release_commit: ${{ steps.release_ref.outputs.previous_commit }}' "$RELEASE_WORKFLOW"
grep -Fq 'previous_release_tag_object: ${{ steps.release_ref.outputs.previous_tag_object }}' "$RELEASE_WORKFLOW"
grep -Fq 'ref: ${{ needs.release-gate.outputs.release_commit }}' "$RELEASE_WORKFLOW"
grep -Fq 'MIGRATION_BASELINE_COMMIT: 0572569c6e0187fd02655bec2d9439e30d9edc04' "$RELEASE_WORKFLOW"
if [[ $(grep -Fh 'check-release-migrations-history.sh \' "$RELEASE_WORKFLOW" "$PREFLIGHT_WORKFLOW" | wc -l) -lt 2 ]]; then
  echo "migration history must be checked before candidate construction and again after tag creation" >&2
  exit 1
fi
test -x "$MIGRATION_HISTORY_CHECK"
grep -Fq "git tag --merged \"\$release_commit\" --list 'v*-ts.*'" "$MIGRATION_HISTORY_CHECK"
grep -Fq 'RELEASE_TAG_RULESET_ID: ${{ vars.RELEASE_TAG_RULESET_ID }}' "$RELEASE_WORKFLOW"
if [[ $(grep -Fh 'RELEASE_TAG_CREATION_RULESET_ID: ${{ vars.RELEASE_TAG_CREATION_RULESET_ID }}' "$RELEASE_WORKFLOW" "$PROMOTE_WORKFLOW" | wc -l) -lt 2 ]]; then
  echo "pre-tag promotion and release gate must validate the controlled tag creation ruleset" >&2
  exit 1
fi
if [[ $(grep -Fh 'GH_TOKEN: ${{ github.token }}' "$RELEASE_WORKFLOW" "$PROMOTE_WORKFLOW" | wc -l) -lt 2 ]]; then
  echo "release and promotion GitHub CLI checks must authenticate" >&2
  exit 1
fi
if [[ $(grep -Fh 'verify-rulesets-runtime-json' "$RELEASE_WORKFLOW" "$PROMOTE_WORKFLOW" | wc -l) -lt 2 ]]; then
  echo "promotion and release gate must validate runtime-visible rulesets" >&2
  exit 1
fi
grep -Fq 'RELEASE_TAG_OBJECT: ${{ needs.release-gate.outputs.release_tag_object }}' "$RELEASE_WORKFLOW"
grep -Fq 'PREVIOUS_RELEASE_TAG_OBJECT: ${{ needs.release-gate.outputs.previous_release_tag_object }}' "$RELEASE_WORKFLOW"
grep -Fq '"$CURRENT_PREVIOUS" == "$PREVIOUS_RELEASE_TAG"' "$RELEASE_WORKFLOW"
grep -Fq 'refs/remotes/origin/main' "$RELEASE_WORKFLOW"
grep -Fq -- '--prune-tags' "$RELEASE_SAFETY"
grep -Fq '"$tag_object" == "$expected_tag_object"' "$RELEASE_SAFETY"
grep -Fq 'git merge-base --is-ancestor "$tag_commit" "$main_commit"' "$RELEASE_SAFETY"
grep -Fq 'latest reachable fork tag' "$RELEASE_SAFETY_TEST"
grep -Fq "version_template: '{{ .Env.RELEASE_VERSION }}'" "$GORELEASER_CONFIG"
grep -Fq 'RELEASE_VERSION: ${{ steps.identity.outputs.version }}' "$PREFLIGHT_WORKFLOW"
grep -Fq 'args: release --snapshot --clean --skip=publish --skip=docker' "$PREFLIGHT_WORKFLOW"
if grep -Fq 'goreleaser-action' "$RELEASE_WORKFLOW" || grep -Fq 'setup-go' "$RELEASE_WORKFLOW" || grep -Fq 'pnpm' "$RELEASE_WORKFLOW"; then
  echo "post-tag release workflow must never compile or rebuild candidate bytes" >&2
  exit 1
fi
grep -Fq 'run-id: ${{ inputs.candidate_run_id }}' "$RELEASE_WORKFLOW"
grep -Fq 'assert-image-digest-matches' "$PROMOTE_WORKFLOW"
grep -Fq 'verify-completion-candidate' "$RELEASE_WORKFLOW"
grep -Fq "go test -tags integration ./internal/deployer -run '^TestRealDockerControlPlaneStaging$'" "$PREFLIGHT_WORKFLOW"
test -f "$REPO_ROOT/backend/internal/deployer/control_plane_upgrade_integration_test.go"
grep -Fq 'release-safety.sh publish-release-with-latest \' "$RELEASE_WORKFLOW"
if [[ $(grep -Fc 'gh release view "$RELEASE_TAG"' "$RELEASE_WORKFLOW") -lt 3 ]]; then
  echo "draft and published release states must be verified through GitHub CLI" >&2
  exit 1
fi
if grep -Fq 'releases/tags/${RELEASE_TAG}' "$RELEASE_WORKFLOW"; then
  echo "draft releases must not be queried through the public release-by-tag endpoint" >&2
  exit 1
fi
grep -Fq 'gh release edit "$release_tag" --draft=false --latest' "$RELEASE_SAFETY"
grep -Fq 'trap rollback_latest_tags EXIT' "$RELEASE_SAFETY"
if grep -Fq 'VERSION_MAJOR' "$RELEASE_WORKFLOW" || grep -Fq 'VERSION_MINOR' "$RELEASE_WORKFLOW"; then
  echo "release workflow must only promote latest, not major/minor mutable tags" >&2
  exit 1
fi
grep -Fq '"$RUNNER_TEMP/candidate/release-assets/"* --clobber' "$RELEASE_WORKFLOW"
if [[ $(grep -Fc 'ARG TARGETPLATFORM' "$GORELEASER_DOCKERFILE") -ne 2 ]]; then
  echo "GoReleaser Dockerfile must redeclare TARGETPLATFORM in the final stage" >&2
  exit 1
fi
grep -Fq 'id: sub2api-deployer' "$GORELEASER_CONFIG"
grep -Fq "mod_timestamp: '{{ .CommitTimestamp }}'" "$GORELEASER_CONFIG"
grep -Fq 'io.tokensupply.sub2api.control-plane-protocol: "1"' "$GORELEASER_CONFIG"
grep -Fq 'io.tokensupply.sub2api.control-plane-manifest-sha256:' "$GORELEASER_CONFIG"
grep -Fq 'COPY ${TARGETPLATFORM}/sub2api-deployer /opt/sub2api-control-plane/sub2api-deployer' "$GORELEASER_DOCKERFILE"
if grep -Fq './cmd/deployer' "$REPO_ROOT/Dockerfile"; then
  echo "the non-release Dockerfile must not maintain a second deployer build path" >&2
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
  '.active_container_id == $container_id' \
  '.active_port == $port' \
  '.active_version == $version' \
  '.control_plane_upgrade_ready == true'; do
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
"$REPO_ROOT/deploy/tests/sub2api-deployer-upgrade-test.sh"

artifact_test_dir=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-control-plane-artifacts.XXXXXX")
for arch in amd64 arm64; do
  printf '#!/usr/bin/env bash\n# %s\nexit 0\n' "$arch" > "$artifact_test_dir/deployer-$arch"
  chmod 0755 "$artifact_test_dir/deployer-$arch"
done
jq -n \
  --arg amd64 "$artifact_test_dir/deployer-amd64" \
  --arg arm64 "$artifact_test_dir/deployer-arm64" '
  [
    {type:"Binary", name:"sub2api-deployer", goos:"linux", goarch:"amd64", path:$amd64, extra:{ID:"sub2api-deployer"}},
    {type:"Binary", name:"sub2api-deployer", goos:"linux", goarch:"arm64", path:$arm64, extra:{ID:"sub2api-deployer"}}
  ]' > "$artifact_test_dir/artifacts.json"
CONTROL_MANIFEST_SHA=$(
  "$CONTROL_PLANE_PREPARER" \
    "$artifact_test_dir/artifacts.json" \
    "$artifact_test_dir/dist" \
    "$artifact_test_dir/CONTROL-PLANE-MANIFEST.json" \
    "0.1.168-ts.1" \
    "$(printf 'd%.0s' {1..40})"
)
[[ "$CONTROL_MANIFEST_SHA" =~ ^sha256:[0-9a-f]{64}$ ]]
for arch in amd64 arm64; do
  cmp -s "$artifact_test_dir/deployer-$arch" "$artifact_test_dir/dist/sub2api-deployer-linux-$arch"
  jq -e \
    --arg arch "linux/$arch" \
    --arg sha "sha256:$(sha256sum "$artifact_test_dir/deployer-$arch" | awk '{print $1}')" '
      .schema == 1
      and .version == "0.1.168-ts.1"
      and .runtime_payload[$arch].path == "/opt/sub2api-control-plane/sub2api-deployer"
      and .runtime_payload[$arch].sha256 == $sha
    ' "$artifact_test_dir/CONTROL-PLANE-MANIFEST.json" >/dev/null
done

bundle_test_dir=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-bundle-test.XXXXXX")
trap 'rm -rf -- "$artifact_test_dir" "$bundle_test_dir"' EXIT
for arch in amd64 arm64; do
  printf '#!/usr/bin/env bash\nexit 0\n' > "$bundle_test_dir/sub2api-deployer-linux-$arch"
  chmod +x "$bundle_test_dir/sub2api-deployer-linux-$arch"
done
"$PACKAGER" "$bundle_test_dir"
(
  cd -- "$bundle_test_dir"
  sha256sum --check sub2api-deployer-checksums.txt
)
for arch in amd64 arm64; do
  extracted="$bundle_test_dir/extracted-$arch"
  mkdir -p "$extracted"
  tar -xzf "$bundle_test_dir/sub2api-deployer-linux-$arch.tar.gz" -C "$extracted"
  root="$extracted/sub2api-deployer-linux-$arch"
  (cd -- "$root" && sha256sum --check MANIFEST.sha256)
  jq -e --arg arch "$arch" '.schema == 1 and .os == "linux" and .architecture == $arch' \
    "$root/BUNDLE-MANIFEST.json" >/dev/null
  [[ -x "$root/install-sub2api-deployer.sh" ]]
  [[ -f "$root/deploy/sub2api-deployer-tmpfiles.conf" ]]
done

echo "managed update deployment asset tests passed"
