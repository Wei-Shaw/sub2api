#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
SAFETY_SCRIPT="$REPO_ROOT/.github/scripts/release-safety.sh"
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-release-safety.XXXXXX")
TEST_REPO="$TEST_ROOT/repo"

cleanup() {
  case "$TEST_ROOT" in
    *sub2api-release-safety.*) find "$TEST_ROOT" -depth -delete ;;
    *) echo "Refusing to clean unexpected test path: $TEST_ROOT" >&2 ;;
  esac
}
trap cleanup EXIT

fail() {
  echo "$*" >&2
  exit 1
}

expect_failure() {
  local expected="$1"
  shift
  local output
  if output=$("$@" 2>&1); then
    fail "Command unexpectedly succeeded: $*"
  fi
  [[ "$output" == *"$expected"* ]] || \
    fail "Failure did not contain '$expected': $output"
}

git init -q -b main "$TEST_REPO"
git -C "$TEST_REPO" config user.name "Release Safety Test"
git -C "$TEST_REPO" config user.email "release-safety@example.invalid"

printf 'base\n' > "$TEST_REPO/state.txt"
git -C "$TEST_REPO" add state.txt
git -C "$TEST_REPO" commit -q -m base
BASE_COMMIT=$(git -C "$TEST_REPO" rev-parse HEAD)
git -C "$TEST_REPO" tag -a v1.0.0-ts.1 -m 'release 1'
git -C "$TEST_REPO" tag v0.9.0-ts.1

printf 'main\n' >> "$TEST_REPO/state.txt"
mkdir -p "$TEST_REPO/docs/releases"
PROBE_FILE="$TEST_ROOT/release-notes-were-executed"
ATTACK_LINE="'; touch \"$PROBE_FILE\"; #"
RELEASE_NOTES=$(printf '%s\n' \
  '# TokenSupply v1.0.0-ts.10' \
  '' \
  "apostrophe: it's data" \
  'EOF' \
  'message<<EOF' \
  "$ATTACK_LINE" \
  'RELEASE_NOTES_FIXED')
printf '%s\n' "$RELEASE_NOTES" > "$TEST_REPO/docs/releases/v1.0.0-ts.10.md"
git -C "$TEST_REPO" add state.txt docs/releases/v1.0.0-ts.10.md
git -C "$TEST_REPO" commit -q -m main
MAIN_COMMIT=$(git -C "$TEST_REPO" rev-parse HEAD)
git -C "$TEST_REPO" tag -a v1.0.0-ts.2 -m 'release 2'

TAG_BODY='This annotated tag body is intentionally not used as release notes.'
git -C "$TEST_REPO" tag -a v1.0.0-ts.10 -m 'release 10' -m "$TAG_BODY"
git -C "$TEST_REPO" tag -a v1.0.0-ts.9 -m 'failed release tag without a published GitHub Release'

RELEASES_JSON="$TEST_ROOT/releases.json"
jq -n '[
  [
    {tag_name: "v1.0.0-ts.1", draft: false, prerelease: false, published_at: "2026-01-01T00:00:00Z", assets: [{name: "sub2api-release-complete.json"}]},
    {tag_name: "v1.0.0-ts.2", draft: false, prerelease: false, published_at: "2026-01-02T00:00:00Z", assets: [{name: "sub2api-release-complete.json"}]}
  ],
  [
    {tag_name: "v1.0.0-ts.7", draft: false, prerelease: false, published_at: null, assets: [{name: "sub2api-release-complete.json"}]},
    {tag_name: "v1.0.0-ts.8", draft: false, prerelease: true, published_at: "2026-01-08T00:00:00Z", assets: [{name: "sub2api-release-complete.json"}]},
    {tag_name: "v1.0.0-ts.9", draft: false, prerelease: false, published_at: "2026-01-09T00:00:00Z", assets: []},
    {tag_name: "v1.0.0-ts.10", draft: true, prerelease: false, published_at: null, assets: [{name: "sub2api-release-complete.json"}]},
    {tag_name: "not-a-fork-release", draft: false, prerelease: false, published_at: "2026-01-03T00:00:00Z", assets: [{name: "sub2api-release-complete.json"}]}
  ]
]' > "$RELEASES_JSON"
PREVIOUS_RELEASE_TAG=$(bash "$SAFETY_SCRIPT" previous-release-json v1.0.0-ts.10 "$RELEASES_JSON" v1.0.0-ts.1)
[[ "$PREVIOUS_RELEASE_TAG" == "v1.0.0-ts.2" ]] || \
  fail "Completed release selection returned $PREVIOUS_RELEASE_TAG instead of v1.0.0-ts.2"

BOOTSTRAP_RELEASES_JSON="$TEST_ROOT/bootstrap-releases.json"
jq -n '[{tag_name: "v1.0.0-ts.2", draft: false, assets: []}]' > "$BOOTSTRAP_RELEASES_JSON"
BOOTSTRAP_PREVIOUS=$(bash "$SAFETY_SCRIPT" previous-release-json v1.0.0-ts.10 "$BOOTSTRAP_RELEASES_JSON" v1.0.0-ts.1)
[[ "$BOOTSTRAP_PREVIOUS" == "v1.0.0-ts.1" ]] || \
  fail "Completion selection did not fall back to the bootstrap release"
printf '{malformed' > "$TEST_ROOT/malformed-releases.json"
expect_failure 'Could not parse GitHub Releases' bash "$SAFETY_SCRIPT" \
  previous-release-json v1.0.0-ts.10 "$TEST_ROOT/malformed-releases.json" v1.0.0-ts.1

bash "$SAFETY_SCRIPT" assert-unpublished-json v1.0.0-ts.11 "$RELEASES_JSON"
expect_failure 'already exists' bash "$SAFETY_SCRIPT" assert-unpublished-json v1.0.0-ts.10 "$RELEASES_JSON"
bash "$SAFETY_SCRIPT" assert-not-published-json v1.0.0-ts.10 "$RELEASES_JSON"
expect_failure 'already published' bash "$SAFETY_SCRIPT" assert-not-published-json v1.0.0-ts.2 "$RELEASES_JSON"

VERSION_FILE="$TEST_ROOT/VERSION"
printf '%s\n' '1.0.0-ts.10' > "$VERSION_FILE"
bash "$SAFETY_SCRIPT" verify-version-file v1.0.0-ts.10 "$VERSION_FILE"
printf '%s\n' '1.0.0-ts.9' > "$VERSION_FILE"
expect_failure 'VERSION file must equal' bash "$SAFETY_SCRIPT" verify-version-file v1.0.0-ts.10 "$VERSION_FILE"

