# State

## Current focus
- DLG web device-code login vertical slice is locally implemented and verification-complete; release handoff remains gated on security decision, independent review/CI, and approval.

## Repo
- Branch: `feat/upstream-minimal-bot-sales`
- Remote: `https://github.com/cong91/sub2api.git`

## Active changes
- Direct canonical `DLG-XXXX-XXXX-XXXX` codes resolve `user_devices.device_code` through `/auth/invite-login`.
- Active device and owner checks, `last_login_at`, existing token pair, and no bootstrap API-key/subscription provisioning are covered.
- Web `LoginView` sends `client_kind=web`; legacy redeem flow remains separate.
- `bot-sales` fulfillment source is unchanged.

## Verification
- Backend: `go vet ./...`, bounded `go test ./...`, focused DLG unit tests, and full unit-tagged service suite passed.
- Frontend: official `npm run build`, `npm run typecheck`, lint, focused login test, and full Vitest suite passed (`242` files / `1703` tests).
- Preview smoke fetched `/` and the generated entry asset successfully.
- Root-owned local build artifacts were repaired in-place; no generated `dist` files are tracked.
- Audit request-body redaction now covers invitation/device/claim codes; regression coverage was added before the commit boundary.

## Release blockers / risks
- DLG remains a persistent bearer credential; expiry, replay prevention, and post-login rotation are not implemented. Persistent-bearer semantics are now explicitly accepted for this compatibility change; lifecycle hardening remains a separately approved follow-up.
- Independent delegated review was unavailable because the configured reviewer model returned HTTP 404; self-review is recorded but does not replace formal review/CI.
- No commit, push, PR, or production deployment has occurred.

## Beads
- br-missing: install beads_rust; .beads not initialized

## Bootstrap metadata
- mode: smart
- cwd: /home/ubuntu/workspaces/projects/sub2api
