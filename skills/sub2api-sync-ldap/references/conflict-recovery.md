# Conflict Recovery SOP

Read this file only when `scripts/overlay-apply.sh` or `scripts/sync.sh` stops on merge conflicts.

## Goal

Finish with a releasable `main` that equals:

- latest upstream state
- plus required LDAP behavior
- plus regenerated artifacts

## Recovery Steps

1. Stay on `main`, which should already be based on upstream for the current sync attempt.
2. Inspect conflicted files with `git diff --name-only --diff-filter=U` and resolve each file to "official upstream + required LDAP changes only".
3. If a conflict is in generated output and the source-of-truth files are already correct, prefer a minimal temporary resolution that allows regeneration to succeed.
4. When all conflicts are resolved, `git add` the files and create the merge commit.
5. Resume the rest of the flow with:

```bash
bash skills/sub2api-sync-ldap/scripts/resume-after-conflict.sh
```
6. Use `--no-publish` only when the user explicitly asked for local-only work.

## Resolution Heuristics

- Keep LDAP login, sync, and admin-setting surfaces intact.
- Keep validation and deploy safety checks intact.
- Remove orphaned provider or generated-code references if upstream moved them and the implementation no longer exists.
- Prefer regenerating Wire/Ent output over hand-editing generated files beyond the minimum needed to unblock generation.

## Large Upstream Churn Notes

- If upstream has heavily refactored `frontend/src/views/admin/SettingsView.vue`, keep the upstream tab/page structure and re-add the LDAP card, form fields, payload fields, and test/sync actions. Do not paste the old LDAP branch's whole settings page over upstream; it can silently revert newer payment, WeChat, OIDC, and auth-source-default behavior.
- In `SettingsView.vue`, avoid initializing the LDAP Pinia store at setup/module scope unless all tests install Pinia. A safer conflict resolution is to use local `ldapTesting`/`ldapSyncing` refs and call `adminAPI.settings.testLDAPConnection()` / `syncLDAPUsersNow()` directly from the LDAP card actions.
- In `backend/internal/handler/admin/setting_handler.go`, prefer the upstream `payload := dto.SystemSettings{...}` plus `systemSettingsResponseData(...)` response path, then add LDAP fields to the payload. Do not switch back to the older direct `response.Success(c, dto.SystemSettings{...})` path because it drops newer auth-source-default response wrapping.
- If `backend/internal/service/wire.go` compiles with `ProvideOAuthRefreshAPI redeclared`, keep the newer single provider implementation and remove the duplicate from the LDAP branch.
- When upstream adds arguments to `service.NewAuthService`, keep the upstream constructor signature order and insert LDAP's `ExternalAuthProvider` as the third argument. For nil-only call sites the expected prefix is `NewAuthService(entClient, userRepo, nil, nil, refreshTokenCacheOrNil, cfg, ...)`, and the final upstream-only dependency can usually be `nil` in LDAP tests.
- If `RegisterWithVerification` conflicts, keep the upstream full signature, including newer promotion/invitation/affiliate parameters, then place the LDAP-only local registration block at the start of the function body.
- If `EditAccountModal.vue` conflicts only by duplicating `loadTLSProfiles`, keep the upstream single helper and remove the duplicate LDAP-side copy.
- If Ent generation fails before regeneration with `UserMutation.SetAuthSource` or `User.AuthSource` missing, generated files are inconsistent. Keep `backend/ent/schema/user.go` as the source of truth, add only the minimal temporary generated field/mutation methods needed to let `go generate ./ent` compile, then rerun `generated-repair.sh` so Ent overwrites the temporary bootstrap.
