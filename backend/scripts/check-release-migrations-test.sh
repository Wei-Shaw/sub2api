#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
BACKEND_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
CHECKER="$SCRIPT_DIR/check-release-migrations.sh"
HISTORY_CHECKER="$SCRIPT_DIR/check-release-migrations-history.sh"
FIXTURES="$SCRIPT_DIR/testdata/release-migrations"
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-migration-gate-test.XXXXXX")
CHECKER_BIN="$TEST_ROOT/check-release-migrations"

cleanup() {
  rm -rf -- "$TEST_ROOT"
}
trap cleanup EXIT

(cd -- "$BACKEND_ROOT" && go build -trimpath -o "$CHECKER_BIN" ./cmd/check-release-migrations)
export SUB2API_MIGRATION_CHECKER_BIN="$CHECKER_BIN"

new_repo() {
  local name="$1"
  local repo="$TEST_ROOT/$name"
  mkdir -p "$repo/backend/migrations"
  git -C "$repo" init -q
  git -C "$repo" config user.email test@example.invalid
  git -C "$repo" config user.name migration-gate-test
  printf '%s\n' 'CREATE TABLE public.baseline (id bigint PRIMARY KEY);' > "$repo/backend/migrations/001_baseline.sql"
  git -C "$repo" add .
  git -C "$repo" commit -qm baseline
  printf '%s\n' "$repo"
}

expect_allowed() {
  local label="$1"
  local repo="$2"
  local base="$3"
  local target="$4"
  local output
  if ! output=$(cd -- "$repo" && "$CHECKER" "$base" "$target" 2>&1); then
    echo "expected migration check to pass: $label" >&2
    echo "$output" >&2
    exit 1
  fi
}

expect_rejected() {
  local label="$1"
  local repo="$2"
  local base="$3"
  local target="$4"
  local output
  if output=$(cd -- "$repo" && "$CHECKER" "$base" "$target" 2>&1); then
    echo "expected migration check to fail: $label" >&2
    echo "$output" >&2
    exit 1
  fi
}

assert_fixture_allowed() {
  local fixture="$1"
  local repo
  repo=$(new_repo "allowed-${fixture%.sql}")
  cp -- "$FIXTURES/$fixture" "$repo/backend/migrations/002_test.sql"
  git -C "$repo" add .
  git -C "$repo" commit -qm candidate
  expect_allowed "$fixture" "$repo" HEAD^ HEAD
}

assert_fixture_rejected() {
  local fixture="$1"
  local repo
  repo=$(new_repo "rejected-${fixture%.sql}")
  cp -- "$FIXTURES/$fixture" "$repo/backend/migrations/002_test.sql"
  git -C "$repo" add .
  git -C "$repo" commit -qm candidate
  expect_rejected "$fixture" "$repo" HEAD^ HEAD
}

assert_fixture_allowed_as() {
  local fixture="$1"
  local target_name="$2"
  local repo
  repo=$(new_repo "allowed-${fixture%.sql}-${target_name%.sql}")
  cp -- "$FIXTURES/$fixture" "$repo/backend/migrations/$target_name"
  git -C "$repo" add .
  git -C "$repo" commit -qm candidate
  expect_allowed "$fixture as $target_name" "$repo" HEAD^ HEAD
}

assert_fixture_rejected_as() {
  local fixture="$1"
  local target_name="$2"
  local repo
  repo=$(new_repo "rejected-${fixture%.sql}-${target_name%.sql}")
  cp -- "$FIXTURES/$fixture" "$repo/backend/migrations/$target_name"
  git -C "$repo" add .
  git -C "$repo" commit -qm candidate
  expect_rejected "$fixture as $target_name" "$repo" HEAD^ HEAD
}

for fixture in \
  additive.sql \
  comments-only.sql \
  create-type.sql \
  insert-conflict-nothing.sql \
  quoted-content.sql \
  reviewed-behavior.sql \
  unique-new-table.sql \
  unique-new-table-qualified.sql; do
  assert_fixture_allowed "$fixture"
done

assert_fixture_allowed_as index-existing-concurrent.sql 002_index_existing_notx.sql
assert_fixture_rejected_as index-existing-plain.sql 002_index_existing.sql
assert_fixture_rejected_as index-existing-concurrent.sql 002_index_existing.sql
assert_fixture_rejected_as index-existing-concurrent-no-guard.sql 002_index_existing_notx.sql

