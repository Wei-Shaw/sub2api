#!/usr/bin/env bash

set -euo pipefail

FORK_TAG_PATTERN='^v[0-9]+\.[0-9]+\.[0-9]+-ts\.[1-9][0-9]*$'
COMPLETION_ASSET_NAME='sub2api-release-complete.json'

fail() {
  echo "$*" >&2
  exit 1
}

require_fork_tag() {
  local release_tag="$1"
  [[ "$release_tag" =~ $FORK_TAG_PATTERN ]] || \
    fail "Invalid fork release tag: $release_tag (expected v<major>.<minor>.<patch>-ts.<revision>)"
}

require_annotated_tag() {
  local release_tag="$1"
  git rev-parse --verify --quiet --end-of-options "refs/tags/${release_tag}^{tag}" >/dev/null || \
    fail "Release tag must be annotated: $release_tag"
}

require_object_id() {
  local label="$1"
  local value="$2"
  [[ "$value" =~ ^[0-9a-fA-F]{40,64}$ ]] || fail "$label is not a valid Git object ID"
}

fetch_release_refs() {
  local remote="$1"
  local main_branch="$2"
  [[ "$remote" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || fail "Invalid Git remote name: $remote"
  git check-ref-format --branch "$main_branch" >/dev/null 2>&1 || fail "Invalid main branch name: $main_branch"
  git fetch --force --prune --prune-tags "$remote" \
    "+refs/heads/${main_branch}:refs/remotes/${remote}/${main_branch}" \
    '+refs/tags/*:refs/tags/*'
}

reachable_fork_tags() {
  local ref="$1"
  git tag --merged "$ref" --list 'v*-ts.*' |
    while IFS= read -r candidate; do
      if [[ "$candidate" =~ $FORK_TAG_PATTERN ]]; then
        printf '%s\n' "$candidate"
      fi
    done
}

latest_reachable_fork_tag() {
  reachable_fork_tags "$1" | sort -V | tail -n 1
}

validate_release() {
  local release_tag="$1"
  local previous_tag="$2"
  local main_ref="$3"
  local tag_object tag_commit checked_out_commit main_commit latest_tag
  local previous_tag_object previous_tag_commit

  require_fork_tag "$release_tag"
  require_fork_tag "$previous_tag"
  [[ "$previous_tag" != "$release_tag" ]] || fail "Previous release tag must differ from $release_tag"
  require_annotated_tag "$release_tag"
  tag_object=$(git rev-parse --verify --end-of-options "refs/tags/${release_tag}^{tag}") || \
    fail "Release tag object could not be resolved: $release_tag"
  tag_commit=$(git rev-parse --verify --end-of-options "refs/tags/${release_tag}^{commit}") || \
    fail "Release tag does not resolve to a commit: $release_tag"
  checked_out_commit=$(git rev-parse --verify HEAD)
  [[ "$tag_commit" == "$checked_out_commit" ]] || \
    fail "Checked-out commit does not match release tag $release_tag"

  main_commit=$(git rev-parse --verify --end-of-options "${main_ref}^{commit}") || \
    fail "Main ref does not resolve to a commit: $main_ref"
  git merge-base --is-ancestor "$tag_commit" "$main_commit" || \
    fail "Release tag $release_tag is not reachable from $main_ref"

  latest_tag=$(latest_reachable_fork_tag "$main_commit")
  [[ -n "$latest_tag" && "$release_tag" == "$latest_tag" ]] || \
    fail "Refusing out-of-order release for $release_tag; latest reachable fork tag is ${latest_tag:-none}"

  require_annotated_tag "$previous_tag"
  previous_tag_object=$(git rev-parse --verify --end-of-options "refs/tags/${previous_tag}^{tag}") || \
    fail "Previous release tag object could not be resolved: $previous_tag"
  previous_tag_commit=$(git rev-parse --verify --end-of-options "refs/tags/${previous_tag}^{commit}") || \
    fail "Previous release tag does not resolve to a commit: $previous_tag"
  git merge-base --is-ancestor "$previous_tag_commit" "$tag_commit" || \
    fail "Previous release tag $previous_tag is not an ancestor of $release_tag"

  printf '%s\t%s\t%s\t%s\t%s\n' \
    "$tag_commit" \
    "$tag_object" \
    "$previous_tag" \
    "$previous_tag_commit" \
    "$previous_tag_object"
}

previous_release_from_json() {
  local release_tag="$1"
  local releases_path="$2"
  local bootstrap_tag="$3"
  local candidates previous_tag

  require_fork_tag "$release_tag"
  require_fork_tag "$bootstrap_tag"
  [[ -f "$releases_path" ]] || fail "GitHub Releases JSON is missing: $releases_path"
  candidates=$(jq -r --arg completion_asset "$COMPLETION_ASSET_NAME" '
        if type != "array" then
          error("release response must be an array")
        elif length == 0 then
          empty
        elif (.[0] | type) == "array" then
          .[][]
        else
          .[]
        end
        | select((.draft // false) == false)
        | select((.prerelease // false) == false)
        | select(.published_at != null)
        | select(any(.assets[]?; .name == $completion_asset))
        | .tag_name
        | strings
        | select(length > 0)
      ' "$releases_path") || fail "Could not parse GitHub Releases while selecting the previous release"
  previous_tag=$(
	printf '%s\n%s\n' "$candidates" "$bootstrap_tag" |
      while IFS= read -r candidate; do
        if [[ "$candidate" =~ $FORK_TAG_PATTERN && "$candidate" != "$release_tag" ]]; then
          printf '%s\n' "$candidate"
        fi
      done |
      sort -Vu |
      tail -n 1
  )
  [[ -n "$previous_tag" ]] || fail "No previous published fork release was found for $release_tag"
  printf '%s\n' "$previous_tag"
}

assert_release_unpublished_json() {
  local release_tag="$1"
  local releases_path="$2"
  local matching_tag

  require_fork_tag "$release_tag"
  [[ -f "$releases_path" ]] || fail "GitHub Releases JSON is missing: $releases_path"
  matching_tag=$(
    jq -r --arg release_tag "$release_tag" '
      if type != "array" then
        error("release response must be an array")
      elif length == 0 then
        empty
      elif (.[0] | type) == "array" then
        .[][]
      else
        .[]
      end
      | .tag_name
      | strings
      | select(. == $release_tag)
    ' "$releases_path" | head -n 1
  ) || fail "Could not parse GitHub Releases while checking $release_tag"
  [[ -z "$matching_tag" ]] || fail "Release $release_tag already exists; use a new immutable fork tag instead of overwriting artifacts"
}

assert_release_not_published_json() {
  local release_tag="$1"
  local releases_path="$2"
  local published_count

  require_fork_tag "$release_tag"
  [[ -f "$releases_path" ]] || fail "GitHub Releases JSON is missing: $releases_path"
  published_count=$(jq -er --arg release_tag "$release_tag" '
    if type != "array" then error("release response must be an array")
    elif length == 0 then []
    elif (.[0] | type) == "array" then [ .[][] ]
    else .
    end
    | [ .[] | select(.tag_name == $release_tag and (.draft // false) == false) ]
    | length
  ' "$releases_path") || fail "Could not parse GitHub Releases while checking $release_tag"
  [[ "$published_count" == 0 ]] || fail "Release $release_tag is already published"
}

verify_version_file() {
  local release_tag="$1"
  local version_path="$2"
  local expected_version actual_version

  require_fork_tag "$release_tag"
  [[ -f "$version_path" ]] || fail "VERSION file is missing: $version_path"
  expected_version=${release_tag#v}
  actual_version=$(cat -- "$version_path")
  [[ "$actual_version" == "$expected_version" ]] || \
    fail "VERSION file must equal $expected_version before creating tag $release_tag (got ${actual_version:-empty})"
}

verify_completion_json() {
  local completion_path="$1"
  local expected_tag="$2"
  local expected_commit="$3"
  local expected_tag_object="$4"
  local expected_image="$5"
  local expected_dockerhub_image="${6:-}"
  local dockerhub_mode=optional

  if [[ $# -ge 6 ]]; then
    dockerhub_mode=exact
  fi

  require_fork_tag "$expected_tag"
  require_object_id "Expected release commit" "$expected_commit"
  require_object_id "Expected release tag object" "$expected_tag_object"
  [[ -f "$completion_path" ]] || fail "Release completion marker is missing: $completion_path"
  jq -e \
    --arg tag "$expected_tag" \
    --arg commit "$expected_commit" \
    --arg tag_object "$expected_tag_object" \
    --arg image "$expected_image" \
    --arg dockerhub_image "$expected_dockerhub_image" \
    --arg dockerhub_mode "$dockerhub_mode" '
      (.schema == 2 or .schema == 3)
      and .tag == $tag
      and .commit == $commit
      and .tag_object == $tag_object
      and .image == $image
      and (.image_digest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
      and .immutable_image == ($image + "@" + .image_digest)
      and ((.architectures | sort) == ["amd64", "arm64"])
      and (
        ((.control_plane_manifest_sha256 // null) == null and (.candidate_manifest_sha256 // null) == null)
        or
        ((.control_plane_manifest_sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$"))
          and (.candidate_manifest_sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$")))
      )
      and (.deployer_checksums_sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$"))
      and (
        if .schema == 3 then
          (.deployer_assets | type == "object")
          and ((.deployer_assets | keys | sort) == [
            "sub2api-deployer-linux-amd64",
            "sub2api-deployer-linux-amd64.tar.gz",
            "sub2api-deployer-linux-arm64",
            "sub2api-deployer-linux-arm64.tar.gz"
          ])
          and ([.deployer_assets[] | type == "string" and test("^sha256:[0-9a-f]{64}$")] | all)
        else
          (.deployer_assets == null)
        end
      )
      and (
        if $dockerhub_mode == "optional" then
          (
            (.dockerhub_image == null
              and .dockerhub_image_digest == null
              and .dockerhub_immutable_image == null)
            or
            ((.dockerhub_image | type == "string" and length > 0)
              and (.dockerhub_image_digest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
              and .dockerhub_immutable_image == (.dockerhub_image + "@" + .dockerhub_image_digest))
          )
        elif $dockerhub_image == "" then
          .dockerhub_image == null
          and .dockerhub_image_digest == null
          and .dockerhub_immutable_image == null
        else
          .dockerhub_image == $dockerhub_image
          and (.dockerhub_image_digest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
          and .dockerhub_immutable_image == ($dockerhub_image + "@" + .dockerhub_image_digest)
        end
      )
    ' "$completion_path" >/dev/null || fail "Release completion marker does not match the immutable release metadata"
}

verify_completion_deployer_checksums() {
  local completion_path="$1"
  local checksums_path="$2"
  local expected_digest actual_digest

  [[ -f "$completion_path" ]] || fail "Release completion marker is missing: $completion_path"
  [[ -f "$checksums_path" ]] || fail "Deployer checksum manifest is missing: $checksums_path"
  expected_digest=$(jq -er '.deployer_checksums_sha256 | select(type == "string")' "$completion_path") || \
    fail "Release completion marker has no deployer checksum digest"
  actual_digest="sha256:$(sha256sum "$checksums_path" | awk '{print $1}')"
  [[ "$actual_digest" == "$expected_digest" ]] || \
    fail "Deployer checksum manifest changed after release completion (expected $expected_digest, got $actual_digest)"

  if [[ $(jq -er '.schema' "$completion_path") == 3 ]]; then
    local asset filename manifest_digest ledger_digest
    for asset in \
      sub2api-deployer-linux-amd64 \
      sub2api-deployer-linux-arm64 \
      sub2api-deployer-linux-amd64.tar.gz \
      sub2api-deployer-linux-arm64.tar.gz; do
      filename=$asset
      manifest_digest=$(awk -v name="$filename" '$2 == name || $2 == "*" name {print "sha256:" $1; exit}' "$checksums_path")
      [[ "$manifest_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "Deployer checksum manifest has no digest for $filename"
      ledger_digest=$(jq -er --arg asset "$asset" '.deployer_assets[$asset] | select(type == "string")' "$completion_path") || \
        fail "Release completion marker has no deployer asset digest for $asset"
      [[ "$ledger_digest" == "$manifest_digest" ]] || \
        fail "Deployer asset digest for $asset does not match the checksum manifest"
    done
  fi
}

verify_completion_manifest() {
  local completion_path="$1"
  local manifest_path="$2"
  local expected_digest actual_digest

  [[ -f "$completion_path" ]] || fail "Release completion marker is missing: $completion_path"
  [[ -f "$manifest_path" ]] || fail "OCI manifest is missing: $manifest_path"
  expected_digest=$(jq -er '.image_digest | select(type == "string")' "$completion_path") || \
    fail "Release completion marker has no image digest"
	actual_digest=$(jq -er '.manifest.digest | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))' "$manifest_path") || \
		fail "OCI image inspection has no valid manifest digest"
  [[ "$actual_digest" == "$expected_digest" ]] || \
    fail "Published image digest changed after release completion (expected $expected_digest, got $actual_digest)"
}

verify_completion_candidate() {
  local completion_path="$1"
  local candidate_dir="$2"
  local candidate_path="$candidate_dir/candidate.json"
  local candidate_manifest="$candidate_dir/MANIFEST.sha256"
  local candidate_manifest_digest

  [[ -f "$completion_path" ]] || fail "Release completion marker is missing: $completion_path"
  [[ -f "$candidate_path" && -f "$candidate_manifest" ]] || fail "Release candidate metadata is incomplete: $candidate_dir"
  candidate_manifest_digest="sha256:$(sha256sum "$candidate_manifest" | awk '{print $1}')"
  jq -e \
    --slurpfile candidate "$candidate_path" \
    --arg candidate_manifest_digest "$candidate_manifest_digest" '
      ($candidate[0]) as $c
      | .schema == 3
        and .commit == $c.commit
        and .image == $c.image
        and .image_digest == $c.image_digest
        and .immutable_image == ($c.image + "@" + $c.image_digest)
        and ((.architectures | sort) == ($c.architectures | sort))
        and .control_plane_manifest_sha256 == $c.control_plane_manifest_sha256
        and .candidate_manifest_sha256 == $candidate_manifest_digest
        and .deployer_checksums_sha256 == $c.deployer_checksums_sha256
        and .deployer_assets == $c.deployer_assets
    ' "$completion_path" >/dev/null || fail "Release completion marker does not match the audited Build Once candidate"
}

goreleaser_image_digest() {
  local artifacts_path="$1"
  local expected_image="$2"

  [[ -f "$artifacts_path" ]] || fail "GoReleaser artifacts metadata is missing: $artifacts_path"
  [[ "$expected_image" =~ ^[a-z0-9][a-z0-9._/-]*:[A-Za-z0-9._-]+$ ]] || fail "Invalid image reference: $expected_image"
  jq -er --arg image "$expected_image" '
    if type != "array" then
      error("GoReleaser artifacts metadata must be an array")
    else
      [.[]
        | select(.type == "Docker Image" and .name == $image)
        | .extra.Digest
        | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))]
      | if length == 1 then .[0]
        else error("expected exactly one matching Docker Image artifact")
        end
    end
  ' "$artifacts_path" || fail "Could not extract one immutable digest for $expected_image from GoReleaser artifacts"
}

assert_image_absent() {
  local image_ref="$1"
  local output

  [[ "$image_ref" =~ ^[a-z0-9][a-z0-9._/-]*:[A-Za-z0-9._-]+$ ]] || fail "Invalid image reference: $image_ref"
  if output=$(docker buildx imagetools inspect "$image_ref" 2>&1); then
    fail "Container image $image_ref already exists; use a new immutable fork tag instead of overwriting it"
  fi
  case "$output" in
    *"not found"*|*"manifest unknown"*) ;;
    *) fail "Could not prove container image $image_ref is absent: $output" ;;
  esac
}

assert_image_digest_matches() {
  local image_ref="$1"
  local expected_digest="$2"
  local actual_digest

  [[ "$expected_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "Invalid expected image digest: $expected_digest"
  if ! actual_digest=$(image_tag_digest_or_missing "$image_ref"); then
    exit 1
  fi
  [[ "$actual_digest" != missing ]] || \
    fail "Container image $image_ref is missing; it cannot be reused as the recorded candidate"
  [[ "$actual_digest" == "$expected_digest" ]] || \
    fail "Container image $image_ref does not match the recorded candidate digest (expected $expected_digest, got $actual_digest)"
}

promote_image_tags() {
  local immutable_image="$1"
  local expected_digest="$2"
  shift 2
  local target manifest_path actual_digest

  [[ "$immutable_image" =~ ^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$ ]] || \
    fail "Invalid immutable image reference: $immutable_image"
  [[ "$expected_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "Invalid expected image digest: $expected_digest"
  [[ "$immutable_image" == *"@$expected_digest" ]] || \
    fail "Immutable image reference does not match expected digest"
  [[ $# -gt 0 ]] || fail "At least one mutable image tag is required"

  manifest_path=$(mktemp "${TMPDIR:-/tmp}/sub2api-promoted-manifest.XXXXXX")
  for target in "$@"; do
    [[ "$target" =~ ^[a-z0-9][a-z0-9._/-]*:[A-Za-z0-9._-]+$ ]] || fail "Invalid mutable image tag: $target"
    docker buildx imagetools create --prefer-index=false --tag "$target" "$immutable_image"
    docker buildx imagetools inspect "$target" --format '{{json .}}' > "$manifest_path"
    actual_digest=$(jq -er '.manifest.digest | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))' "$manifest_path") || \
      fail "Promoted image tag $target has no valid registry manifest digest"
    [[ "$actual_digest" == "$expected_digest" ]] || \
      fail "Promoted image tag $target does not resolve to the verified digest (expected $expected_digest, got $actual_digest)"
  done
  rm -f -- "$manifest_path"
}

image_tag_digest_or_missing() {
  local image_ref="$1"
  local output digest

  [[ "$image_ref" =~ ^[a-z0-9][a-z0-9._/-]*:[A-Za-z0-9._-]+$ ]] || fail "Invalid image reference: $image_ref"
  if output=$(docker buildx imagetools inspect "$image_ref" --format '{{json .}}' 2>&1); then
    digest=$(jq -er '.manifest.digest | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))' <<<"$output") || \
      fail "Image tag $image_ref has no valid registry manifest digest"
    printf '%s\n' "$digest"
    return
  fi
  case "$output" in
    *"not found"*|*"manifest unknown"*) printf '%s\n' missing ;;
    *) fail "Could not inspect image tag $image_ref: $output" ;;
  esac
}

publish_release_with_latest() (
  local release_tag="$1"
  local ghcr_repository="$2"
  local ghcr_digest="$3"
  local previous_ghcr_digest="$4"
  local dockerhub_repository="${5:-}"
  local dockerhub_digest="${6:-}"
  local previous_dockerhub_digest="${7:-}"
  local release_published=0
  local release_state
  local old_digest target repository new_digest expected_old_digest
  local -a targets=()
  local -a expected_old_digests=("$previous_ghcr_digest")
  local -a rollback_targets=()
  local -a rollback_digests=()
  local -a repositories=("$ghcr_repository")
  local -a new_digests=("$ghcr_digest")

  require_fork_tag "$release_tag"
  [[ "$ghcr_repository" =~ ^[a-z0-9][a-z0-9._/-]*$ ]] || fail "Invalid GHCR repository: $ghcr_repository"
  [[ "$ghcr_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "Invalid GHCR release digest"
  [[ "$previous_ghcr_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "Invalid previous completed GHCR digest"
  if [[ -n "$dockerhub_repository" || -n "$dockerhub_digest" || -n "$previous_dockerhub_digest" ]]; then
    [[ -n "$dockerhub_repository" && -n "$dockerhub_digest" && -n "$previous_dockerhub_digest" ]] || \
      fail "DockerHub repository, release digest, and previous completed digest must be provided together"
    [[ "$dockerhub_repository" =~ ^[a-z0-9][a-z0-9._/-]*$ ]] || fail "Invalid DockerHub repository: $dockerhub_repository"
    [[ "$dockerhub_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "Invalid DockerHub release digest"
    [[ "$previous_dockerhub_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "Invalid previous completed DockerHub digest"
    repositories+=("$dockerhub_repository")
    new_digests+=("$dockerhub_digest")
    expected_old_digests+=("$previous_dockerhub_digest")
  fi

  rollback_latest_tags() {
    local status=$?
    local i
    if [[ $release_published -eq 0 && ${#rollback_targets[@]} -gt 0 ]]; then
      set +e
      echo "Release publication failed; restoring previous latest image tags" >&2
      for ((i=${#rollback_targets[@]}-1; i>=0; i--)); do
        ( promote_image_tags \
            "${rollback_targets[$i]%:*}@${rollback_digests[$i]}" \
            "${rollback_digests[$i]}" \
            "${rollback_targets[$i]}" ) || \
          echo "CRITICAL: failed to restore ${rollback_targets[$i]} to ${rollback_digests[$i]}" >&2
      done
    fi
    exit "$status"
  }
  trap rollback_latest_tags EXIT

  # Prove every mutable tag still points at the last completed release before
  # changing any registry. A drifted tag is not a trustworthy rollback target.
  local i
  for ((i=0; i<${#repositories[@]}; i++)); do
    repository=${repositories[$i]}
    expected_old_digest=${expected_old_digests[$i]}
    target="${repository}:latest"
    old_digest=$(image_tag_digest_or_missing "$target")
    [[ "$old_digest" != missing ]] || \
      fail "Refusing to promote $target because it has no verified previous completed digest to restore"
    [[ "$old_digest" == "$expected_old_digest" ]] || \
      fail "Refusing to promote drifted tag $target (expected previous completed digest $expected_old_digest, got $old_digest)"
    targets+=("$target")
  done

  for ((i=0; i<${#targets[@]}; i++)); do
    target=${targets[$i]}
    repository=${target%:*}
    new_digest=${ghcr_digest}
    if [[ -n "$dockerhub_repository" && "$repository" == "$dockerhub_repository" ]]; then
      new_digest=$dockerhub_digest
    fi
    # Register the target before mutation so an ambiguous create/verify
    # failure still restores the last completed release digest.
    rollback_targets+=("$target")
    rollback_digests+=("${expected_old_digests[$i]}")
    promote_image_tags "${repository}@${new_digest}" "$new_digest" "$target"
  done

  # The publication request can succeed server-side while the CLI reports a
  # transport failure. Query the resulting state before deciding whether the
  # mutable tags are still rollback candidates.
  gh release edit "$release_tag" --draft=false --latest || true
  if release_state=$(gh release view "$release_tag" --json isDraft --jq '.isDraft' 2>/dev/null); then
    case "$release_state" in
      false)
        release_published=1
        ;;
      true)
        fail "GitHub Release $release_tag is still a draft after publication"
        ;;
      *)
        release_published=1
        fail "CRITICAL: GitHub Release $release_tag returned an invalid publication state; preserving promoted latest tags"
        ;;
    esac
  else
    release_published=1
    fail "CRITICAL: GitHub Release $release_tag publication state is unknown; preserving promoted latest tags"
  fi
)

verify_tag_commit() {
  local release_tag="$1"
  local expected_commit="$2"
  local expected_tag_object="$3"
  local expected_previous_tag="$4"
  local expected_previous_commit="$5"
  local expected_previous_tag_object="$6"
  local main_ref="$7"
  local metadata
  local tag_commit tag_object previous_tag previous_tag_commit previous_tag_object

  require_fork_tag "$release_tag"
  require_fork_tag "$expected_previous_tag"
  require_object_id "Validated release commit" "$expected_commit"
  require_object_id "Validated release tag object" "$expected_tag_object"
  require_object_id "Validated previous release commit" "$expected_previous_commit"
  require_object_id "Validated previous release tag object" "$expected_previous_tag_object"

  metadata=$(validate_release "$release_tag" "$expected_previous_tag" "$main_ref")
  IFS=$'\t' read -r tag_commit tag_object previous_tag previous_tag_commit previous_tag_object <<<"$metadata"
  [[ "$tag_object" == "$expected_tag_object" ]] || \
    fail "Release tag object $release_tag moved after validation"
  [[ "$tag_commit" == "$expected_commit" ]] || \
    fail "Release tag $release_tag moved after validation"
  [[ "$previous_tag" == "$expected_previous_tag" ]] || \
    fail "Previous release tag changed after validation: expected $expected_previous_tag, got $previous_tag"
  [[ "$previous_tag_commit" == "$expected_previous_commit" ]] || \
    fail "Previous release tag $expected_previous_tag moved after validation"
  [[ "$previous_tag_object" == "$expected_previous_tag_object" ]] || \
    fail "Previous release tag object $expected_previous_tag moved after validation"
}

verify_ruleset_structure_json() {
  local immutable_ruleset_path="$1"
  local creation_ruleset_path="$2"
  [[ -f "$immutable_ruleset_path" ]] || \
    fail "Immutable release tag ruleset JSON is missing: $immutable_ruleset_path"
  [[ -f "$creation_ruleset_path" ]] || \
    fail "Release tag creation ruleset JSON is missing: $creation_ruleset_path"

  jq -e --arg pattern 'refs/tags/v*-ts.*' '
    (.conditions.ref_name.include // []) as $includes
    | ([.rules[]?.type]) as $rules
    | .target == "tag"
      and .enforcement == "active"
      and (($includes | index($pattern)) != null or ($includes | index("~ALL")) != null)
      and ((.conditions.ref_name.exclude // []) | length == 0)
      and (($rules | index("update")) != null)
      and (($rules | index("deletion")) != null)
      and (($rules | index("creation")) == null)
  ' "$immutable_ruleset_path" >/dev/null || \
    fail "Immutable release tag ruleset must only forbid updates and deletion for refs/tags/v*-ts.*"

  jq -e --arg pattern 'refs/tags/v*-ts.*' '
    (.conditions.ref_name.include // []) as $includes
    | ([.rules[]?.type]) as $rules
    | .target == "tag"
      and .enforcement == "active"
      and (($includes | index($pattern)) != null or ($includes | index("~ALL")) != null)
      and ((.conditions.ref_name.exclude // []) | length == 0)
      and (($rules | index("creation")) != null)
      and (($rules | index("update")) == null)
      and (($rules | index("deletion")) == null)
  ' "$creation_ruleset_path" >/dev/null || \
    fail "Release tag creation ruleset must only forbid creation for refs/tags/v*-ts.*"
}

verify_rulesets_json() {
  local immutable_ruleset_path="$1"
  local creation_ruleset_path="$2"
  verify_ruleset_structure_json "$immutable_ruleset_path" "$creation_ruleset_path"

  jq -e '(.bypass_actors | type) == "array" and (.bypass_actors | length) == 0' "$immutable_ruleset_path" >/dev/null || \
    fail "Immutable release tag ruleset must have no bypass actors"
  jq -e '
    (.bypass_actors // []) as $bypass
    | ($bypass | length == 1)
      and ($bypass[0].actor_type == "DeployKey")
      and ($bypass[0].actor_id == null)
      and ($bypass[0].bypass_mode == "always")
  ' "$creation_ruleset_path" >/dev/null || \
    fail "Release tag creation ruleset must use the dedicated release deploy key as its sole bypass"
}

verify_rulesets_runtime_json() {
  local immutable_ruleset_path="$1"
  local creation_ruleset_path="$2"
  verify_ruleset_structure_json "$immutable_ruleset_path" "$creation_ruleset_path"

  # GITHUB_TOKEN ruleset responses can redact bypass actors. Reject any
  # visible actor unless it matches the operator-audited release contract.
  jq -e '
    (.bypass_actors? // null) as $bypass
    | ($bypass == null)
      or ((($bypass | type) == "array") and (($bypass | length) == 0))
  ' "$immutable_ruleset_path" >/dev/null || \
    fail "Immutable release tag ruleset exposed unexpected bypass actors at runtime"
  jq -e '
    (.bypass_actors? // null) as $bypass
    | ($bypass == null)
      or ((($bypass | type) == "array") and (
        (($bypass | length) == 0)
        or (
          ($bypass | length) == 1
          and $bypass[0].actor_type == "DeployKey"
          and $bypass[0].actor_id == null
          and $bypass[0].bypass_mode == "always"
        )
      ))
  ' "$creation_ruleset_path" >/dev/null || \
    fail "Release tag creation ruleset exposed an unexpected bypass actor at runtime"
}

write_release_notes_output() {
  local release_tag="$1"
  local expected_tag_object="$2"
  local notes_directory="$3"
  local output_path="$4"
  local tag_object notes_path release_notes delimiter

  require_fork_tag "$release_tag"
  require_annotated_tag "$release_tag"
  require_object_id "Validated release tag object" "$expected_tag_object"
  [[ "$notes_directory" =~ ^[A-Za-z0-9._/-]+$ ]] || fail "Release notes directory is invalid"
  [[ "$notes_directory" != /* && "$notes_directory" != *'..'* ]] || fail "Release notes directory must be a safe repository-relative path"
  [[ -n "$output_path" ]] || fail "GitHub output path is empty"
  tag_object=$(git rev-parse --verify --end-of-options "refs/tags/${release_tag}^{tag}") || \
    fail "Release tag object could not be resolved: $release_tag"
  [[ "$tag_object" == "$expected_tag_object" ]] || fail "Release tag object $release_tag moved before reading release notes"

  notes_path="${notes_directory%/}/${release_tag}.md"
  git cat-file -e "${release_tag}^{commit}:${notes_path}" 2>/dev/null || \
    fail "Release notes are missing from $release_tag: $notes_path"
  release_notes=$(git show "${release_tag}^{commit}:${notes_path}") || \
    fail "Could not read release notes from $release_tag: $notes_path"
  delimiter=${RELEASE_OUTPUT_DELIMITER:-}
  if [[ -z "$delimiter" ]]; then
    delimiter="RELEASE_NOTES_$(openssl rand -hex 24)"
  fi
  [[ "$delimiter" =~ ^[A-Za-z0-9_]+$ ]] || fail "GitHub output delimiter is invalid"
  if grep -Fqx -- "$delimiter" <<<"$release_notes"; then
    fail "GitHub output delimiter conflicts with the release notes"
  fi

  {
    printf 'message<<%s\n' "$delimiter"
    printf '%s\n' "$release_notes"
    printf '%s\n' "$delimiter"
  } >> "$output_path"
  echo "Release notes length: ${#release_notes}"
}

usage() {
  cat >&2 <<'EOF'
Usage:
  release-safety.sh fetch <remote> <main-branch>
  release-safety.sh previous-release-json <release-tag> <github-releases-json> <bootstrap-tag>
  release-safety.sh assert-unpublished-json <release-tag> <github-releases-json>
  release-safety.sh assert-not-published-json <release-tag> <github-releases-json>
  release-safety.sh verify-version-file <release-tag> <version-file>
  release-safety.sh verify-completion-json <completion-json> <release-tag> <commit> <tag-object> <ghcr-image-ref> [<dockerhub-image-ref>]
  release-safety.sh verify-completion-manifest <completion-json> <oci-manifest-json>
  release-safety.sh verify-completion-candidate <completion-json> <candidate-directory>
  release-safety.sh verify-completion-deployer-checksums <completion-json> <deployer-checksums>
  release-safety.sh goreleaser-image-digest <artifacts-json> <image-ref>
  release-safety.sh assert-image-absent <image-ref>
  release-safety.sh assert-image-digest-matches <image-ref> <sha256-digest>
  release-safety.sh promote-image-tags <immutable-image@digest> <digest> <target-tag> [<target-tag>...]
  release-safety.sh image-tag-digest-or-missing <image-tag>
  release-safety.sh publish-release-with-latest <release-tag> <ghcr-repository> <ghcr-digest> <previous-ghcr-digest> [<dockerhub-repository> <dockerhub-digest> <previous-dockerhub-digest>]
  release-safety.sh validate <release-tag> <previous-release-tag> <main-ref>
  release-safety.sh verify-tag <release-tag> <validated-commit> <validated-tag-object> <previous-tag> <previous-commit> <previous-tag-object> <main-ref>
  release-safety.sh verify-rulesets-json <immutable-ruleset-json> <creation-ruleset-json>
  release-safety.sh verify-rulesets-runtime-json <immutable-ruleset-json> <creation-ruleset-json>
  release-safety.sh write-notes-output <release-tag> <validated-tag-object> <notes-directory> <github-output-path>
EOF
  exit 2
}

case "${1:-}" in
  fetch)
    [[ $# -eq 3 ]] || usage
    fetch_release_refs "$2" "$3"
    ;;
  validate)
    [[ $# -eq 4 ]] || usage
    validate_release "$2" "$3" "$4"
    ;;
  previous-release-json)
    [[ $# -eq 4 ]] || usage
    previous_release_from_json "$2" "$3" "$4"
    ;;
  assert-unpublished-json)
    [[ $# -eq 3 ]] || usage
    assert_release_unpublished_json "$2" "$3"
    ;;
  assert-not-published-json)
    [[ $# -eq 3 ]] || usage
    assert_release_not_published_json "$2" "$3"
    ;;
  verify-version-file)
    [[ $# -eq 3 ]] || usage
    verify_version_file "$2" "$3"
    ;;
  verify-completion-json)
    [[ $# -eq 6 || $# -eq 7 ]] || usage
    verify_completion_json "${@:2}"
    ;;
  verify-completion-manifest)
    [[ $# -eq 3 ]] || usage
    verify_completion_manifest "$2" "$3"
    ;;
  verify-completion-candidate)
    [[ $# -eq 3 ]] || usage
    verify_completion_candidate "$2" "$3"
    ;;
  verify-completion-deployer-checksums)
    [[ $# -eq 3 ]] || usage
    verify_completion_deployer_checksums "$2" "$3"
    ;;
  goreleaser-image-digest)
    [[ $# -eq 3 ]] || usage
    goreleaser_image_digest "$2" "$3"
    ;;
  assert-image-absent)
    [[ $# -eq 2 ]] || usage
    assert_image_absent "$2"
    ;;
  assert-image-digest-matches)
    [[ $# -eq 3 ]] || usage
    assert_image_digest_matches "$2" "$3"
    ;;
  promote-image-tags)
    [[ $# -ge 4 ]] || usage
    promote_image_tags "${@:2}"
    ;;
  image-tag-digest-or-missing)
    [[ $# -eq 2 ]] || usage
    image_tag_digest_or_missing "$2"
    ;;
  publish-release-with-latest)
    [[ $# -eq 5 || $# -eq 8 ]] || usage
    publish_release_with_latest "${@:2}"
    ;;
  verify-tag)
    [[ $# -eq 8 ]] || usage
    verify_tag_commit "$2" "$3" "$4" "$5" "$6" "$7" "$8"
    ;;
  verify-rulesets-json)
    [[ $# -eq 3 ]] || usage
    verify_rulesets_json "$2" "$3"
    ;;
  verify-rulesets-runtime-json)
    [[ $# -eq 3 ]] || usage
    verify_rulesets_runtime_json "$2" "$3"
    ;;
  write-notes-output)
    [[ $# -eq 5 ]] || usage
    write_release_notes_output "$2" "$3" "$4" "$5"
    ;;
  *)
    usage
    ;;
esac
