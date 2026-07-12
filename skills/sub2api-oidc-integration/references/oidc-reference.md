# Sub2API OIDC — RP Integration Reference

Technical reference for integrating an application as an OIDC **Relying Party (RP)**
against a **Sub2API** instance acting as the **OpenID Connect Provider (OP)**.

All endpoints derive from the **Issuer URL** provided by the operator (e.g.
`https://api.example.com`, no trailing slash). Prefer reading capabilities from the
discovery document at runtime.

---

## 1. Endpoints

```
GET  {issuer}/.well-known/openid-configuration   Discovery document
GET  {issuer}/.well-known/jwks.json              JWKS (public RSA keys for RS256)
GET  {issuer}/oidc/authorize                     Authorization endpoint (browser redirect)
POST {issuer}/oidc/token                         Token endpoint
GET  {issuer}/oidc/userinfo                      UserInfo (also accepts POST)
```

All endpoints return **HTTP 404** when the provider feature is disabled on the instance.

### Discovery document (example)

```json
{
  "issuer": "https://api.example.com",
  "authorization_endpoint": "https://api.example.com/oidc/authorize",
  "token_endpoint": "https://api.example.com/oidc/token",
  "userinfo_endpoint": "https://api.example.com/oidc/userinfo",
  "jwks_uri": "https://api.example.com/.well-known/jwks.json",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "subject_types_supported": ["public"],
  "id_token_signing_alg_values_supported": ["RS256"],
  "scopes_supported": ["openid","profile","email","offline_access","sub2api:apikey"],
  "token_endpoint_auth_methods_supported": ["client_secret_basic","client_secret_post"],
  "code_challenge_methods_supported": ["S256"],
  "claims_supported": ["sub","iss","aud","exp","iat","auth_time","nonce","acr","name","preferred_username","email","email_verified"]
}
```

---

## 2. Capabilities

| Capability | Value |
|------------|-------|
| Response type | `code` only (no implicit/hybrid) |
| Grant types | `authorization_code`, `refresh_token` |
| PKCE | `S256` **mandatory** — `code_challenge` required, `plain` rejected |
| Client auth | `client_secret_basic` (HTTP Basic) or `client_secret_post` (form) |
| ID Token alg | `RS256` (rotating keys via JWKS) |
| Subject type | `public` (stable numeric `sub`) |

---

## 3. Scopes → Claims

| Scope | Releases | Where |
|-------|----------|-------|
| `openid` (required) | `sub` | ID Token + UserInfo |
| `profile` | `name`, `preferred_username` | ID Token + UserInfo |
| `email` | `email`, `email_verified` | ID Token + UserInfo |
| `offline_access` | (enables `refresh_token`) | — |
| `sub2api:apikey` ⚠️ | `sub2api_apikey_count`, `sub2api_apikeys` | **UserInfo only** |

ID Token always carries `iss`, `aud`, `exp`, `iat`, `auth_time`, `acr`, and `nonce`
(if sent). The sensitive `sub2api_*` claims are **never** in the ID Token.

---

## 4. Authorization request

Per login attempt, generate:

```
code_verifier  = base64url(random 32–96 bytes)        # store server-side (session)
code_challenge = base64url(SHA256(code_verifier))      # no padding
state          = random  (CSRF)
nonce          = random  (ID Token replay binding)
```

Redirect the browser:

```
GET {issuer}/oidc/authorize
  ?client_id={CLIENT_ID}
  &redirect_uri={REGISTERED_REDIRECT_URI}     # exact match
  &response_type=code
  &scope=openid%20profile%20email%20offline_access
  &state={state}
  &nonce={nonce}
  &code_challenge={code_challenge}
  &code_challenge_method=S256
  [&prompt=login]                              # optional: force re-auth
```

### Callback

Success:
```
{REDIRECT_URI}?code={authorization_code}&state={state}
```
Denied / error:
```
{REDIRECT_URI}?error=access_denied&error_description=...&state={state}
```

Validate `state` before continuing.

---

## 5. Token exchange (authorization_code)

Code is single-use, default TTL 10 min. Authenticate the client (Basic shown):

