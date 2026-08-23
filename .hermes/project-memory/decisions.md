# Decisions

Record durable architecture/product decisions here.

## 2026-08-23
- Security policy: accept persistent DLG bearer semantics for this compatibility fix. A valid DLG code may be replayed until its `user_devices` binding or owner is deactivated/revoked; expiry, one-time consume, replay prevention, and post-login rotation are explicitly deferred and are not release blockers for this scoped change.
- Required controls: canonical 12-character code format, direct binding/owner active checks, captcha verification, fail-closed Redis-backed rate limiting on `/auth/invite-login`, no credential values in logs/audit bodies, and operational revoke by changing device/owner status. Any future lifecycle hardening must be a separately approved contract change with bot-sales compatibility and rollback design.
- Release boundary: merge/release still requires CI and independent review; production mutation is not part of this change.

## 2026-08-22
- Decision: `InviteLogin` accepts canonical `DLG-XXXX-XXXX-XXXX` device codes through `user_devices.device_code`; the binding and owner must be active, `last_login_at` is updated, and direct DLG login issues the existing token pair without invitation bootstrap API-key/subscription provisioning. Legacy invitation/redeem and redeem-backed device-hash validation remain separate contracts.
- Context: Bot-sales and VClaw issue persistent device login codes for web/device sign-in; the public invite-login route already has fail-closed auth rate limiting and captcha verification.
- Consequence: Web clients identify themselves with `client_kind=web`; non-web redeem-backed/device flows continue requiring a valid bound `device_hash`. DLG remains a persistent bearer credential: expiry, replay prevention, and post-login rotation are not implemented and must be explicitly accepted or addressed before release approval.