git -C "$TEST_REPO" checkout -q "$BASE_COMMIT"
expect_failure 'latest reachable fork tag is v1.0.0-ts.10' \
  bash -c 'cd "$1" && bash "$2" validate v1.0.0-ts.1 v0.9.0-ts.1 "$3"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$MAIN_COMMIT"

git -C "$TEST_REPO" checkout -q main
VALIDATED_METADATA=$(bash -c 'cd "$1" && bash "$2" validate v1.0.0-ts.10 "$3" "$4"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$PREVIOUS_RELEASE_TAG" "$MAIN_COMMIT")
IFS=$'\t' read -r \
  VALIDATED_COMMIT \
  VALIDATED_TAG_OBJECT \
  VALIDATED_PREVIOUS_TAG \
  VALIDATED_PREVIOUS_COMMIT \
  VALIDATED_PREVIOUS_TAG_OBJECT <<<"$VALIDATED_METADATA"
[[ "$VALIDATED_COMMIT" == "$MAIN_COMMIT" ]] || \
  fail "Release validation returned unexpected commit: $VALIDATED_COMMIT"
[[ "$VALIDATED_TAG_OBJECT" == "$(git -C "$TEST_REPO" rev-parse v1.0.0-ts.10^{tag})" ]] || \
  fail "Release validation returned unexpected tag object: $VALIDATED_TAG_OBJECT"
[[ "$VALIDATED_PREVIOUS_TAG" == "v1.0.0-ts.2" ]] || \
  fail "Release validation returned unexpected previous tag: $VALIDATED_PREVIOUS_TAG"
[[ "$VALIDATED_PREVIOUS_COMMIT" == "$MAIN_COMMIT" ]] || \
  fail "Release validation returned unexpected previous commit: $VALIDATED_PREVIOUS_COMMIT"
[[ "$VALIDATED_PREVIOUS_TAG_OBJECT" == "$(git -C "$TEST_REPO" rev-parse v1.0.0-ts.2^{tag})" ]] || \
  fail "Release validation returned unexpected previous tag object: $VALIDATED_PREVIOUS_TAG_OBJECT"
bash -c 'cd "$1" && bash "$2" verify-tag v1.0.0-ts.10 "$3" "$4" "$5" "$6" "$7" "$8"' _ \
  "$TEST_REPO" \
  "$SAFETY_SCRIPT" \
  "$VALIDATED_COMMIT" \
  "$VALIDATED_TAG_OBJECT" \
  "$VALIDATED_PREVIOUS_TAG" \
  "$VALIDATED_PREVIOUS_COMMIT" \
  "$VALIDATED_PREVIOUS_TAG_OBJECT" \
  "$MAIN_COMMIT"
expect_failure 'moved after validation' \
  bash -c 'cd "$1" && bash "$2" verify-tag v1.0.0-ts.10 "$3" "$4" "$5" "$6" "$7" "$8"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$BASE_COMMIT" "$VALIDATED_TAG_OBJECT" \
  "$VALIDATED_PREVIOUS_TAG" "$VALIDATED_PREVIOUS_COMMIT" "$VALIDATED_PREVIOUS_TAG_OBJECT" "$MAIN_COMMIT"

git -C "$TEST_REPO" tag -f -a v1.0.0-ts.10 -m 'replacement annotation' "$MAIN_COMMIT" >/dev/null
expect_failure 'tag object v1.0.0-ts.10 moved' \
  bash -c 'cd "$1" && bash "$2" verify-tag v1.0.0-ts.10 "$3" "$4" "$5" "$6" "$7" "$8"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$VALIDATED_COMMIT" "$VALIDATED_TAG_OBJECT" \
  "$VALIDATED_PREVIOUS_TAG" "$VALIDATED_PREVIOUS_COMMIT" "$VALIDATED_PREVIOUS_TAG_OBJECT" "$MAIN_COMMIT"
git -C "$TEST_REPO" tag -f -a v1.0.0-ts.10 -m 'release 10' -m "$TAG_BODY" "$MAIN_COMMIT" >/dev/null
VALIDATED_TAG_OBJECT=$(git -C "$TEST_REPO" rev-parse v1.0.0-ts.10^{tag})

git -C "$TEST_REPO" tag -f -a v1.0.0-ts.2 -m 'replacement previous annotation' "$MAIN_COMMIT" >/dev/null
expect_failure 'Previous release tag object v1.0.0-ts.2 moved' \
  bash -c 'cd "$1" && bash "$2" verify-tag v1.0.0-ts.10 "$3" "$4" "$5" "$6" "$7" "$8"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$VALIDATED_COMMIT" "$VALIDATED_TAG_OBJECT" \
  "$VALIDATED_PREVIOUS_TAG" "$VALIDATED_PREVIOUS_COMMIT" "$VALIDATED_PREVIOUS_TAG_OBJECT" "$MAIN_COMMIT"
git -C "$TEST_REPO" tag -f -a v1.0.0-ts.2 -m 'release 2' "$MAIN_COMMIT" >/dev/null
VALIDATED_PREVIOUS_TAG_OBJECT=$(git -C "$TEST_REPO" rev-parse v1.0.0-ts.2^{tag})

git -C "$TEST_REPO" checkout -q -b side "$BASE_COMMIT"
printf 'side\n' >> "$TEST_REPO/state.txt"
git -C "$TEST_REPO" commit -q -am side
git -C "$TEST_REPO" tag -a v9.0.0-ts.1 -m 'unmerged release'
expect_failure 'is not reachable' \
  bash -c 'cd "$1" && bash "$2" validate v9.0.0-ts.1 v1.0.0-ts.2 "$3"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$MAIN_COMMIT"
expect_failure 'Invalid fork release tag' \
  bash -c 'cd "$1" && bash "$2" validate "v1.0.0-ts.1;touch-x" v1.0.0-ts.2 "$3"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$MAIN_COMMIT"
expect_failure 'must be annotated' \
  bash -c 'cd "$1" && bash "$2" validate v0.9.0-ts.1 v1.0.0-ts.2 "$3"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$MAIN_COMMIT"

OUTPUT_FILE="$TEST_ROOT/github-output"
EXPECTED_FILE="$TEST_ROOT/expected-message"
RECOVERED_FILE="$TEST_ROOT/recovered-message"
RELEASE_OUTPUT_DELIMITER=RELEASE_NOTES_SAFE \
  bash -c 'cd "$1" && bash "$2" write-notes-output v1.0.0-ts.10 "$3" docs/releases "$4"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$VALIDATED_TAG_OBJECT" "$OUTPUT_FILE"
git -C "$TEST_REPO" show v1.0.0-ts.10:docs/releases/v1.0.0-ts.10.md > "$EXPECTED_FILE"
sed '1d;$d' "$OUTPUT_FILE" > "$RECOVERED_FILE"
cmp "$EXPECTED_FILE" "$RECOVERED_FILE"
[[ $(sed -n '1p' "$OUTPUT_FILE") == 'message<<RELEASE_NOTES_SAFE' ]] || \
  fail "Unexpected GitHub output header"
[[ $(tail -n 1 "$OUTPUT_FILE") == 'RELEASE_NOTES_SAFE' ]] || \
  fail "Unexpected GitHub output terminator"
[[ ! -e "$PROBE_FILE" ]] || fail "Release notes were executed as shell code"

COLLISION_OUTPUT="$TEST_ROOT/collision-output"
expect_failure 'delimiter conflicts' \
  env RELEASE_OUTPUT_DELIMITER=RELEASE_NOTES_FIXED \
  bash -c 'cd "$1" && bash "$2" write-notes-output v1.0.0-ts.10 "$3" docs/releases "$4"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$VALIDATED_TAG_OBJECT" "$COLLISION_OUTPUT"
[[ ! -e "$COLLISION_OUTPUT" ]] || fail "Collision path wrote a partial GitHub output"

MISSING_NOTES_TAG_OBJECT=$(git -C "$TEST_REPO" rev-parse v1.0.0-ts.9^{tag})
expect_failure 'Release notes are missing' \
  bash -c 'cd "$1" && bash "$2" write-notes-output v1.0.0-ts.9 "$3" docs/releases "$4"' _ \
  "$TEST_REPO" "$SAFETY_SCRIPT" "$MISSING_NOTES_TAG_OBJECT" "$TEST_ROOT/missing-notes-output"

MANIFEST_FILE="$TEST_ROOT/manifest.json"
MANIFEST_DIGEST="sha256:$(printf 'a%.0s' {1..64})"
DOCKERHUB_MANIFEST_DIGEST="sha256:$(printf 'e%.0s' {1..64})"
jq -n --arg digest "$MANIFEST_DIGEST" '{manifest: {digest: $digest, manifests: []}}' > "$MANIFEST_FILE"
COMPLETION_FILE="$TEST_ROOT/sub2api-release-complete.json"
DEPLOYER_CHECKSUMS_FILE="$TEST_ROOT/sub2api-deployer-checksums.txt"
DEPLOYER_AMD64_SHA=$(printf 'amd64 binary' | sha256sum | awk '{print $1}')
DEPLOYER_ARM64_SHA=$(printf 'arm64 binary' | sha256sum | awk '{print $1}')
DEPLOYER_BUNDLE_AMD64_SHA=$(printf 'amd64 bundle' | sha256sum | awk '{print $1}')
DEPLOYER_BUNDLE_ARM64_SHA=$(printf 'arm64 bundle' | sha256sum | awk '{print $1}')
printf '%s  %s\n' \
  "$DEPLOYER_AMD64_SHA" sub2api-deployer-linux-amd64 \
  "$DEPLOYER_ARM64_SHA" sub2api-deployer-linux-arm64 \
  "$DEPLOYER_BUNDLE_AMD64_SHA" sub2api-deployer-linux-amd64.tar.gz \
  "$DEPLOYER_BUNDLE_ARM64_SHA" sub2api-deployer-linux-arm64.tar.gz > "$DEPLOYER_CHECKSUMS_FILE"
DEPLOYER_CHECKSUMS_DIGEST="sha256:$(sha256sum "$DEPLOYER_CHECKSUMS_FILE" | awk '{print $1}')"
jq -n \
  --arg tag v1.0.0-ts.10 \
  --arg commit "$MAIN_COMMIT" \
  --arg tag_object "$VALIDATED_TAG_OBJECT" \
  --arg image ghcr.io/example/sub2api:1.0.0-ts.10 \
  --arg image_digest "$MANIFEST_DIGEST" \
  --arg dockerhub_image docker.io/example/sub2api:1.0.0-ts.10 \
  --arg dockerhub_image_digest "$DOCKERHUB_MANIFEST_DIGEST" \
  --arg deployer_checksums_sha256 "$DEPLOYER_CHECKSUMS_DIGEST" \
  --arg control_plane_manifest_sha256 "sha256:$(printf 'f%.0s' {1..64})" \
  --arg candidate_manifest_sha256 "sha256:$(printf '1%.0s' {1..64})" \
  --arg deployer_amd64_sha256 "sha256:$DEPLOYER_AMD64_SHA" \
  --arg deployer_arm64_sha256 "sha256:$DEPLOYER_ARM64_SHA" \
  --arg deployer_bundle_amd64_sha256 "sha256:$DEPLOYER_BUNDLE_AMD64_SHA" \
  --arg deployer_bundle_arm64_sha256 "sha256:$DEPLOYER_BUNDLE_ARM64_SHA" \
  '{
    schema: 3,
    tag: $tag,
    commit: $commit,
    tag_object: $tag_object,
    image: $image,
    image_digest: $image_digest,
    immutable_image: ($image + "@" + $image_digest),
    dockerhub_image: $dockerhub_image,
    dockerhub_image_digest: $dockerhub_image_digest,
    dockerhub_immutable_image: ($dockerhub_image + "@" + $dockerhub_image_digest),
    architectures: ["amd64", "arm64"],
    control_plane_manifest_sha256: $control_plane_manifest_sha256,
    candidate_manifest_sha256: $candidate_manifest_sha256,
    deployer_checksums_sha256: $deployer_checksums_sha256,
    deployer_assets: {
      "sub2api-deployer-linux-amd64": $deployer_amd64_sha256,
      "sub2api-deployer-linux-arm64": $deployer_arm64_sha256,
      "sub2api-deployer-linux-amd64.tar.gz": $deployer_bundle_amd64_sha256,
      "sub2api-deployer-linux-arm64.tar.gz": $deployer_bundle_arm64_sha256
    }
  }' > "$COMPLETION_FILE"
