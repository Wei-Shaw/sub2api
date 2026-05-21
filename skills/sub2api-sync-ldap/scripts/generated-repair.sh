#!/usr/bin/env bash

set -euo pipefail

if [[ ! -d backend ]]; then
    echo "ERROR: run this script in sub2api repository root."
    exit 1
fi

sync_embedded_version() {
    local f="backend/cmd/server/VERSION"
    local latest_tag=""
    local latest_version=""
    local current_version=""
    local tag_sha=""
    local commits_ahead=""

    [[ -f "$f" ]] || return 0

    latest_tag="$(git describe --tags --match 'v[0-9]*' --abbrev=0 2>/dev/null || true)"
    if [[ -n "$latest_tag" ]]; then
        latest_version="${latest_tag#v}"
        tag_sha="$(git rev-list -n 1 "$latest_tag" 2>/dev/null || true)"

        if [[ -n "$tag_sha" ]] && git merge-base --is-ancestor "$tag_sha" HEAD >/dev/null 2>&1; then
            commits_ahead="$(git rev-list --count "${latest_tag}..HEAD" 2>/dev/null || echo 0)"
            if [[ "$commits_ahead" != "0" ]]; then
                latest_version="${latest_version}.${commits_ahead}"
            fi
        fi
    fi

    [[ -n "$latest_version" ]] || return 0

    current_version="$(tr -d '\r\n' < "$f" || true)"
    if [[ "$current_version" == "$latest_version" ]]; then
        return 0
    fi

    printf '%s\n' "$latest_version" > "$f"
    echo "[fix] sync embedded build version -> ${latest_version}"
}

clean_known_schema_dups() {
    local f="backend/ent/migrate/schema.go"
    [[ -f "$f" ]] || return 0

    for sym in IdempotencyRecordsColumns IdempotencyRecordsTable; do
        local cnt
        cnt="$(grep -n "^[[:space:]]*${sym}[[:space:]]*=" "$f" | wc -l | tr -d ' ')"
        if [[ "$cnt" -le 1 ]]; then
            continue
        fi

        echo "[fix] remove duplicate declaration: ${sym}"
        awk -v sym="$sym" '
          function brace_delta(s, i, c, d) {
            d=0
            for (i=1;i<=length(s);i++) {
              c=substr(s,i,1)
              if (c=="{") d++
              else if (c=="}") d--
            }
            return d
          }
          {
            if (!skip && $0 ~ "^[[:space:]]*" sym "[[:space:]]*=") {
              if (seen) {
                skip=1
                depth=brace_delta($0)
                next
              }
              seen=1
            }
            if (skip) {
              depth += brace_delta($0)
              if (depth <= 0) {
                skip=0
              }
              next
            }
            print
          }
        ' "$f" > "${f}.tmp" && mv "${f}.tmp" "$f"
    done
}

run_ent_generate() {
    (cd backend && go generate ./ent)
}

repair_missing_gosum() {
    local err_out="$1"
    local missing_pkg=""

    missing_pkg="$(grep -oE 'missing go.sum entry for module providing package [^[:space:]]+' <<<"$err_out" | head -n 1 | sed -E 's/.* package //')"
    [[ -n "$missing_pkg" ]] || return 1

    echo "[fix] add missing go.sum entry for ${missing_pkg}"
    if (cd backend && go mod download "$missing_pkg"); then
        return 0
    fi

    echo "[fix] fallback to go get for ${missing_pkg}"
    (cd backend && go get "$missing_pkg")
}

sync_embedded_version

echo "Run go generate ./ent ..."
if ! run_ent_generate; then
    echo "WARN: ent generate failed once, try schema duplicate cleanup and retry."
    clean_known_schema_dups
    run_ent_generate
fi

clean_known_schema_dups

echo "Run go generate ./cmd/server ..."
set +e
ERR_OUT="$(cd backend && go generate ./cmd/server 2>&1)"
GEN_EXIT=$?
set -e
if [[ $GEN_EXIT -ne 0 ]]; then
    echo "$ERR_OUT"
    if grep -q "missing go.sum entry for module providing package" <<<"$ERR_OUT"; then
        if repair_missing_gosum "$ERR_OUT"; then
            echo "Retry go generate ./cmd/server after go.sum repair ..."
            set +e
            ERR_OUT="$(cd backend && go generate ./cmd/server 2>&1)"
            GEN_EXIT=$?
            set -e
            if [[ $GEN_EXIT -eq 0 ]]; then
                if [[ "${LDAP_SYNC_TIDY:-0}" == "1" ]]; then
                    echo "Run go mod tidy ..."
                    (cd backend && go mod tidy)
                fi
                echo "OK: generated repair completed."
                exit 0
            fi
            echo "$ERR_OUT"
        fi
    fi
    echo "ERROR: wire generation failed."
    if grep -q "no provider found" <<<"$ERR_OUT"; then
        MISSING="$(grep "no provider found for" <<<"$ERR_OUT" | head -n 1 | sed -E 's/.*no provider found for ([^,]+),.*/\1/' || true)"
        if [[ -n "$MISSING" ]]; then
            echo "Hint: add provider/bind for missing type: ${MISSING}"
        fi
    fi
    exit 1
fi

if [[ "${LDAP_SYNC_TIDY:-0}" == "1" ]]; then
    echo "Run go mod tidy ..."
    (cd backend && go mod tidy)
fi

echo "OK: generated repair completed."
