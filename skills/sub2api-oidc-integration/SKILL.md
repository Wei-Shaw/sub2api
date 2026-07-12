---
name: sub2api-oidc-integration
description: Integrate a third-party application as a Relying Party (RP) with a Sub2API deployment acting as an OpenID Connect Provider (OP). Use when the user wants to "add Login with Sub2API", enable SSO / single sign-on through Sub2API, authenticate users via a Sub2API account, implement OIDC/OAuth2 against Sub2API, wire up the Authorization Code + PKCE flow, exchange tokens, verify ID Tokens, call UserInfo, or refresh tokens against Sub2API's /oidc endpoints. Covers Node.js, Go, Python, and any conformant OIDC client library.
---

# Sub2API OIDC Integration (Relying Party)

Help the user integrate their application as an **OIDC Relying Party (RP)** against
a **Sub2API** instance acting as the **OpenID Connect Provider (OP)**.

Sub2API is a standards-compliant OP: **Authorization Code flow + PKCE (S256)** and
**rotating Refresh Tokens**, RS256-signed ID Tokens, with OIDC Discovery. Prefer a
**mature OIDC client library** driven by the discovery URL over hand-rolling HTTP.

## Before writing code — gather these inputs

Ask the user (or look for them in env/config) for the values the **Sub2API operator**
must provide. The RP cannot self-register.

- `ISSUER_URL` — e.g. `https://api.example.com` (no trailing slash).
- `CLIENT_ID` — e.g. `rp_xxxxxxxx`.
- `CLIENT_SECRET` — confidential; backend only.
- Registered `REDIRECT_URI`(s) — must match the app's callback **byte-for-byte**.
- Granted scopes — the subset the client is allowed to request.

If any are missing, tell the user to obtain them from whoever runs the Sub2API
instance (Admin → OIDC Clients). If endpoints return `404`, the feature is disabled
on that instance.

## Verify the provider first (read-only)

Always confirm the instance is live and read its real capabilities from discovery
before generating code:

```bash
curl -s "$ISSUER_URL/.well-known/openid-configuration" | jq .
```

Use the returned `authorization_endpoint`, `token_endpoint`, `userinfo_endpoint`,
`jwks_uri`, and `scopes_supported` — do not hard-code paths if discovery is reachable.

## Implementation workflow

1. **Choose a library** for the user's stack (do NOT hand-roll if avoidable):
   - Node.js → `openid-client`
   - Go → `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2`
   - Python → `authlib`
   - Browser/SPA/mobile → use a **backend-for-frontend (BFF)**; never embed `client_secret`.
2. **Login start**: generate per-attempt `code_verifier`/`code_challenge` (S256),
   `state`, and `nonce`; store them in the user's session; redirect to the
   authorization endpoint with `scope` including `openid`.
3. **Callback**: validate `state`, then exchange `code` + `code_verifier` at the
   token endpoint (authenticate the client via Basic or form body).
4. **Verify the ID Token**: RS256 via JWKS, check `iss`, `aud == client_id`, `exp`,
   and `nonce`. (Libraries do this — make sure it is actually enabled.)
5. **UserInfo** (optional): call with the Bearer access token for live claims,
   including the sensitive `sub2api:*` claims (which are never in the ID Token).
6. **Refresh** (only if `offline_access` was requested): rotate tokens; persist the
   **new** refresh token; never reuse an old one (reuse revokes the whole family).

## Critical rules (enforced by Sub2API)

- **PKCE S256 is mandatory** — always send `code_challenge` + `code_challenge_method=S256`.
- **`redirect_uri` must match exactly** (scheme, path, trailing slash, query). Prod = `https://`.
- **`offline_access` is required to get a `refresh_token`.**
- **Refresh tokens rotate**: store the newest; reusing a rotated token → `invalid_grant`
  + family revocation → user must re-authenticate.
- **`scope` must include `openid`** and stay within the client's allowed set.
- Keep `client_secret` server-side only.
- Request the **minimum scopes**; `sub2api:apikey` is sensitive
  and warns the user on the consent screen.

## Full reference

For endpoints, scope→claim mapping, request/response shapes, error codes, token
lifetimes, and copy-paste examples (Node/Go/Python), read
[references/oidc-reference.md](references/oidc-reference.md).

## After implementing — verify

- Run the login flow end-to-end; confirm the callback receives `code` + matching `state`.
- Confirm the ID Token verifies (signature + `iss`/`aud`/`nonce`) and `sub` is stable.
- If using refresh, confirm a second refresh with the **old** token fails with
  `invalid_grant` (proves rotation is wired correctly) and that you persisted the new one.