bash "$SAFETY_SCRIPT" verify-completion-json \
  "$COMPLETION_FILE" \
  v1.0.0-ts.10 \
  "$MAIN_COMMIT" \
  "$VALIDATED_TAG_OBJECT" \
  ghcr.io/example/sub2api:1.0.0-ts.10 \
  docker.io/example/sub2api:1.0.0-ts.10
bash "$SAFETY_SCRIPT" verify-completion-json \
  "$COMPLETION_FILE" \
  v1.0.0-ts.10 \
  "$MAIN_COMMIT" \
  "$VALIDATED_TAG_OBJECT" \
  ghcr.io/example/sub2api:1.0.0-ts.10
bash "$SAFETY_SCRIPT" verify-completion-manifest "$COMPLETION_FILE" "$MANIFEST_FILE"
bash "$SAFETY_SCRIPT" verify-completion-deployer-checksums \
  "$COMPLETION_FILE" "$DEPLOYER_CHECKSUMS_FILE"
CANDIDATE_DIR="$TEST_ROOT/candidate"
mkdir -p "$CANDIDATE_DIR"
jq '{
  schema: 1,
  version: (.tag | ltrimstr("v")),
  commit,
  image,
  candidate_image: ((.image | split(":")[0:-1] | join(":")) + ":candidate-" + .commit),
  image_digest,
  immutable_candidate_image: (((.image | split(":")[0:-1] | join(":")) + ":candidate-" + .commit) + "@" + .image_digest),
  architectures,
  control_plane_manifest_sha256,
  deployer_checksums_sha256,
  deployer_assets
}' "$COMPLETION_FILE" > "$CANDIDATE_DIR/candidate.json"
printf 'candidate manifest\n' > "$CANDIDATE_DIR/MANIFEST.sha256"
CANDIDATE_MANIFEST_DIGEST="sha256:$(sha256sum "$CANDIDATE_DIR/MANIFEST.sha256" | awk '{print $1}')"
jq --arg digest "$CANDIDATE_MANIFEST_DIGEST" '.candidate_manifest_sha256 = $digest' \
  "$COMPLETION_FILE" > "$TEST_ROOT/completion-with-candidate.json"