for fixture in \
  add-constraint.sql \
  add-default.sql \
  add-not-null.sql \
  add-reference.sql \
  add-serial.sql \
  add-domain-not-null.sql \
  alter-type.sql \
  comment-token-bypass.sql \
  copy-from.sql \
  create-table-writable-cte.sql \
  delete-data.sql \
  drop-column-without-keyword.sql \
  drop-constraint.sql \
  drop-default.sql \
  drop-table.sql \
  merge-data.sql \
  multiline-drop.sql \
  on-conflict-update.sql \
	insert-writable-cte.sql \
	insert-side-effect-function.sql \
  rename-column.sql \
  rename-table.sql \
  reviewed-drop.sql \
  set-not-null.sql \
  truncate-table.sql \
  unique-existing-table.sql \
  unique-ambiguous-schema.sql \
  unreviewed-behavior.sql \
  unterminated-dollar-quote.sql \
  unicode-uescape-identifier-bypass.sql \
  unicode-uescape-string-bypass.sql \
  update-data.sql; do
  assert_fixture_rejected "$fixture"
done

modified_repo=$(new_repo modified-existing)
printf '%s\n' 'ALTER TABLE baseline ADD COLUMN changed text;' >> "$modified_repo/backend/migrations/001_baseline.sql"
git -C "$modified_repo" add .
git -C "$modified_repo" commit -qm candidate
expect_rejected "modified released migration" "$modified_repo" HEAD^ HEAD

renamed_repo=$(new_repo renamed-existing)
git -C "$renamed_repo" mv backend/migrations/001_baseline.sql backend/migrations/001_baseline_renamed.sql
git -C "$renamed_repo" commit -qm candidate
expect_rejected "renamed released migration" "$renamed_repo" HEAD^ HEAD

copied_repo=$(new_repo copied-existing)
cp -- "$copied_repo/backend/migrations/001_baseline.sql" "$copied_repo/backend/migrations/002_baseline_copy.sql"
git -C "$copied_repo" add .
git -C "$copied_repo" commit -qm candidate
expect_rejected "copied released migration" "$copied_repo" HEAD^ HEAD

commented_copy_repo=$(new_repo commented-copy-existing)
{
  printf '%s\n' '-- added comment must not disguise a released migration replay'
  cat -- "$commented_copy_repo/backend/migrations/001_baseline.sql"
} > "$commented_copy_repo/backend/migrations/002_baseline_copy.sql"
git -C "$commented_copy_repo" add .
git -C "$commented_copy_repo" commit -qm candidate
expect_rejected "commented copy of released migration" "$commented_copy_repo" HEAD^ HEAD

out_of_order_repo=$(new_repo out-of-order-addition)
printf '%s\n' 'CREATE TABLE late_but_low_number (id bigint PRIMARY KEY);' > "$out_of_order_repo/backend/migrations/000_late.sql"
git -C "$out_of_order_repo" add .
git -C "$out_of_order_repo" commit -qm candidate
expect_rejected "new migration must append lexicographically" "$out_of_order_repo" HEAD^ HEAD

post_baseline_repo=$(new_repo post-baseline-modification)
post_baseline_commit=$(git -C "$post_baseline_repo" rev-parse HEAD)
printf '%s\n' 'ALTER TABLE baseline ADD COLUMN note text;' > "$post_baseline_repo/backend/migrations/002_released.sql"
git -C "$post_baseline_repo" add .
git -C "$post_baseline_repo" commit -qm previous-release
previous_release_commit=$(git -C "$post_baseline_repo" rev-parse HEAD)
printf '%s\n' 'ALTER TABLE baseline ADD COLUMN note integer;' > "$post_baseline_repo/backend/migrations/002_released.sql"
git -C "$post_baseline_repo" add .
git -C "$post_baseline_repo" commit -qm candidate
expect_allowed "fixed baseline check alone sees the final post-baseline file as additive" "$post_baseline_repo" "$post_baseline_commit" HEAD
expect_rejected "previous release detects post-baseline modification" "$post_baseline_repo" "$previous_release_commit" HEAD

