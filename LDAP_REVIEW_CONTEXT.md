# LDAP Review Context (2026-03-01)

## Inputs Read
- `deploy/README_LDAP_ENTERPRISE.md`
- `DEV_GUIDE.md`
- `backend/internal/service/auth_service.go`
- `backend/internal/service/auth_service_ldap.go`
- `backend/internal/service/wire.go`
- `backend/cmd/server/wire.go`
- `backend/cmd/server/wire_gen.go`
- `backend/internal/repository/user_repo.go`
- `backend/internal/handler/admin/setting_handler.go`
- `backend/internal/server/routes/admin.go`
- `backend/internal/server/api_contract_test.go`
- `frontend/src/views/admin/SettingsView.vue`
- `frontend/src/api/admin/settings.ts`
- `frontend/src/stores/app.ts`
- `frontend/src/stores/adminSettings.ts`
- `/root/.codex/skills/sub2api-sync-ldap/scripts/sync.sh`

## Current Repo Signals
- `go test ./internal/server/routes -run TestNonExistent` fails.
- Failure: `backend/internal/server/routes/admin.go:402:2 syntax error: non-declaration statement outside function body`.
- Worktree already has uncommitted edits in generated/wire files (before this review).

## Dimension 1: Backend Architectural Decoupling

### High-risk Coupling Points
1. `AuthService` uses runtime type assertion for LDAP capability:
   - `if ldapRepo, ok := userRepo.(LDAPUserRepository); ok { ... }`
   - This is fragile when repository composition changes; LDAP capability can silently disappear.
2. LDAP mode branching is embedded in core auth methods:
   - `Register`, `SendVerifyCode`, `Login`, `LoginOrRegisterOAuth`, `LoginOrRegisterOAuthWithTokenPair`.
3. LDAP sync worker lifecycle is coupled to `NewAuthService` and package globals (`sync.Once`, `sync.Mutex`).
4. Admin setting handler depends on concrete `*service.AuthService` instead of interface, reducing plugin replaceability.

### Suggested Target Design
- Introduce `ExternalAuthProvider` interface and route LDAP through it:
  - `Login(ctx, identifier, password)`
  - `OnSettingsChanged(ctx)` (optional)
  - `TestConnection(ctx)`
  - `SyncNow(ctx)`
- Keep official `AuthService` as orchestration owner with one extension point:
  - `AuthService.externalAuth ExternalAuthProvider`
- Move LDAP runtime logic into dedicated provider object:
  - `LDAPProvider` holds LDAP-specific repos/settings/parser/sync scheduler.
- Replace package-level `sync.Once` with provider-level lifecycle (`Start`, `Stop`), allowing test isolation.

### Wire Conflict Reduction Plan
- Split provider sets:
  - `service.ProviderSetCore` (upstream-compatible)
  - `service.ProviderSetLDAP` (fork extension)
- In `cmd/server/wire.go`, select provider set with build tags:
  - `wire_core.go` (`//go:build !ldap`)
  - `wire_ldap.go` (`//go:build ldap`)
- Do **not** treat `wire_gen.go` as conflict source of truth; always regenerate from tagged wire injectors.

## Dimension 2: Skill Resilience (`sync.sh`)

## Observed Gaps in Current Script
- No dirty-worktree guard before rebase.
- No rebase conflict preflight hooks.
- Runs `go generate ./ent` but lacks post-generation sanity checks.
- No wire health check (`go generate ./cmd/server` + provider diagnostics).
- No automatic handling for known duplicate declarations in generated files.

### Suggested Hardening Snippet (drop-in shell logic)
```bash
# Known duplicate declarations cleanup in generated schema.go
clean_known_schema_dups() {
  local f="backend/ent/migrate/schema.go"
  [ -f "$f" ] || return 0

  for sym in IdempotencyRecordsColumns IdempotencyRecordsTable; do
    local cnt
    cnt=$(rg -n "^[[:space:]]*${sym}[[:space:]]*=" "$f" | wc -l | tr -d ' ')
    if [ "${cnt}" -le 1 ]; then
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
    ' "$f" >"${f}.tmp" && mv "${f}.tmp" "$f"
  done
}
```

### Wire Strategy Recommendation
- Pre-rebase backup `wire_gen.go` is optional and low value (generated artifact).
- Better strategy:
  1. Rebase complete.
  2. `go generate ./ent`.
  3. `go generate ./cmd/server`.
  4. If failure includes `no provider found`, parse type and locate provider set gaps (`service/wire.go`, `repository/wire.go`, `handler/wire.go`).
  5. Regenerate `wire_gen.go` after fix.

### Health Self-check Pseudocode
```text
health_check():
  run go generate ./ent
  if fail: print "Ent generation failed" + stderr summary + fix tips

  run go generate ./cmd/server
  if fail:
    if stderr contains "no provider found":
      extract missing type
      print:
        - check constructor exists
        - check constructor is in ProviderSet (service/repository/handler)
        - check wire inject file includes correct ProviderSet
    else:
      print generic wire failure tips
    return fail

  run go test -tags=unit ./...
  if fail: print first failing package and suggestion

  return ok
```

## Dimension 3: Frontend Isolation & Pluginization

### Observed Coupling
- LDAP UI block is hardcoded inside `SettingsView.vue` template and script payload construction.
- LDAP parsing/serialization is mixed with unrelated settings fields in one giant `saveSettings()` payload builder.
- No dedicated LDAP store; current stores only include global app/admin ops settings.

### Suggested Frontend Refactor
1. Extract `LDAPSettingsPanel.vue` (pure LDAP form + test/sync actions).
2. Add `useLdapSettingsStore` for:
   - form state
   - text<->array/json normalization
   - `testConnection()` / `syncNow()` actions
3. Mount via plugin slot registry in settings page:
   - `SettingsExtensionHost` reads extension list and mounts panels.
   - LDAP contributes one extension with stable key (e.g. `ldap.settings.panel`).
4. Keep parent page generic:
   - parent only loads full settings and dispatches subsection save handlers.

### Minimal plugin anchor pattern
- Keep one stable anchor id in settings page (`<SettingsExtensionHost area="auth" />`).
- LDAP panel is discovered by extension registry instead of static block.

## Dimension 4: Regression Testing & API Contract

### Current Coverage State
- `api_contract_test.go` only validates LDAP fields in `GET /admin/settings` response schema.
- No contract tests for:
  - LDAP login flow
  - LDAP JIT user creation
  - AuthSource persistence
  - group mapping enforcement
  - periodic sync disable behavior

### Recommended LDAP Contract Test Matrix
1. `POST /auth/login` with LDAP enabled + valid LDAP user:
   - expect success
   - user `auth_source=ldap`
   - role/balance/concurrency follow mapping
2. Existing local admin + LDAP enabled:
   - local admin password login still works
3. User removed from allowed LDAP groups:
   - sync run disables user and revokes sessions
4. Upstream model drift simulation:
   - missing/renamed user fields should not panic; fail with stable error code
5. Group mapping precedence:
   - multiple groups -> highest priority mapping wins

## Immediate Action Items
1. Fix compile blocker in `backend/internal/server/routes/admin.go` first.
2. Add `ExternalAuthProvider` seam and migrate LDAP logic behind it.
3. Split wire injector files by build tags (`ldap` / `!ldap`).
4. Harden `sync.sh` with duplicate cleanup + wire health checks.
5. Extract LDAP settings panel + dedicated store + extension host.
6. Add LDAP contract tests and at least one sync lifecycle test.
