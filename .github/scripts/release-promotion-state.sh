#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "$*" >&2
  exit 1
}

require_sha() {
  local value="$1"
  [[ "$value" =~ ^[0-9a-f]{40}$ ]] || \
    fail "Expected release commit must be a 40-character lowercase SHA"
}

resolve_commit() {
  git rev-parse --verify "$1^{commit}" 2>/dev/null || \
    fail "Git ref does not resolve to a commit: $1"
}

inspect() {
  local expected_sha="$1"
  local version="$2"
  local main_ref="$3"
  local remote="$4"
  local actual_sha main_sha release_tag tag_commit

  require_sha "$expected_sha"
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+-ts\.[1-9][0-9]*$ ]] || \
    fail "Release version must use X.Y.Z-ts.N with N >= 1"

  actual_sha=$(resolve_commit HEAD)
  main_sha=$(resolve_commit "$main_ref")
  [[ "$actual_sha" == "$expected_sha" ]] || \
    fail "Checkout drifted from audited SHA: $actual_sha"

  release_tag="v${version}"
  if git ls-remote --exit-code --tags "$remote" "refs/tags/${release_tag}" >/dev/null 2>&1; then
    git fetch --force "$remote" \
      "+refs/tags/${release_tag}:refs/tags/${release_tag}" >/dev/null
    git rev-parse --verify "refs/tags/${release_tag}^{tag}" >/dev/null 2>&1 || \
      fail "Existing release tag is not annotated: $release_tag"
    tag_commit=$(resolve_commit "refs/tags/${release_tag}")
    [[ "$tag_commit" == "$expected_sha" ]] || \
      fail "Existing tag $release_tag points to $tag_commit, expected $expected_sha"
    git merge-base --is-ancestor "$tag_commit" "$main_sha" || \
      fail "Existing tag $release_tag is not an ancestor of $main_ref"
    printf 'true\n'
    return
  fi

  [[ "$main_sha" == "$expected_sha" ]] || \
    fail "$main_ref moved to $main_sha before tag creation; rerun checks and audit"
  printf 'false\n'
}

require_current_main() {
  local expected_sha="$1"
  local main_ref="$2"
  local main_sha

  require_sha "$expected_sha"
  main_sha=$(resolve_commit "$main_ref")
  [[ "$main_sha" == "$expected_sha" ]] || \
    fail "$main_ref moved to $main_sha before tag creation; rerun checks and audit"
}

case "${1:-}" in
  inspect)
    [[ $# -eq 5 ]] || fail "Usage: release-promotion-state.sh inspect <sha> <version> <main-ref> <remote>"
    inspect "$2" "$3" "$4" "$5"
    ;;
  require-current-main)
    [[ $# -eq 3 ]] || fail "Usage: release-promotion-state.sh require-current-main <sha> <main-ref>"
    require_current_main "$2" "$3"
    ;;
  *)
    fail "Usage: release-promotion-state.sh <inspect|require-current-main> ..."
    ;;
esac