bash "$SAFETY_SCRIPT" verify-completion-candidate \
  "$TEST_ROOT/completion-with-candidate.json" "$CANDIDATE_DIR"
jq '.image_digest = "sha256:'"$(printf '9%.0s' {1..64})"'"' \
  "$TEST_ROOT/completion-with-candidate.json" > "$TEST_ROOT/bad-candidate-completion.json"
expect_failure 'does not match the audited Build Once candidate' bash "$SAFETY_SCRIPT" \
  verify-completion-candidate "$TEST_ROOT/bad-candidate-completion.json" "$CANDIDATE_DIR"
LEGACY_COMPLETION_FILE="$TEST_ROOT/sub2api-release-complete-schema2.json"
jq '.schema = 2 | del(.deployer_assets)' "$COMPLETION_FILE" > "$LEGACY_COMPLETION_FILE"
bash "$SAFETY_SCRIPT" verify-completion-json \
  "$LEGACY_COMPLETION_FILE" \
  v1.0.0-ts.10 \
  "$MAIN_COMMIT" \
  "$VALIDATED_TAG_OBJECT" \
  ghcr.io/example/sub2api:1.0.0-ts.10 \
  docker.io/example/sub2api:1.0.0-ts.10
bash "$SAFETY_SCRIPT" verify-completion-deployer-checksums \
  "$LEGACY_COMPLETION_FILE" "$DEPLOYER_CHECKSUMS_FILE"
jq '.architectures = ["amd64"]' "$COMPLETION_FILE" > "$TEST_ROOT/bad-completion.json"
expect_failure 'does not match' bash "$SAFETY_SCRIPT" verify-completion-json \
  "$TEST_ROOT/bad-completion.json" \
  v1.0.0-ts.10 \
  "$MAIN_COMMIT" \
  "$VALIDATED_TAG_OBJECT" \
  ghcr.io/example/sub2api:1.0.0-ts.10 \
  docker.io/example/sub2api:1.0.0-ts.10
expect_failure 'does not match' bash "$SAFETY_SCRIPT" verify-completion-json \
  "$COMPLETION_FILE" \
  v1.0.0-ts.10 \
  "$MAIN_COMMIT" \
  "$VALIDATED_TAG_OBJECT" \
  ghcr.io/example/sub2api:1.0.0-ts.10 \
  ''
CHANGED_MANIFEST_DIGEST="sha256:$(printf 'b%.0s' {1..64})"
jq -n --arg digest "$CHANGED_MANIFEST_DIGEST" '{manifest: {digest: $digest, manifests: []}}' > "$TEST_ROOT/changed-manifest.json"
expect_failure 'digest changed' bash "$SAFETY_SCRIPT" verify-completion-manifest \
  "$COMPLETION_FILE" "$TEST_ROOT/changed-manifest.json"
printf 'changed deployer checksums' > "$TEST_ROOT/changed-deployer-checksums.txt"
expect_failure 'checksum manifest changed' bash "$SAFETY_SCRIPT" \
  verify-completion-deployer-checksums \
  "$COMPLETION_FILE" "$TEST_ROOT/changed-deployer-checksums.txt"

GORELEASER_ARTIFACTS="$TEST_ROOT/artifacts.json"
jq -n --arg image ghcr.io/example/sub2api:1.0.0-ts.10 --arg digest "$MANIFEST_DIGEST" '[
  {type: "Archive", name: "sub2api.tar.gz"},
  {type: "Docker Image", name: $image, extra: {Digest: $digest}}
]' > "$GORELEASER_ARTIFACTS"
BUILT_DIGEST=$(bash "$SAFETY_SCRIPT" goreleaser-image-digest \
  "$GORELEASER_ARTIFACTS" ghcr.io/example/sub2api:1.0.0-ts.10)
[[ "$BUILT_DIGEST" == "$MANIFEST_DIGEST" ]] || fail "GoReleaser build digest was not extracted"
jq '.[1] as $artifact | . + [$artifact]' "$GORELEASER_ARTIFACTS" > "$TEST_ROOT/duplicate-artifacts.json"
expect_failure 'exactly one' bash "$SAFETY_SCRIPT" goreleaser-image-digest \
  "$TEST_ROOT/duplicate-artifacts.json" ghcr.io/example/sub2api:1.0.0-ts.10
