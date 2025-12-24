# Design: Harden security and reliability

## Setup protection
- Add optional `setup.token` config (env `SETUP_TOKEN`) and require `X-Setup-Token` for non-local requests.
- If no token is configured, only allow loopback IPs during setup.
- Honor trusted proxy configuration for client IP detection; ignore forwarded headers unless explicitly trusted.
- When `AUTO_SETUP` is enabled, require `ADMIN_PASSWORD` and fail fast if missing; do not log secrets.

## CORS
- Replace manual header middleware with `gin-contrib/cors`.
- Introduce `cors.allowed_origins` config (comma-separated env), enable credentials only when origins are explicitly configured.

## API keys
- Add `key_hash` and `key_last4` columns.
- On create: store `key_hash` (HMAC-SHA256) and `key_last4` and return full key once.
- On list/get: return masked key (e.g., `sk-...abcd`) and never return full key.
- On verify: look up by `key_hash`; migrate legacy plaintext keys on first use.
- HMAC secret source: `security.api_key_hmac_secret` (fallback to `jwt.secret` if empty).

## Auth tokens
- Keep `Authorization: Bearer` for API clients; add HttpOnly cookie for browser auth.
- Enforce `SameSite=Lax` and `Secure` in production; require Origin/Referer checks for state-changing requests when cookie auth is used.
- Frontend uses `withCredentials` and removes localStorage persistence.

## Secrets exposure
- Mask SMTP and Turnstile secrets in settings responses; provide boolean flags for presence.

## Reliability
- Add global `Timeout` for upstream HTTP clients with a streaming-safe default (streaming uses context cancellation and avoids premature timeouts).
- Wrap redeem usage + entitlement updates in a single GORM transaction.
- Proxy probe TLS verification enabled by default with config override.

## Config validation
- Reject known default secrets and weak admin password values in `release` mode.
- Update sample config with placeholders.