```http
POST {issuer}/oidc/token
Authorization: Basic base64({CLIENT_ID}:{CLIENT_SECRET})
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&code={code}&redirect_uri={REDIRECT_URI}&code_verifier={code_verifier}
```

Response (`200`, `Cache-Control: no-store`):

```json
{
  "access_token": "opaque",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "opaque",
  "id_token": "eyJ...",
  "scope": "openid profile email offline_access"
}
```

`refresh_token` present only when `offline_access` was granted.

---

## 6. ID Token validation

JWT signed RS256. Manual validation steps:

1. Fetch JWKS, pick key by header `kid`.
2. Verify RS256 signature.
3. Assert `iss == issuer`, `aud == CLIENT_ID`, `exp` future, `nonce ==` your nonce.

```json
{
  "iss": "https://api.example.com",
  "sub": "12345",
  "aud": "rp_xxx",
  "exp": 1718600000,
  "iat": 1718596400,
  "auth_time": 1718596400,
  "nonce": "...",
  "acr": "urn:sub2api:authn:basic",
  "name": "alice",
  "preferred_username": "alice",
  "email": "alice@example.com",
  "email_verified": true
}
```

Cache JWKS; refetch on unknown `kid`. Retired keys verify for ~7 days after rotation.

---

## 7. UserInfo

```http
GET {issuer}/oidc/userinfo
Authorization: Bearer {access_token}
```

```json
{
  "sub": "12345",
  "name": "alice",
  "email": "alice@example.com",
  "email_verified": true,
  "sub2api_apikey_count": 2,
  "sub2api_apikeys": [
    {
      "id": 101,
      "key": "sk-xxxxxxxxxxxxxxxxxxxxxxxx",
      "name": "prod",
      "status": "active",
      "created_at": "2026-01-01T00:00:00Z",
      "last_used_at": "2026-06-01T08:00:00Z",
      "expires_at": null
    },
    {
      "id": 102,
      "key": "sk-yyyyyyyyyyyyyyyyyyyyyyyy",
      "name": "dev",
      "status": "active",
      "created_at": "2026-02-01T00:00:00Z",
      "last_used_at": null,
      "expires_at": null
    }
  ]
}
```

Fields depend on granted scopes. Invalid/expired/revoked token → `401` with
`WWW-Authenticate: Bearer error="invalid_token"`.

---

## 7a. Resource: API Keys (paginated)

Protected resource endpoint for RPs that need a structured / paginated view of
the authenticated user's API Keys (instead of the bulk list returned by
UserInfo).

```http
GET {issuer}/oidc/resource/api-keys?page=1&page_size=20&search=&status=&group_id=
Authorization: Bearer {access_token}
```

- Requires the access token to carry scope `sub2api:apikey`.
- Supported query params: `page`, `page_size`, `sort_by` (default `created_at`),
  `sort_order` (`desc` / `asc`), `search`, `status` (`active` / `inactive`),
  `group_id`.

```json
{
  "data": [
    {
      "id": 101,
      "key": "sk-xxxxxxxxxxxxxxxxxxxxxxxx",
      "name": "prod",
      "status": "active",
      "created_at": "2026-01-01T00:00:00Z",
      "last_used_at": "2026-06-01T08:00:00Z",
      "expires_at": null
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

Errors:
- Missing/invalid token → `401` with `WWW-Authenticate: Bearer error="invalid_token"`.
- Token without `sub2api:apikey` → `403` with
  `WWW-Authenticate: Bearer error="insufficient_scope", scope="sub2api:apikey"`.

---

## 8. Refresh (rotating, reuse-detected)

```http
POST {issuer}/oidc/token
Authorization: Basic base64({CLIENT_ID}:{CLIENT_SECRET})
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&refresh_token={current}[&scope=openid%20email]
```

- Returns a **new** `refresh_token`; persist it, discard the old one.
- Reusing a rotated token → entire family revoked → `invalid_grant` (re-auth required).
- Optional `scope` may only **downgrade** (subset of original); widening is rejected.

---

## 9. Errors

Token/UserInfo → JSON `{ "error": "...", "error_description": "..." }`.

| HTTP | error | Cause |
|------|-------|-------|
| 400 | `invalid_request` | missing/malformed param |
| 400 | `invalid_grant` | bad/expired/used code, PKCE/redirect mismatch, refresh reuse |
| 400 | `invalid_scope` | scope exceeds grant |
| 400 | `unsupported_grant_type` | unsupported grant |
| 401 | `invalid_client` | bad client creds / disabled |
| 401 | `invalid_token` | UserInfo / resource token expired/revoked/unknown |
| 403 | `insufficient_scope` | resource endpoint called without required scope |

Authorize errors redirect to `redirect_uri?error=...&state=...` when the URI is
valid; otherwise a `400` JSON body (it won't redirect to an untrusted URI).

---

## 10. Default token lifetimes

| Token | Default |
|-------|---------|
| Authorization code | 10 min (single-use) |
| Access token | 1 hour |
| ID Token | 1 hour |
| Refresh token | 30 days (rotating) |

Read `expires_in` / `exp` at runtime; don't hard-code.

---

## 11. Examples

### Node.js — `openid-client`

```js
import { Issuer, generators } from 'openid-client';

