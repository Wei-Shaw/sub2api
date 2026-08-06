#!/usr/bin/env bash

set -Eeuo pipefail

UPSTREAM_REPO="${UPSTREAM_REPO:-Wei-Shaw/sub2api}"
FORK_REPO="${FORK_REPO:-dextok/sub2api}"
FORK_BRANCH="${FORK_BRANCH:-main}"
FORK_REMOTE="${FORK_REMOTE:-origin}"
GITHUB_API_URL="${GITHUB_API_URL:-https://api.github.com}"
UPSTREAM_GIT_URL="${UPSTREAM_GIT_URL:-https://github.com/${UPSTREAM_REPO}.git}"
SYNC_VERIFY_COMMAND="${SYNC_VERIFY_COMMAND:-}"
SYNC_DRY_RUN="${SYNC_DRY_RUN:-false}"

log() {
    printf '[upstream-sync] %s\n' "$*"
}

die() {
    printf '[upstream-sync] ERROR: %s\n' "$*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

validate_repository() {
    [[ "$1" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "invalid GitHub repository: $1"
}

github_curl() {
    local -a args=(
        --silent
        --show-error
        --location
        --connect-timeout 15
        --max-time 60
        -H 'Accept: application/vnd.github+json'
        -H 'X-GitHub-Api-Version: 2022-11-28'
    )

    if [[ -n "${GITHUB_TOKEN:-}" ]]; then
        args+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
    fi

    curl "${args[@]}" "$@"
}

write_output() {
    local tag="$1"
    if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        printf 'tag=%s\n' "$tag" >> "$GITHUB_OUTPUT"
    fi
}

for command_name in curl git jq mktemp; do
    require_command "$command_name"
done
validate_repository "$UPSTREAM_REPO"
validate_repository "$FORK_REPO"
[[ "$FORK_BRANCH" =~ ^[A-Za-z0-9._/-]+$ ]] || die "invalid fork branch: $FORK_BRANCH"
[[ "$SYNC_DRY_RUN" == "true" || "$SYNC_DRY_RUN" == "false" ]] || die "SYNC_DRY_RUN must be true or false"

git rev-parse --git-dir >/dev/null 2>&1 || die "run this script inside the fork repository"
git remote get-url "$FORK_REMOTE" >/dev/null 2>&1 || die "git remote not found: $FORK_REMOTE"

log "checking latest release from ${UPSTREAM_REPO}"
release_json="$(github_curl --fail "${GITHUB_API_URL}/repos/${UPSTREAM_REPO}/releases/latest")" || \
    die "failed to query the upstream latest release"

tag="$(jq -er '.tag_name' <<<"$release_json")" || die "upstream response has no tag_name"
release_name="$(jq -r '.name // empty' <<<"$release_json")"
release_body="$(jq -r '.body // empty' <<<"$release_json")"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || die "unsupported upstream release tag: $tag"

fork_release_status="$(github_curl --output /dev/null --write-out '%{http_code}' \
    "${GITHUB_API_URL}/repos/${FORK_REPO}/releases/tags/${tag}")" || \
    die "failed to query fork release ${tag}"

case "$fork_release_status" in
    200)
        log "fork release ${tag} already exists; nothing to do"
        write_output ""
        exit 0
        ;;
    404)
        ;;
    *)
        die "unexpected GitHub API status for fork release ${tag}: ${fork_release_status}"
        ;;
esac

log "fetching ${FORK_REMOTE}/${FORK_BRANCH}"
git fetch --no-tags "$FORK_REMOTE" "+refs/heads/${FORK_BRANCH}:refs/remotes/${FORK_REMOTE}/${FORK_BRANCH}"

if git ls-remote --exit-code --tags "$FORK_REMOTE" "refs/tags/${tag}" >/dev/null 2>&1; then
    log "fork tag ${tag} already exists but its release is missing; requesting a release retry"
    write_output "$tag"
    exit 0