printf '{malformed' > "$TEST_ROOT/malformed-artifacts.json"
expect_failure 'Could not extract one immutable digest' bash "$SAFETY_SCRIPT" goreleaser-image-digest \
  "$TEST_ROOT/malformed-artifacts.json" ghcr.io/example/sub2api:1.0.0-ts.10

FAKE_BIN="$TEST_ROOT/fake-bin"
mkdir -p "$FAKE_BIN"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'ref=""' \
  'for ((i=1; i<=$#; i++)); do' \
  '  if [[ "${!i}" == inspect ]]; then next=$((i+1)); ref=${!next}; break; fi' \
  'done' \
  'case "$ref" in' \
  '  *:missing) echo "manifest unknown" >&2; exit 1 ;;' \
  '  *:network-error) echo "registry timeout" >&2; exit 1 ;;' \
  '  *) echo "image exists"; exit 0 ;;' \
  'esac' > "$FAKE_BIN/docker"
chmod +x "$FAKE_BIN/docker"
PATH="$FAKE_BIN:$PATH" bash "$SAFETY_SCRIPT" assert-image-absent ghcr.io/example/sub2api:missing
expect_failure 'already exists' env PATH="$FAKE_BIN:$PATH" bash "$SAFETY_SCRIPT" \
  assert-image-absent ghcr.io/example/sub2api:existing
expect_failure 'Could not prove' env PATH="$FAKE_BIN:$PATH" bash "$SAFETY_SCRIPT" \
  assert-image-absent ghcr.io/example/sub2api:network-error

MATCHING_IMAGE_DIGEST="sha256:$(printf 'd%.0s' {1..64})"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'ref=""' \
  'for ((i=1; i<=$#; i++)); do' \
  '  if [[ "${!i}" == inspect ]]; then next=$((i+1)); ref=${!next}; break; fi' \
  'done' \
  'case "$ref" in' \
  '  *:matching) printf '\''{"manifest":{"digest":"%s"}}\n'\'' "$MATCHING_IMAGE_DIGEST"; exit 0 ;;' \
  '  *:mismatch) printf '\''{"manifest":{"digest":"sha256:%064d"}}\n'\'' 0; exit 0 ;;' \
  '  *:missing) echo "manifest unknown" >&2; exit 1 ;;' \
  '  *:network-error) echo "registry timeout" >&2; exit 1 ;;' \
  'esac' \
  'exit 1' > "$FAKE_BIN/docker"
chmod +x "$FAKE_BIN/docker"
env PATH="$FAKE_BIN:$PATH" MATCHING_IMAGE_DIGEST="$MATCHING_IMAGE_DIGEST" \
  bash "$SAFETY_SCRIPT" assert-image-digest-matches \
  ghcr.io/example/sub2api:matching "$MATCHING_IMAGE_DIGEST"
expect_failure 'does not match the recorded candidate digest' env PATH="$FAKE_BIN:$PATH" \
  MATCHING_IMAGE_DIGEST="$MATCHING_IMAGE_DIGEST" bash "$SAFETY_SCRIPT" \
  assert-image-digest-matches ghcr.io/example/sub2api:mismatch "$MATCHING_IMAGE_DIGEST"
expect_failure 'is missing' env PATH="$FAKE_BIN:$PATH" \
  MATCHING_IMAGE_DIGEST="$MATCHING_IMAGE_DIGEST" bash "$SAFETY_SCRIPT" \
  assert-image-digest-matches ghcr.io/example/sub2api:missing "$MATCHING_IMAGE_DIGEST"
expect_failure 'Could not inspect image tag' env PATH="$FAKE_BIN:$PATH" \
  MATCHING_IMAGE_DIGEST="$MATCHING_IMAGE_DIGEST" bash "$SAFETY_SCRIPT" \
  assert-image-digest-matches ghcr.io/example/sub2api:network-error "$MATCHING_IMAGE_DIGEST"
expect_failure 'Invalid expected image digest' env PATH="$FAKE_BIN:$PATH" \
  MATCHING_IMAGE_DIGEST="$MATCHING_IMAGE_DIGEST" bash "$SAFETY_SCRIPT" \
  assert-image-digest-matches ghcr.io/example/sub2api:matching invalid

PROMOTION_MANIFEST="$TEST_ROOT/promotion-manifest.json"
PROMOTION_LOG="$TEST_ROOT/promotion.log"
PROMOTION_DIGEST="sha256:$(printf 'c%.0s' {1..64})"
jq -n --arg digest "$PROMOTION_DIGEST" '{manifest: {digest: $digest, manifests: []}}' > "$PROMOTION_MANIFEST"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'if [[ "$*" == *"imagetools create"* ]]; then' \
  '  printf "%s\n" "$*" >> "$PROMOTION_LOG"' \
  '  exit 0' \
  'fi' \
  'if [[ "$*" == *"imagetools inspect"* ]]; then' \
  '  cat -- "$PROMOTION_MANIFEST"' \
  '  exit 0' \
  'fi' \
  'exit 1' > "$FAKE_BIN/docker"
chmod +x "$FAKE_BIN/docker"
env \
  PATH="$FAKE_BIN:$PATH" \
  PROMOTION_LOG="$PROMOTION_LOG" \
  PROMOTION_MANIFEST="$PROMOTION_MANIFEST" \
  bash "$SAFETY_SCRIPT" promote-image-tags \
    "ghcr.io/example/sub2api@$PROMOTION_DIGEST" \
    "$PROMOTION_DIGEST" \
    ghcr.io/example/sub2api:latest \
    ghcr.io/example/sub2api:1.2
[[ $(wc -l < "$PROMOTION_LOG") -eq 2 ]] || fail "Expected two digest-pinned image promotions"
grep -F -- "ghcr.io/example/sub2api@$PROMOTION_DIGEST" "$PROMOTION_LOG" >/dev/null || \
  fail "Mutable image tags were not promoted from the verified immutable digest"
[[ $(grep -c -- '--prefer-index=false' "$PROMOTION_LOG") -eq 2 ]] || \
  fail "Image promotion did not preserve single-manifest digests"
