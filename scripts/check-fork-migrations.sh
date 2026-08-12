#!/usr/bin/env bash

set -Eeuo pipefail

upstream_ref="${1:-}"
[[ -n "$upstream_ref" ]] || {
    printf 'usage: %s <upstream-ref>\n' "$0" >&2
    exit 2
}

git rev-parse --verify "${upstream_ref}^{commit}" >/dev/null 2>&1 || {
    printf 'invalid upstream ref: %s\n' "$upstream_ref" >&2
    exit 2
}

# These migrations shipped before the fork namespace policy existed. Their
# filenames and contents are immutable because deployed databases record both.
readonly -a grandfathered=(
    backend/migrations/194_openai_sol_hidden_billing_backfill.sql
    backend/migrations/195_revert_openai_sol_hidden_billing_backfill.sql
)

is_grandfathered() {
    local candidate="$1"
    local known
    for known in "${grandfathered[@]}"; do
        [[ "$candidate" == "$known" ]] && return 0
    done
    return 1
}

failed=false
while IFS= read -r migration; do
    [[ -n "$migration" ]] || continue
    is_grandfathered "$migration" && continue

    filename="${migration##*/}"
    if [[ ! "$filename" =~ ^fork_[0-9]{14}_[a-z0-9_]+(_notx)?\.sql$ ]]; then
        printf '%s\n' \
            "fork-only migration must use fork_YYYYMMDDHHMM_description.sql: ${migration}" >&2
        failed=true
    fi
done < <(git diff --name-only --diff-filter=AM "${upstream_ref}...HEAD" -- 'backend/migrations/*.sql')

if [[ "$failed" == "true" ]]; then
    printf '%s\n' \
        'Fork migrations use a separate namespace so upstream numeric migrations cannot collide.' >&2
    exit 1
fi

printf 'fork migration namespace check passed against %s\n' "$upstream_ref"