const issuer = await Issuer.discover(process.env.SUB2API_ISSUER_URL);
const client = new issuer.Client({
  client_id: process.env.SUB2API_CLIENT_ID,
  client_secret: process.env.SUB2API_CLIENT_SECRET,
  redirect_uris: [process.env.SUB2API_REDIRECT_URI],
  response_types: ['code'],
});

// start
const code_verifier = generators.codeVerifier();
const code_challenge = generators.codeChallenge(code_verifier);
const state = generators.state();
const nonce = generators.nonce();
// save {code_verifier, state, nonce} in session
const url = client.authorizationUrl({
  scope: 'openid profile email offline_access',
  state, nonce, code_challenge, code_challenge_method: 'S256',
});

// callback
const params = client.callbackParams(req);
const tokenSet = await client.callback(process.env.SUB2API_REDIRECT_URI, params,
  { state, nonce, code_verifier });
const claims = tokenSet.claims();
const userinfo = await client.userinfo(tokenSet.access_token);

// refresh
const next = await client.refresh(tokenSet.refresh_token);
// persist next.refresh_token
```

### Go — `go-oidc/v3` + `oauth2`

```go
provider, _ := oidc.NewProvider(ctx, issuerURL)
cfg := &oauth2.Config{
    ClientID:     clientID,
    ClientSecret: clientSecret,
    RedirectURL:  redirectURI,
    Endpoint:     provider.Endpoint(),
    Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "offline_access"},
}
verifier := oauth2.GenerateVerifier() // store in session
authURL := cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce))

// callback
tok, _ := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
rawID := tok.Extra("id_token").(string)
idt, _ := provider.Verifier(&oidc.Config{ClientID: clientID}).Verify(ctx, rawID)
// assert idt.Nonce == nonce, then read claims
```

### Python — `authlib`

```python
from authlib.integrations.requests_client import OAuth2Session

sess = OAuth2Session(CLIENT_ID, CLIENT_SECRET,
                     scope="openid profile email offline_access",
                     redirect_uri=REDIRECT_URI,
                     code_challenge_method="S256")
uri, state = sess.create_authorization_url(
    f"{ISSUER}/oidc/authorize", code_verifier=verifier, nonce=nonce)
# redirect to uri

token = sess.fetch_token(f"{ISSUER}/oidc/token",
                         authorization_response=callback_url, code_verifier=verifier)
userinfo = sess.get(f"{ISSUER}/oidc/userinfo").json()
```

---

## 12. Pre-flight checklist

- [ ] `openid` always in the requested scope set.
- [ ] PKCE `S256` (`code_challenge` + `code_challenge_method=S256`).
- [ ] `redirect_uri` matches a registered URI byte-for-byte.
- [ ] `state` validated on callback; `nonce` validated in the ID Token.
- [ ] ID Token signature + `iss` + `aud` verified before trusting claims.
- [ ] `offline_access` requested if refresh is needed.
- [ ] Rotated refresh token persisted after every refresh; old one discarded.
- [ ] `client_secret` kept server-side (BFF for SPA/mobile).
- [ ] Sensitive `sub2api:*` scopes only requested when genuinely needed.