MISMATCHED_PROMOTION_DIGEST="sha256:$(printf 'd%.0s' {1..64})"
jq -n --arg digest "$MISMATCHED_PROMOTION_DIGEST" '{manifest: {digest: $digest, manifests: []}}' > "$PROMOTION_MANIFEST"
expect_failure 'does not resolve to the verified digest' env \
  PATH="$FAKE_BIN:$PATH" \
  PROMOTION_LOG="$PROMOTION_LOG" \
  PROMOTION_MANIFEST="$PROMOTION_MANIFEST" \
  bash "$SAFETY_SCRIPT" promote-image-tags \
    "ghcr.io/example/sub2api@$PROMOTION_DIGEST" \
    "$PROMOTION_DIGEST" \
    ghcr.io/example/sub2api:latest

TRANSACTION_STATE="$TEST_ROOT/transaction-state"
TRANSACTION_LOG="$TEST_ROOT/transaction.log"
mkdir -p "$TRANSACTION_STATE"
OLD_GHCR_DIGEST="sha256:$(printf '1%.0s' {1..64})"
OLD_DOCKERHUB_DIGEST="sha256:$(printf '2%.0s' {1..64})"
NEW_GHCR_DIGEST="sha256:$(printf '3%.0s' {1..64})"
NEW_DOCKERHUB_DIGEST="sha256:$(printf '4%.0s' {1..64})"
printf '%s\n' "$OLD_GHCR_DIGEST" > "$TRANSACTION_STATE/ghcr"
printf '%s\n' "$OLD_DOCKERHUB_DIGEST" > "$TRANSACTION_STATE/dockerhub"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'target=""' \
  'source_ref=""' \
  'for ((i=1; i<=$#; i++)); do' \
  '  arg=${!i}' \
  '  if [[ "$arg" == "--tag" ]]; then next=$((i+1)); target=${!next}; fi' \
  'done' \
  'source_ref=${*: -1}' \
  'ref=$target' \
  'if [[ "$*" == *"imagetools inspect"* ]]; then' \
  '  for ((i=1; i<=$#; i++)); do' \
  '    arg=${!i}' \
  '    if [[ "$arg" == "inspect" ]]; then next=$((i+1)); ref=${!next}; break; fi' \
  '  done' \
  'fi' \
  'case "$ref" in' \
  '  ghcr.io/*) state="$TRANSACTION_STATE/ghcr" ;;' \
  '  docker.io/*) state="$TRANSACTION_STATE/dockerhub" ;;' \
  '  *) exit 2 ;;' \
  'esac' \
  'if [[ "$*" == *"imagetools create"* ]]; then' \
  '  printf "%s\n" "$*" >> "$TRANSACTION_LOG"' \
  '  [[ "${FAIL_PROMOTE_TARGET:-}" != "$target" ]] || exit 1' \
  '  if [[ "${FAIL_ROLLBACK_TARGET:-}" == "$target" && "$source_ref" == *"@${FAIL_ROLLBACK_DIGEST:-unset}" ]]; then exit 1; fi' \
  '  printf "%s\n" "${source_ref##*@}" > "$state"' \
  '  exit 0' \
  'fi' \
  'if [[ "$*" == *"imagetools inspect"* ]]; then' \
  '  digest=$(cat "$state")' \
  '  [[ "$digest" != missing ]] || { echo "manifest unknown" >&2; exit 1; }' \
  '  jq -n --arg digest "$digest" "{manifest: {digest: \$digest, manifests: []}}"' \
  '  exit 0' \
  'fi' \
  'exit 2' > "$FAKE_BIN/docker"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'printf "%s\n" "$*" >> "$TRANSACTION_LOG"' \
  'if [[ "$*" == *"release view"* ]]; then' \
  '  if [[ -n "${GH_RELEASE_STATE:-}" ]]; then printf "%s\n" "$GH_RELEASE_STATE";' \
  '  elif [[ "${GH_EDIT_FAIL:-}" == 1 ]]; then printf "true\n";' \
  '  else printf "false\n"; fi' \
  '  exit 0' \
  'fi' \
  '[[ "${GH_EDIT_FAIL:-}" != 1 ]] || exit 1' > "$FAKE_BIN/gh"
chmod +x "$FAKE_BIN/docker" "$FAKE_BIN/gh"

expect_failure 'restoring previous latest' env \
  PATH="$FAKE_BIN:$PATH" \
  TRANSACTION_STATE="$TRANSACTION_STATE" \
  TRANSACTION_LOG="$TRANSACTION_LOG" \
  GH_EDIT_FAIL=1 \
  bash "$SAFETY_SCRIPT" publish-release-with-latest \
    v1.0.0-ts.10 ghcr.io/example/sub2api "$NEW_GHCR_DIGEST" "$OLD_GHCR_DIGEST" \
    docker.io/example/sub2api "$NEW_DOCKERHUB_DIGEST" "$OLD_DOCKERHUB_DIGEST"
[[ $(cat "$TRANSACTION_STATE/ghcr") == "$OLD_GHCR_DIGEST" ]] || fail "GHCR latest was not rolled back"
[[ $(cat "$TRANSACTION_STATE/dockerhub") == "$OLD_DOCKERHUB_DIGEST" ]] || fail "DockerHub latest was not rolled back"

: > "$TRANSACTION_LOG"
env \
  PATH="$FAKE_BIN:$PATH" \
  TRANSACTION_STATE="$TRANSACTION_STATE" \
  TRANSACTION_LOG="$TRANSACTION_LOG" \
  bash "$SAFETY_SCRIPT" publish-release-with-latest \
    v1.0.0-ts.10 ghcr.io/example/sub2api "$NEW_GHCR_DIGEST" "$OLD_GHCR_DIGEST" \
    docker.io/example/sub2api "$NEW_DOCKERHUB_DIGEST" "$OLD_DOCKERHUB_DIGEST"
[[ $(cat "$TRANSACTION_STATE/ghcr") == "$NEW_GHCR_DIGEST" ]] || fail "GHCR latest was not promoted"
[[ $(cat "$TRANSACTION_STATE/dockerhub") == "$NEW_DOCKERHUB_DIGEST" ]] || fail "DockerHub latest was not promoted"
grep -F -- 'release edit v1.0.0-ts.10 --draft=false --latest' "$TRANSACTION_LOG" >/dev/null || \
  fail "GitHub Release was not published after latest promotion"

