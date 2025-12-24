# Change: Harden security and reliability

## Why
A full audit found multiple security and reliability risks (setup exposure, CORS misconfiguration, plaintext API keys, missing timeouts, non-atomic redeem flow, token storage risks, and secret leakage). This change hardens the service and aligns behavior with safe defaults.

## What Changes
- Restrict setup endpoints to local-only by default, require a setup token for non-local requests, and honor trusted proxy configuration for client IP detection.
- Replace ad-hoc CORS headers with allowlist-based configuration; credentials only for allowed origins.
- Require `ADMIN_PASSWORD` when `AUTO_SETUP` is enabled and never log admin credentials.
- Store API keys as HMAC-SHA256 hashes + last4; return full key only on creation and mask in list/get responses. **BREAKING** (response shape and storage model)
- Mask SMTP and Turnstile secrets in settings responses.
- Enable TLS verification for proxy probe by default (configurable override).
- Add overall timeouts to upstream HTTP clients with explicit streaming behavior.
- Make redeem operations atomic using a database transaction.
- Keep `Authorization: Bearer` support while adding HttpOnly cookie auth; enforce SameSite=Lax plus Origin/Referer checks for state-changing requests. **BREAKING** (frontend auth flow)
- Sanitize or replace `v-html` usage with safe rendering for dynamic content.
- Tighten config validation for default secrets and update sample config values.

## Impact
- Affected specs: security-hardening
- Affected code: setup handlers, middleware, config, API key model/repo/service, auth handlers, frontend auth store/client, settings service, proxy probe, upstream HTTP client, redeem service, UI components using `v-html`.
- Risks: authentication and CORS behavior changes, API response changes for API keys, proxy/loopback detection behavior changes.
