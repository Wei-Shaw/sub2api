## 1. Implementation
- [x] Add config fields for CORS allowlist, setup token, trusted proxies, proxy TLS verify, upstream timeout, and API key HMAC secret
- [x] Harden setup routes (local-only + token header for non-local + trusted proxy IP resolution)
- [x] Require ADMIN_PASSWORD for AUTO_SETUP and remove any secret logging
- [x] Replace CORS middleware with allowlist-based config
- [x] Implement API key HMAC hashing + masking and migration path for legacy keys
- [x] Update API key responses (create returns full key once, list/get masked)
- [x] Add HttpOnly cookie auth while keeping Authorization header support
- [x] Add Origin/Referer checks for cookie-auth state-changing requests
- [x] Update frontend client/store to use cookies and stop localStorage persistence
- [x] Mask settings secrets in responses; add presence flags
- [x] Enable TLS verification for proxy probe with config override
- [x] Add global timeout to upstream HTTP client with streaming-safe behavior
- [x] Make redeem flow transactional
- [x] Sanitize or replace `v-html` usage with safe rendering
- [x] Update config validation and sample config placeholders

## 2. Verification
- [ ] Manual checks for setup token/local-only behavior
- [ ] CORS preflight and credentialed requests with allowlist
- [ ] API key create/list/get behavior and legacy key verification
- [ ] Login sets HttpOnly cookie; authenticated requests succeed without localStorage
- [ ] Redeem flow remains consistent on failure scenarios