printf '%s\n' "$OLD_GHCR_DIGEST" > "$TRANSACTION_STATE/ghcr"
printf '%s\n' "$OLD_DOCKERHUB_DIGEST" > "$TRANSACTION_STATE/dockerhub"
expect_failure 'restoring previous latest' env \
  PATH="$FAKE_BIN:$PATH" \
  TRANSACTION_STATE="$TRANSACTION_STATE" \
  TRANSACTION_LOG="$TRANSACTION_LOG" \
  FAIL_PROMOTE_TARGET=docker.io/example/sub2api:latest \
  bash "$SAFETY_SCRIPT" publish-release-with-latest \
    v1.0.0-ts.10 ghcr.io/example/sub2api "$NEW_GHCR_DIGEST" "$OLD_GHCR_DIGEST" \
    docker.io/example/sub2api "$NEW_DOCKERHUB_DIGEST" "$OLD_DOCKERHUB_DIGEST"
[[ $(cat "$TRANSACTION_STATE/ghcr") == "$OLD_GHCR_DIGEST" ]] || fail "Partial promotion did not restore GHCR latest"
[[ $(cat "$TRANSACTION_STATE/dockerhub") == "$OLD_DOCKERHUB_DIGEST" ]] || fail "Partial promotion changed DockerHub latest"

printf '%s\n' "$OLD_GHCR_DIGEST" > "$TRANSACTION_STATE/ghcr"
printf '%s\n' "$OLD_DOCKERHUB_DIGEST" > "$TRANSACTION_STATE/dockerhub"
expect_failure 'failed to restore docker.io/example/sub2api:latest' env \
  PATH="$FAKE_BIN:$PATH" \
  TRANSACTION_STATE="$TRANSACTION_STATE" \
  TRANSACTION_LOG="$TRANSACTION_LOG" \
  GH_EDIT_FAIL=1 \
  FAIL_ROLLBACK_TARGET=docker.io/example/sub2api:latest \
  FAIL_ROLLBACK_DIGEST="$OLD_DOCKERHUB_DIGEST" \
  bash "$SAFETY_SCRIPT" publish-release-with-latest \
    v1.0.0-ts.10 ghcr.io/example/sub2api "$NEW_GHCR_DIGEST" "$OLD_GHCR_DIGEST" \
    docker.io/example/sub2api "$NEW_DOCKERHUB_DIGEST" "$OLD_DOCKERHUB_DIGEST"
[[ $(cat "$TRANSACTION_STATE/ghcr") == "$OLD_GHCR_DIGEST" ]] || \
  fail "A DockerHub rollback failure prevented GHCR from being restored"
[[ $(cat "$TRANSACTION_STATE/dockerhub") == "$NEW_DOCKERHUB_DIGEST" ]] || \
  fail "Rollback failure injection did not leave DockerHub on the promoted digest"

printf '%s\n' "$OLD_GHCR_DIGEST" > "$TRANSACTION_STATE/ghcr"
printf '%s\n' "$OLD_DOCKERHUB_DIGEST" > "$TRANSACTION_STATE/dockerhub"
env \
  PATH="$FAKE_BIN:$PATH" \
  TRANSACTION_STATE="$TRANSACTION_STATE" \
  TRANSACTION_LOG="$TRANSACTION_LOG" \
  GH_EDIT_FAIL=1 \
  GH_RELEASE_STATE=false \
  bash "$SAFETY_SCRIPT" publish-release-with-latest \
    v1.0.0-ts.10 ghcr.io/example/sub2api "$NEW_GHCR_DIGEST" "$OLD_GHCR_DIGEST" \
    docker.io/example/sub2api "$NEW_DOCKERHUB_DIGEST" "$OLD_DOCKERHUB_DIGEST"
[[ $(cat "$TRANSACTION_STATE/ghcr") == "$NEW_GHCR_DIGEST" ]] || \
  fail "A server-side published release incorrectly rolled back GHCR latest"
[[ $(cat "$TRANSACTION_STATE/dockerhub") == "$NEW_DOCKERHUB_DIGEST" ]] || \
  fail "A server-side published release incorrectly rolled back DockerHub latest"

printf '%s\n' "$OLD_GHCR_DIGEST" > "$TRANSACTION_STATE/ghcr"
printf '%s\n' "$OLD_DOCKERHUB_DIGEST" > "$TRANSACTION_STATE/dockerhub"
expect_failure 'invalid publication state' env \
  PATH="$FAKE_BIN:$PATH" \
  TRANSACTION_STATE="$TRANSACTION_STATE" \
  TRANSACTION_LOG="$TRANSACTION_LOG" \
  GH_EDIT_FAIL=1 \
  GH_RELEASE_STATE=unknown \
  bash "$SAFETY_SCRIPT" publish-release-with-latest \
    v1.0.0-ts.10 ghcr.io/example/sub2api "$NEW_GHCR_DIGEST" "$OLD_GHCR_DIGEST" \
    docker.io/example/sub2api "$NEW_DOCKERHUB_DIGEST" "$OLD_DOCKERHUB_DIGEST"
[[ $(cat "$TRANSACTION_STATE/ghcr") == "$NEW_GHCR_DIGEST" ]] || \
  fail "Unknown release state incorrectly rolled back GHCR latest"
[[ $(cat "$TRANSACTION_STATE/dockerhub") == "$NEW_DOCKERHUB_DIGEST" ]] || \
  fail "Unknown release state incorrectly rolled back DockerHub latest"

DRIFTED_DIGEST="sha256:$(printf '5%.0s' {1..64})"
printf '%s\n' "$DRIFTED_DIGEST" > "$TRANSACTION_STATE/ghcr"
printf '%s\n' "$OLD_DOCKERHUB_DIGEST" > "$TRANSACTION_STATE/dockerhub"
: > "$TRANSACTION_LOG"
expect_failure 'Refusing to promote drifted tag ghcr.io/example/sub2api:latest' env \
  PATH="$FAKE_BIN:$PATH" \
  TRANSACTION_STATE="$TRANSACTION_STATE" \
  TRANSACTION_LOG="$TRANSACTION_LOG" \
  bash "$SAFETY_SCRIPT" publish-release-with-latest \
    v1.0.0-ts.10 ghcr.io/example/sub2api "$NEW_GHCR_DIGEST" "$OLD_GHCR_DIGEST" \
    docker.io/example/sub2api "$NEW_DOCKERHUB_DIGEST" "$OLD_DOCKERHUB_DIGEST"
[[ ! -s "$TRANSACTION_LOG" ]] || fail "Drift detection mutated a registry or GitHub Release"

