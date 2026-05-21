#!/usr/bin/env bash

set -euo pipefail

if [[ ! -d backend ]]; then
    echo "ERROR: run this script in sub2api repository root."
    exit 1
fi

echo "Gate: LDAP contract (unit)"
(cd backend && go test -tags=unit ./internal/server -run TestLDAPLoginContract -count=1)

echo "Gate: LDAP contract (unit+ldap)"
(cd backend && go test -tags='unit ldap' ./internal/server -run TestLDAPLoginContract -count=1)

echo "Gate: backend server build compile"
(cd backend && go test ./cmd/server -run TestDoesNotExist -count=1)

if [[ "${LDAP_SYNC_FULL_TESTS:-0}" == "1" ]]; then
    echo "Gate: full backend tests (unit)"
    (cd backend && go test -tags=unit ./...)

    echo "Gate: full backend tests (unit+ldap)"
    (cd backend && go test -tags='unit ldap' ./...)
fi

if [[ -d frontend && -f frontend/package.json ]]; then
    echo "Gate: frontend typecheck"
    (
        cd frontend
        pnpm install --silent
        pnpm run typecheck
    )

    echo "Gate: frontend test suite"
    (
        cd frontend
        pnpm run test:run
    )
fi

echo "OK: contract gate passed."
