#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
CHECKER="$SCRIPT_DIR/check-release-migrations.sh"
FORK_TAG_PATTERN='^v[0-9]+\.[0-9]+\.[0-9]+-ts\.[1-9][0-9]*$'

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <immutable-baseline-ref> <release-ref> <current-release-tag>" >&2
  exit 2
fi

baseline_ref="$1"
release_ref="$2"
current_tag="$3"

if [[ ! "$current_tag" =~ $FORK_TAG_PATTERN ]]; then
  echo "invalid current fork release tag: $current_tag" >&2
  exit 2
fi

repository_root=$(git rev-parse --show-toplevel) || {
  echo "migration history check must run inside a Git repository" >&2
  exit 2
}
cd -- "$repository_root"
baseline_commit=$(git rev-parse --verify --end-of-options "${baseline_ref}^{commit}") || {
  echo "immutable migration baseline does not resolve to a commit: $baseline_ref" >&2
  exit 2
}
release_commit=$(git rev-parse --verify --end-of-options "${release_ref}^{commit}") || {
  echo "release ref does not resolve to a commit: $release_ref" >&2
  exit 2
}
git merge-base --is-ancestor "$baseline_commit" "$release_commit" || {
  echo "immutable migration baseline is not an ancestor of the release commit" >&2
  exit 2
}

"$CHECKER" "$baseline_commit" "$release_commit"

tags_path=$(mktemp "${TMPDIR:-/tmp}/sub2api-release-migration-tags.XXXXXX")
cleanup() {
  rm -f -- "$tags_path"
}
trap cleanup EXIT

git tag --merged "$release_commit" --list 'v*-ts.*' > "$tags_path"
while IFS= read -r candidate; do
  [[ -n "$candidate" && "$candidate" != "$current_tag" ]] || continue
  [[ "$candidate" =~ $FORK_TAG_PATTERN ]] || continue

  candidate_commit=$(git rev-parse --verify --end-of-options "refs/tags/${candidate}^{commit}") || {
    echo "fork release tag does not resolve to a commit: $candidate" >&2
    exit 2
  }
  git merge-base --is-ancestor "$baseline_commit" "$candidate_commit" || continue
  git merge-base --is-ancestor "$candidate_commit" "$release_commit" || continue

  "$CHECKER" "$candidate_commit" "$release_commit"
done < "$tags_path"