printf 'missing\n' > "$TRANSACTION_STATE/ghcr"
printf '%s\n' "$OLD_DOCKERHUB_DIGEST" > "$TRANSACTION_STATE/dockerhub"
: > "$TRANSACTION_LOG"
expect_failure 'has no verified previous completed digest' env \
  PATH="$FAKE_BIN:$PATH" \
  TRANSACTION_STATE="$TRANSACTION_STATE" \
  TRANSACTION_LOG="$TRANSACTION_LOG" \
  bash "$SAFETY_SCRIPT" publish-release-with-latest \
    v1.0.0-ts.10 ghcr.io/example/sub2api "$NEW_GHCR_DIGEST" "$OLD_GHCR_DIGEST" \
    docker.io/example/sub2api "$NEW_DOCKERHUB_DIGEST" "$OLD_DOCKERHUB_DIGEST"
[[ ! -s "$TRANSACTION_LOG" ]] || fail "Missing latest detection mutated a registry or GitHub Release"

GOOD_IMMUTABLE_RULESET="$TEST_ROOT/good-immutable-ruleset.json"
jq -n '{
  target: "tag",
  enforcement: "active",
  conditions: {ref_name: {include: ["refs/tags/v*-ts.*"], exclude: []}},
  bypass_actors: [],
  rules: [{type: "update"}, {type: "deletion"}]
}' > "$GOOD_IMMUTABLE_RULESET"
GOOD_CREATION_RULESET="$TEST_ROOT/good-creation-ruleset.json"
jq -n '{
  target: "tag",
  enforcement: "active",
  conditions: {ref_name: {include: ["refs/tags/v*-ts.*"], exclude: []}},
  bypass_actors: [{actor_type: "DeployKey", actor_id: null, bypass_mode: "always"}],
  rules: [{type: "creation"}]
}' > "$GOOD_CREATION_RULESET"
bash "$SAFETY_SCRIPT" verify-rulesets-json "$GOOD_IMMUTABLE_RULESET" "$GOOD_CREATION_RULESET"
bash "$SAFETY_SCRIPT" verify-rulesets-runtime-json "$GOOD_IMMUTABLE_RULESET" "$GOOD_CREATION_RULESET"

REDACTED_IMMUTABLE_RULESET="$TEST_ROOT/redacted-immutable-ruleset.json"
REDACTED_CREATION_RULESET="$TEST_ROOT/redacted-creation-ruleset.json"
jq '.bypass_actors = null' "$GOOD_IMMUTABLE_RULESET" > "$REDACTED_IMMUTABLE_RULESET"
jq '.bypass_actors = null' "$GOOD_CREATION_RULESET" > "$REDACTED_CREATION_RULESET"
bash "$SAFETY_SCRIPT" verify-rulesets-runtime-json "$REDACTED_IMMUTABLE_RULESET" "$REDACTED_CREATION_RULESET"
expect_failure 'no bypass actors' bash "$SAFETY_SCRIPT" verify-rulesets-json "$REDACTED_IMMUTABLE_RULESET" "$GOOD_CREATION_RULESET"
jq '.bypass_actors = []' "$GOOD_CREATION_RULESET" > "$REDACTED_CREATION_RULESET"
bash "$SAFETY_SCRIPT" verify-rulesets-runtime-json "$GOOD_IMMUTABLE_RULESET" "$REDACTED_CREATION_RULESET"

BAD_RULESET="$TEST_ROOT/bad-ruleset.json"
jq '.bypass_actors = [{actor_type: "DeployKey", actor_id: null, bypass_mode: "always"}]' "$GOOD_IMMUTABLE_RULESET" > "$BAD_RULESET"
expect_failure 'no bypass actors' bash "$SAFETY_SCRIPT" verify-rulesets-json "$BAD_RULESET" "$GOOD_CREATION_RULESET"
expect_failure 'unexpected bypass actors at runtime' bash "$SAFETY_SCRIPT" verify-rulesets-runtime-json "$BAD_RULESET" "$GOOD_CREATION_RULESET"
jq '.rules += [{type: "creation"}]' "$GOOD_IMMUTABLE_RULESET" > "$BAD_RULESET"
expect_failure 'forbid updates and deletion' bash "$SAFETY_SCRIPT" verify-rulesets-json "$BAD_RULESET" "$GOOD_CREATION_RULESET"
expect_failure 'forbid updates and deletion' bash "$SAFETY_SCRIPT" verify-rulesets-runtime-json "$BAD_RULESET" "$GOOD_CREATION_RULESET"
jq '.bypass_actors = [{actor_type: "RepositoryRole", actor_id: 5, bypass_mode: "always"}]' "$GOOD_CREATION_RULESET" > "$BAD_RULESET"
expect_failure 'dedicated release deploy key' bash "$SAFETY_SCRIPT" verify-rulesets-json "$GOOD_IMMUTABLE_RULESET" "$BAD_RULESET"
expect_failure 'unexpected bypass actor at runtime' bash "$SAFETY_SCRIPT" verify-rulesets-runtime-json "$GOOD_IMMUTABLE_RULESET" "$BAD_RULESET"
jq '.rules += [{type: "update"}]' "$GOOD_CREATION_RULESET" > "$BAD_RULESET"
expect_failure 'must only forbid creation' bash "$SAFETY_SCRIPT" verify-rulesets-json "$GOOD_IMMUTABLE_RULESET" "$BAD_RULESET"
expect_failure 'must only forbid creation' bash "$SAFETY_SCRIPT" verify-rulesets-runtime-json "$GOOD_IMMUTABLE_RULESET" "$BAD_RULESET"

REMOTE_REPO="$TEST_ROOT/origin.git"
FETCH_REPO="$TEST_ROOT/fetch-repo"
git clone -q --bare "$TEST_REPO" "$REMOTE_REPO"
git clone -q "$REMOTE_REPO" "$FETCH_REPO"
bash -c 'cd "$1" && bash "$2" fetch origin main' _ "$FETCH_REPO" "$SAFETY_SCRIPT"
git -C "$FETCH_REPO" rev-parse --verify refs/tags/v1.0.0-ts.10 >/dev/null
git --git-dir="$REMOTE_REPO" update-ref -d refs/tags/v1.0.0-ts.10
bash -c 'cd "$1" && bash "$2" fetch origin main' _ "$FETCH_REPO" "$SAFETY_SCRIPT"
if git -C "$FETCH_REPO" rev-parse --verify --quiet refs/tags/v1.0.0-ts.10 >/dev/null; then
  fail "Pruned remote release tag remained in the local clone"
fi

echo "release safety tests passed"