tag_history_repo=$(new_repo all-reachable-tag-history)
tag_history_baseline=$(git -C "$tag_history_repo" rev-parse HEAD)
git -C "$tag_history_repo" tag -a v1.0.0-ts.1 -m v1
printf '%s\n' 'CREATE TABLE public.tag_history (id bigint PRIMARY KEY, value text);' > "$tag_history_repo/backend/migrations/003_tag_history.sql"
git -C "$tag_history_repo" add .
git -C "$tag_history_repo" commit -qm failed-v2
tag_history_v2=$(git -C "$tag_history_repo" rev-parse HEAD)
# A failed workflow can still leave a lightweight version tag and registry image.
git -C "$tag_history_repo" tag v1.0.0-ts.2
printf '%s\n' 'CREATE TABLE public.tag_history (id bigint PRIMARY KEY, value bigint);' > "$tag_history_repo/backend/migrations/003_tag_history.sql"
git -C "$tag_history_repo" add .
git -C "$tag_history_repo" commit -qm v3
tag_history_v3=$(git -C "$tag_history_repo" rev-parse HEAD)
git -C "$tag_history_repo" tag -a v1.0.0-ts.3 -m v3

expect_allowed "fixed baseline sees v3 migration as additive" "$tag_history_repo" "$tag_history_baseline" "$tag_history_v3"
expect_allowed "v1 sees v3 migration as additive" "$tag_history_repo" v1.0.0-ts.1 "$tag_history_v3"
expect_rejected "failed v2 tag detects rewritten migration in v3" "$tag_history_repo" "$tag_history_v2" "$tag_history_v3"
if output=$(cd -- "$tag_history_repo" && "$HISTORY_CHECKER" "$tag_history_baseline" "$tag_history_v3" v1.0.0-ts.3 2>&1); then
  echo "expected all-tag migration history check to reject rewritten v2 migration" >&2
  echo "$output" >&2
  exit 1
fi

invalid_repo=$(new_repo invalid-refs)
expect_rejected "invalid baseline ref" "$invalid_repo" refs/tags/does-not-exist HEAD
expect_rejected "option-shaped invalid baseline ref" "$invalid_repo" --definitely-not-a-ref HEAD
expect_rejected "invalid target ref" "$invalid_repo" HEAD refs/tags/does-not-exist

divergent_repo=$(new_repo non-ancestor)
common_commit=$(git -C "$divergent_repo" rev-parse HEAD)
git -C "$divergent_repo" switch -qc candidate
cp -- "$FIXTURES/additive.sql" "$divergent_repo/backend/migrations/002_candidate.sql"
git -C "$divergent_repo" add .
git -C "$divergent_repo" commit -qm candidate
candidate_commit=$(git -C "$divergent_repo" rev-parse HEAD)
git -C "$divergent_repo" switch -qc divergent "$common_commit"
cp -- "$FIXTURES/create-type.sql" "$divergent_repo/backend/migrations/002_divergent.sql"
git -C "$divergent_repo" add .
git -C "$divergent_repo" commit -qm divergent
divergent_commit=$(git -C "$divergent_repo" rev-parse HEAD)
expect_rejected "baseline is not a target ancestor" "$divergent_repo" "$divergent_commit" "$candidate_commit"

allowed_target_repo=$(new_repo target-blob-allowed)
allowed_base=$(git -C "$allowed_target_repo" rev-parse HEAD)
cp -- "$FIXTURES/additive.sql" "$allowed_target_repo/backend/migrations/002_test.sql"
git -C "$allowed_target_repo" add .
git -C "$allowed_target_repo" commit -qm candidate
allowed_target=$(git -C "$allowed_target_repo" rev-parse HEAD)
cp -- "$FIXTURES/drop-table.sql" "$allowed_target_repo/backend/migrations/002_test.sql"
expect_allowed "allowed target blob with unsafe worktree replacement" "$allowed_target_repo" "$allowed_base" "$allowed_target"

rejected_target_repo=$(new_repo target-blob-rejected)
rejected_base=$(git -C "$rejected_target_repo" rev-parse HEAD)
cp -- "$FIXTURES/drop-table.sql" "$rejected_target_repo/backend/migrations/002_test.sql"
git -C "$rejected_target_repo" add .
git -C "$rejected_target_repo" commit -qm candidate
rejected_target=$(git -C "$rejected_target_repo" rev-parse HEAD)
cp -- "$FIXTURES/additive.sql" "$rejected_target_repo/backend/migrations/002_test.sql"
expect_rejected "unsafe target blob with allowed worktree replacement" "$rejected_target_repo" "$rejected_base" "$rejected_target"

expect_rejected "not inside a Git repository" "$TEST_ROOT" HEAD HEAD

echo "release migration gate tests passed"