else
    ls_remote_status=$?
    [[ "$ls_remote_status" -eq 2 ]] || die "failed to check whether fork tag ${tag} exists"
fi

upstream_ref="refs/remotes/upstream-release/${tag}"
log "fetching official tag ${tag}"
git fetch --no-tags "$UPSTREAM_GIT_URL" "+refs/tags/${tag}:${upstream_ref}"
git rev-parse --verify "${upstream_ref}^{commit}" >/dev/null 2>&1 || die "official tag ${tag} does not resolve to a commit"

worktree_root="$(mktemp -d "${TMPDIR:-/tmp}/sub2api-upstream-sync.XXXXXX")"
worktree_dir="${worktree_root}/worktree"
tag_created=false

cleanup() {
    if [[ -d "$worktree_dir" ]]; then
        git worktree remove --force "$worktree_dir" >/dev/null 2>&1 || true
    fi
    if [[ "$tag_created" == "true" ]]; then
        git tag --delete "$tag" >/dev/null 2>&1 || true
    fi
    rmdir "$worktree_root" >/dev/null 2>&1 || true
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

git worktree add --detach "$worktree_dir" "refs/remotes/${FORK_REMOTE}/${FORK_BRANCH}" >/dev/null

export GIT_AUTHOR_NAME="${GIT_AUTHOR_NAME:-sub2api-upstream-sync[bot]}"
export GIT_AUTHOR_EMAIL="${GIT_AUTHOR_EMAIL:-sub2api-upstream-sync[bot]@users.noreply.github.com}"
export GIT_COMMITTER_NAME="${GIT_COMMITTER_NAME:-$GIT_AUTHOR_NAME}"
export GIT_COMMITTER_EMAIL="${GIT_COMMITTER_EMAIL:-$GIT_AUTHOR_EMAIL}"

if git -C "$worktree_dir" merge-base --is-ancestor "$upstream_ref" HEAD; then
    log "official ${tag} is already contained in ${FORK_BRANCH}; no merge commit needed"
else
    log "merging official ${tag} into fork ${FORK_BRANCH}"
    if ! git -C "$worktree_dir" merge --no-ff --no-edit \
        -m "chore: merge upstream release ${tag}" "$upstream_ref"; then
        conflict_files="$(git -C "$worktree_dir" diff --name-only --diff-filter=U | tr '\n' ' ')"
        git -C "$worktree_dir" merge --abort >/dev/null 2>&1 || true
        die "merge conflict while applying ${tag}${conflict_files:+: ${conflict_files}}"
    fi
fi

if [[ -n "$SYNC_VERIFY_COMMAND" ]]; then
    log "running verification command"
    (cd "$worktree_dir" && /bin/bash -o pipefail -c "$SYNC_VERIFY_COMMAND") || \
        die "verification failed for ${tag}; fork branch was not changed"
fi

tag_message_file="${worktree_dir}/.git-tag-message"
{
    printf 'Fork release based on upstream %s' "$tag"
    if [[ -n "$release_name" ]]; then
        printf ' (%s)' "$release_name"
    fi
    printf '\n\nUpstream: https://github.com/%s/releases/tag/%s\n' "$UPSTREAM_REPO" "$tag"
    if [[ -n "$release_body" ]]; then
        printf '\n%s\n' "$release_body"
    fi
} > "$tag_message_file"

if [[ "$SYNC_DRY_RUN" == "true" ]]; then
    log "dry run succeeded for ${tag}; no refs were pushed"
    write_output ""
    exit 0
fi

git -C "$worktree_dir" tag --annotate "$tag" --file "$tag_message_file"
tag_created=true

log "atomically pushing ${FORK_BRANCH} and ${tag} to ${FORK_REMOTE}"
git -C "$worktree_dir" push --atomic "$FORK_REMOTE" \
    "HEAD:refs/heads/${FORK_BRANCH}" "refs/tags/${tag}:refs/tags/${tag}"

tag_created=false
write_output "$tag"
log "synchronized ${tag}; release workflow can now publish fork artifacts"
