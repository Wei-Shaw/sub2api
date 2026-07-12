# oidc-provider Specification

## Purpose
TBD - created by archiving change add-oidc-provider. Update Purpose after archive.
## Requirements
### Requirement: OIDC Discovery Endpoint

The system SHALL expose an OpenID Connect Discovery 1.0 compliant document at `GET /.well-known/openid-configuration` whose body advertises the issuer URL configured by the administrator and the locations of all other OIDC endpoints.

#### Scenario: Discovery returns standards-compliant JSON when provider is enabled

- **WHEN** an unauthenticated client sends `GET /.well-known/openid-configuration` and `oidc_provider.enabled=true` and `oidc_provider.issuer_url` is set
- **THEN** the response is HTTP 200 with `Content-Type: application/json`
- **AND** the JSON body contains `issuer` equal to the configured `oidc_provider.issuer_url` (no trailing slash)
- **AND** the JSON body contains `authorization_endpoint`, `token_endpoint`, `userinfo_endpoint`, `jwks_uri` formed by appending `/oidc/authorize`, `/oidc/token`, `/oidc/userinfo`, `/.well-known/jwks.json` respectively to the issuer URL
- **AND** the JSON body declares `response_types_supported: ["code"]`, `grant_types_supported: ["authorization_code","refresh_token"]`, `id_token_signing_alg_values_supported: ["RS256"]`, `subject_types_supported: ["public"]`, `code_challenge_methods_supported: ["S256"]`, `token_endpoint_auth_methods_supported: ["client_secret_basic","client_secret_post"]`
- **AND** the JSON body declares `scopes_supported` containing exactly `openid`, `profile`, `email`, `offline_access`, `sub2api:apikey`

#### Scenario: Discovery is hidden when provider is disabled

- **WHEN** an unauthenticated client sends `GET /.well-known/openid-configuration` and `oidc_provider.enabled=false`
- **THEN** the response is HTTP 404 with no OIDC metadata body

### Requirement: JWKS Endpoint

The system SHALL expose a JSON Web Key Set at `GET /.well-known/jwks.json` containing the RSA public keys for every active and grace-period signing key, and SHALL never expose any private key material on this endpoint.

#### Scenario: JWKS lists all loaded public keys

- **WHEN** an unauthenticated client sends `GET /.well-known/jwks.json` while the provider has at least one signing key
- **THEN** the response is HTTP 200 with `Content-Type: application/json`
- **AND** the JSON body has shape `{ "keys": [ {kty, kid, use, alg, n, e}, ... ] }`
- **AND** every key entry has `kty: "RSA"`, `use: "sig"`, `alg: "RS256"`
- **AND** every key entry's `n` and `e` are base64url-encoded RSA modulus and exponent, with no padding
- **AND** the array contains the currently active `kid` plus every prior `kid` whose retirement timestamp is within the grace window of 7 days

#### Scenario: JWKS never leaks private key material

- **WHEN** the JWKS response body is generated
- **THEN** no field named `d`, `p`, `q`, `dp`, `dq`, `qi`, or any PEM-encoded private content appears anywhere in the JSON

### Requirement: Authorization Endpoint with PKCE

The system SHALL implement `GET /oidc/authorize` per OIDC Core 1.0 Authorization Code Flow, requiring PKCE with `code_challenge_method=S256`, redirecting to the registered redirect URI with an authorization code on success and with a standards-compliant error parameter set on failure.

#### Scenario: Successful authorization with valid PKCE

- **WHEN** the user has a valid SSO session
- **AND** a request arrives at `/oidc/authorize` with `response_type=code`, a known `client_id`, a `redirect_uri` exactly matching one of the client's registered URIs, `scope` containing `openid` plus a subset of the client's `allowed_scopes`, a `state`, a `nonce`, `code_challenge`, and `code_challenge_method=S256`
- **AND** the client's `consent_required` is `false` OR a covering `oidc_consent` record already exists for this `(user, client)` whose granted scopes are a superset of the requested scopes
- **THEN** the system MUST issue a one-time authorization code, persist it bound to `(client_id, user_id, redirect_uri, scopes, code_challenge, nonce)` with a 10-minute expiry
- **AND** redirect 302 to `<redirect_uri>?code=<code>&state=<state>` with no other parameters added

#### Scenario: Authorize requires PKCE

- **WHEN** an authorize request omits `code_challenge` OR uses `code_challenge_method` other than `S256`
- **THEN** the system MUST redirect to `<redirect_uri>?error=invalid_request&error_description=PKCE+S256+required&state=<state>` if `redirect_uri` and `client_id` are valid
- **AND** the system MUST respond with HTTP 400 JSON `{error: "invalid_request"}` if `client_id` or `redirect_uri` is invalid (cannot safely redirect)

#### Scenario: Unknown client or mismatched redirect URI

- **WHEN** an authorize request carries a `client_id` not present in `oidc_client` or a `redirect_uri` not present in that client's registered URIs (after exact-match comparison)
- **THEN** the system MUST respond with HTTP 400 JSON `{error: "invalid_client", error_description: ...}` and MUST NOT redirect

#### Scenario: User not signed in is sent to login

- **WHEN** an authorize request arrives but the request lacks a valid `sub2api_sso` cookie
- **THEN** the system MUST redirect 302 to the sub2api login page with a `next` query parameter containing the original authorize URL (URL-encoded)
- **AND** after the user successfully authenticates, the system MUST redirect back to the original authorize URL so the flow resumes

#### Scenario: prompt=login forces reauthentication

- **WHEN** an authorize request includes `prompt=login` and the user has a valid SSO session
- **THEN** the system MUST clear the SSO cookie and redirect 302 to the login page with `next` set to the authorize URL minus the `prompt` parameter

#### Scenario: User denies consent

- **WHEN** the user is presented with the consent page and submits a deny decision
- **THEN** the system MUST redirect 302 to `<redirect_uri>?error=access_denied&error_description=User+denied+consent&state=<state>`
- **AND** the system MUST NOT issue an authorization code

### Requirement: Token Endpoint with Authorization Code and Refresh Token Grants

The system SHALL implement `POST /oidc/token` accepting only `grant_type=authorization_code` and `grant_type=refresh_token`, validating client credentials via either `client_secret_basic` (HTTP Basic header) or `client_secret_post` (form fields), and returning a standards-compliant token response.

#### Scenario: Authorization code exchange succeeds

- **WHEN** a client posts `grant_type=authorization_code`, a `code` matching an unconsumed unexpired `oidc_authorization_code` row, the matching `redirect_uri`, the matching `code_verifier` (whose `S256` hash equals the stored `code_challenge`), and valid client credentials
- **THEN** the system MUST mark the code consumed (write `consumed_at`)
- **AND** issue an opaque access token and an opaque refresh token (only if `offline_access` was in scopes)
- **AND** issue an ID Token signed with the active RSA key (`alg=RS256`) whose claims include `iss`, `sub` (user_id as string), `aud` (client_id), `exp`, `iat`, `auth_time`, `nonce` (from the original authorize request, if any), `acr`, `amr`, plus all profile/email claims for the granted scopes
- **AND** respond HTTP 200 with `Cache-Control: no-store` and JSON `{access_token, token_type: "Bearer", expires_in, refresh_token?, id_token, scope}`

#### Scenario: Authorization code is single-use

- **WHEN** a client attempts to exchange the same `code` a second time (regardless of success or failure of the first attempt that consumed it)
- **THEN** the system MUST respond HTTP 400 JSON `{error: "invalid_grant"}`
- **AND** the system MUST revoke any tokens already issued from that code's authorization flow

#### Scenario: Authorization code rejects redirect_uri mismatch

- **WHEN** the `redirect_uri` posted to the token endpoint does not exactly equal the `redirect_uri` recorded with the authorization code
- **THEN** the system MUST respond HTTP 400 JSON `{error: "invalid_grant"}`

#### Scenario: PKCE verifier mismatch

- **WHEN** SHA-256 of the posted `code_verifier`, base64url-encoded without padding, does not equal the stored `code_challenge`
- **THEN** the system MUST respond HTTP 400 JSON `{error: "invalid_grant"}`

#### Scenario: Refresh token rotation succeeds

- **WHEN** a client posts `grant_type=refresh_token` with a valid unrevoked unexpired refresh token plus valid client credentials
- **THEN** the system MUST mark the presented refresh token revoked
- **AND** issue a new refresh token in the same `family_id` whose `parent_token_hash` records the prior token
- **AND** issue a new access token and a new ID Token
- **AND** respond HTTP 200 with the same JSON shape as authorization_code exchange

#### Scenario: Refresh token reuse triggers family revocation

- **WHEN** a client presents a refresh token that has already been marked revoked
- **THEN** the system MUST revoke every token in the same `family_id` (set `revoked_at`)
- **AND** respond HTTP 400 JSON `{error: "invalid_grant", error_description: "refresh token reuse detected"}`
- **AND** log a security event `oidc.refresh_token.reuse_detected` with `family_id`, `client_id`, `user_id`

#### Scenario: Scope downgrade on refresh

- **WHEN** a refresh request includes a `scope` parameter whose values are a strict subset of the original refresh token's stored scopes
- **THEN** the system MUST issue new tokens carrying only the requested subset
- **AND** non-requested scopes MUST NOT appear in the new ID Token claims

#### Scenario: Invalid client credentials

- **WHEN** the token endpoint receives credentials whose `client_id` is unknown OR whose `client_secret` (after bcrypt comparison) does not match the stored hash
- **THEN** the system MUST respond HTTP 401 JSON `{error: "invalid_client"}`

#### Scenario: Unsupported grant_type

- **WHEN** the token endpoint receives `grant_type` other than `authorization_code` or `refresh_token`
- **THEN** the system MUST respond HTTP 400 JSON `{error: "unsupported_grant_type"}`

### Requirement: UserInfo Endpoint

The system SHALL implement `GET /oidc/userinfo` requiring a Bearer access token issued by the OIDC token endpoint, returning JSON claims that match the scopes granted to that access token.

#### Scenario: Bearer token returns scoped claims

- **WHEN** a client sends `GET /oidc/userinfo` with header `Authorization: Bearer <opaque-access-token>` matching an unrevoked unexpired access token tied to scopes `openid profile email`
- **THEN** the response is HTTP 200 JSON containing exactly `sub`, `email`, `email_verified`, `name`, `preferred_username`, `updated_at` (other claims MUST be absent)
- **AND** `sub` equals the user_id as a decimal string



#### Scenario: sub2api:apikey scope returns apikey list with key values

- **WHEN** the access token's scopes include `sub2api:apikey`
- **THEN** the userinfo response MUST contain `sub2api_apikey_count` (integer count of api_keys for the user)
- **AND** the userinfo response MUST contain `sub2api_apikeys` (array of api_key objects, each containing `id`, `key` (plaintext key value), `name`, `status`, `created_at`, `last_used_at`, `expires_at`)

#### Scenario: Missing or invalid Bearer token

- **WHEN** the userinfo request lacks `Authorization` header OR carries a token that is unknown / revoked / expired / not issued by the OIDC token endpoint
- **THEN** the response is HTTP 401 with header `WWW-Authenticate: Bearer error="invalid_token"`

### Requirement: Resource Endpoint — API Keys

The system SHALL implement `GET /oidc/resource/api-keys` as an OAuth2-protected resource endpoint that returns the API Keys belonging to the user bound to the presented OIDC access token. The endpoint MUST require Bearer authentication using an OIDC access token whose granted scopes include `sub2api:apikey`.

#### Scenario: Bearer token with sub2api:apikey scope returns paginated API Keys

- **WHEN** a client sends `GET /oidc/resource/api-keys` with header `Authorization: Bearer <opaque-access-token>` matching an unrevoked unexpired access token whose scopes include `sub2api:apikey`
- **THEN** the response is HTTP 200 JSON of shape `{data: [APIKey...], total: int, page: int, page_size: int}` where each `APIKey` includes at least `id`, `key` (plaintext key value), `name`, `status`, `created_at`, `last_used_at`, `expires_at`
- **AND** only API Keys whose `user_id` equals the access token's bound user MUST be returned
- **AND** the endpoint MUST honor query parameters `page`, `page_size`, `sort_by`, `sort_order`, `search`, `status`, `group_id` consistently with the existing `/api/v1/user/keys` endpoint

#### Scenario: Bearer token without sub2api:apikey scope is rejected

- **WHEN** the request carries a valid access token whose scopes do NOT include `sub2api:apikey`
- **THEN** the response MUST be HTTP 403 JSON `{error: "insufficient_scope", error_description: ...}`
- **AND** the response MUST include header `WWW-Authenticate: Bearer error="insufficient_scope", scope="sub2api:apikey"`
- **AND** the response MUST NOT leak any API Key data

#### Scenario: Missing or invalid Bearer token

- **WHEN** the request lacks an `Authorization` header OR carries a token that is unknown / revoked / expired / not issued by the OIDC token endpoint
- **THEN** the response MUST be HTTP 401 with header `WWW-Authenticate: Bearer error="invalid_token"` and JSON `{error: "invalid_token", error_description: ...}`

#### Scenario: Provider disabled hides the resource endpoint

- **WHEN** any request reaches `/oidc/resource/api-keys` while `oidc_provider.enabled=false`
- **THEN** the response MUST be HTTP 404 with no OAuth2 error metadata body

### Requirement: Consent Page

The system SHALL display a consent page at `GET /oidc/consent` when a client whose `consent_required=true` requests scopes that are not already covered by a stored consent for the current user, accept the user's allow/deny decision via `POST /oidc/consent`, and persist allow decisions to the `oidc_consent` table.

#### Scenario: Consent page renders requested client and scopes

- **WHEN** the user is signed in and is redirected to `GET /oidc/consent?session=<csrf-bound-token>`
- **THEN** the page displays the client's `client_name`, every requested scope with a human-readable description, and "Allow" / "Deny" buttons
- **AND** the page MUST include a CSRF token validated on `POST`

#### Scenario: Allow decision persists consent and resumes flow

- **WHEN** the user clicks "Allow"
- **THEN** the system MUST upsert `oidc_consent (user_id, client_id, granted_scopes, granted_at, last_used_at)` so that `granted_scopes` is the union of any prior grant and the current request
- **AND** issue an authorization code and redirect to the original `redirect_uri` per the authorization endpoint contract

#### Scenario: Existing consent superset bypasses page

- **WHEN** the authorize flow checks `oidc_consent` for the `(user_id, client_id)` pair
- **AND** the stored `granted_scopes` is a superset of the currently requested scopes
- **THEN** the system MUST skip the consent page and proceed directly to issuing an authorization code

#### Scenario: Incremental scope re-prompts

- **WHEN** a client requests one or more scopes not contained in the stored `granted_scopes` for that user
- **THEN** the consent page MUST be displayed again listing the FULL current request (not just the new scopes), so the user always sees the complete picture

### Requirement: HttpOnly SSO Session Cookie

The system SHALL issue an HttpOnly SSO session cookie named `sub2api_sso` upon every successful user authentication, identify the current user at `/oidc/authorize` via this cookie, and provide a server-side revocation mechanism via the `sso_session` table.

#### Scenario: Cookie is set on every login path

- **WHEN** any login path completes successfully (password login, magic-link email login, external OAuth completion via Linux.do/WeChat/DingTalk/feishu/OIDC client, registration auto-login)
- **THEN** the response MUST include `Set-Cookie: sub2api_sso=<32B base64url session id>; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=2592000` (30 days default; configurable via setting)
- **AND** the system MUST insert a corresponding row into `sso_session` with `user_id`, `issued_at`, `expires_at`, optional `user_agent` and `ip_address`

#### Scenario: Cookie domain follows admin setting

- **WHEN** `oidc_provider.sso_cookie_domain` is set to a non-empty value (e.g. `.sub2api.com`)
- **THEN** the `Set-Cookie` MUST include `Domain=<value>`
- **AND** when the setting is empty, the `Set-Cookie` MUST omit the `Domain` attribute (host-only cookie)

#### Scenario: Authorize identifies user from cookie

- **WHEN** `/oidc/authorize` receives a request bearing a `sub2api_sso` cookie whose value matches an unrevoked unexpired `sso_session` row
- **THEN** the request is treated as authenticated as that row's `user_id`

#### Scenario: Logout revokes the SSO session

- **WHEN** a user calls the logout endpoint
- **THEN** the system MUST set `revoked_at` on the corresponding `sso_session` row
- **AND** the response MUST include `Set-Cookie: sub2api_sso=; Max-Age=0; Path=/`

#### Scenario: Revoked or expired session is rejected

- **WHEN** `/oidc/authorize` receives a `sub2api_sso` cookie matching a row whose `revoked_at IS NOT NULL` or `expires_at < now()`
- **THEN** the request MUST be treated as unauthenticated and follow the "User not signed in" flow

### Requirement: RSA Signing Key Lifecycle

The system SHALL bootstrap an RSA-2048 signing key pair on first startup if none exists, store private keys in the `security_secrets` table under `key='oidc_provider.signing_key.<kid>'`, expose only public keys via JWKS, and support administrator-initiated key rotation that retains retired keys for a 7-day grace period.

#### Scenario: Key auto-generation on first start

- **WHEN** the OIDC provider starts up and no row in `security_secrets` matches prefix `oidc_provider.signing_key.`
- **THEN** the system MUST generate a new RSA-2048 key pair using `crypto/rand`
- **AND** the system MUST persist the PKCS#1 PEM-encoded private key into `security_secrets` with key `oidc_provider.signing_key.<kid>` where `<kid>` is `time.Now().UTC().Format("20060102T150405Z")`
- **AND** the system MUST set setting `oidc_provider.signing_key_active_kid` to that `<kid>`

#### Scenario: Idempotent key load on subsequent starts

- **WHEN** the OIDC provider starts up and `oidc_provider.signing_key_active_kid` already references an existing row
- **THEN** the system MUST load the existing key without generating a new one

#### Scenario: Admin rotates signing key

- **WHEN** an administrator calls `POST /api/v1/admin/oidc/signing-keys/rotate`
- **THEN** the system MUST generate a new RSA-2048 key pair with a fresh `<kid>`
- **AND** persist it in `security_secrets`
- **AND** atomically update `oidc_provider.signing_key_active_kid` to the new `<kid>`
- **AND** record the prior `<kid>`'s retirement timestamp such that JWKS continues to publish its public key for at least 7 days

#### Scenario: Admin deletes retired signing key

- **WHEN** an administrator calls `DELETE /api/v1/admin/oidc/signing-keys/<kid>` and `<kid>` is NOT the currently active kid
- **THEN** the system MUST remove that row from `security_secrets` and the corresponding entry from JWKS responses immediately

#### Scenario: Active key cannot be deleted

- **WHEN** an administrator calls `DELETE /api/v1/admin/oidc/signing-keys/<kid>` and `<kid>` equals the active kid
- **THEN** the system MUST respond HTTP 409 JSON `{error: "active_kid_cannot_be_deleted"}`
- **AND** the active key MUST remain unchanged

### Requirement: OIDC Client Registration via Admin

The system SHALL provide administrator-only HTTP APIs to create, list, update, delete, and reset secrets for OIDC clients, persisting client records in the `oidc_client` table with `client_secret` stored only as a bcrypt hash.

#### Scenario: Create client returns plaintext secret exactly once

- **WHEN** an administrator posts a valid client creation request including `client_name`, `redirect_uris[]`, `allowed_scopes[]`, and `consent_required`
- **THEN** the system MUST generate a fresh `client_id` (string with prefix `rp_`) and a 256-bit `client_secret`
- **AND** the system MUST persist `client_secret_hash = bcrypt(secret, cost=10)` in the database
- **AND** the system MUST return the plaintext `client_secret` exactly once in the creation response
- **AND** any subsequent GET on the client MUST NOT include the plaintext secret

#### Scenario: Reset secret invalidates old secret

- **WHEN** an administrator calls `POST /api/v1/admin/oidc/clients/:id/reset-secret`
- **THEN** the system MUST generate a new 256-bit secret, replace `client_secret_hash` with its bcrypt, and return the new plaintext secret exactly once
- **AND** any subsequent token-endpoint requests using the old secret MUST be rejected with `invalid_client`

#### Scenario: Redirect URIs use exact-match comparison

- **WHEN** a token endpoint or authorize endpoint compares an incoming `redirect_uri` to the stored `redirect_uris` for a client
- **THEN** the comparison MUST be byte-exact (no trailing-slash normalization, no scheme upgrade, no port defaulting, no wildcards)

#### Scenario: Allowed scopes restrict authorize requests

- **WHEN** an authorize request asks for any `scope` not contained in the client's `allowed_scopes`
- **THEN** the system MUST redirect to `<redirect_uri>?error=invalid_scope&error_description=...&state=<state>`

#### Scenario: Disabled client cannot authorize

- **WHEN** any OIDC endpoint receives a request for a client whose `enabled=false`
- **THEN** the request MUST be rejected with `invalid_client` (or redirected with `error=unauthorized_client` if redirect is safe)

#### Scenario: Deleting a client cascades

- **WHEN** an administrator deletes an OIDC client
- **THEN** all `oidc_consent`, `oidc_authorization_code`, and `oidc_refresh_token` rows referencing that client_id MUST be removed in the same transaction

### Requirement: Scope to Claim Mapping

The system SHALL project granted scopes to ID Token claims and UserInfo claims according to a fixed mapping.

#### Scenario: openid scope produces required ID Token claims

- **WHEN** an ID Token is signed with `openid` in scopes
- **THEN** the token MUST contain `iss`, `sub` (string-encoded user_id), `aud` (client_id), `exp`, `iat`, `auth_time`, `acr`, `amr`
- **AND** if the original authorize request carried a `nonce`, the token MUST contain that exact `nonce`

#### Scenario: profile scope adds profile claims

- **WHEN** scopes include `profile`
- **THEN** the ID Token MUST contain `name`, `preferred_username`, `updated_at`
- **AND** when the user has no `username`, both `name` and `preferred_username` MUST default to the local-part of the user's email

#### Scenario: email scope adds email claims

- **WHEN** scopes include `email`
- **THEN** the ID Token MUST contain `email` and `email_verified` (true for sub2api registered emails)

#### Scenario: offline_access scope triggers refresh token issuance

- **WHEN** scopes include `offline_access` and authorization succeeds
- **THEN** the token endpoint response MUST include a `refresh_token` field
- **AND** when scopes do NOT include `offline_access`, the response MUST omit `refresh_token`

#### Scenario: Private scopes appear only in UserInfo

- **WHEN** scopes include `sub2api:apikey`
- **THEN** the corresponding claim (`sub2api_apikey_count`) MUST be absent from the ID Token
- **AND** it MUST be present in the `/oidc/userinfo` response per the UserInfo Endpoint requirement

#### Scenario: acr and amr reflect authentication strength

- **WHEN** a user authenticated using only password (or email magic link)
- **THEN** the ID Token's `acr` MUST equal `"urn:sub2api:authn:basic"` and `amr` MUST equal `["pwd"]`
- **WHEN** a user completed TOTP verification during the same SSO session
- **THEN** the ID Token's `acr` MUST equal `"urn:sub2api:authn:mfa"` and `amr` MUST equal `["pwd","mfa"]`

### Requirement: Admin Configuration Settings

The system SHALL expose the OIDC provider configuration via the existing settings table under the `oidc_provider.*` namespace, validated on read and rejected at startup when the provider is enabled but `issuer_url` is empty.

#### Scenario: Required settings are validated at startup

- **WHEN** the application starts and the setting `oidc_provider.enabled` is `true`
- **AND** the setting `oidc_provider.issuer_url` is empty or missing
- **THEN** the application MUST refuse to serve OIDC endpoints and MUST log a fatal-level error identifying the missing setting

#### Scenario: issuer_url format is enforced

- **WHEN** an administrator submits a value for `oidc_provider.issuer_url`
- **THEN** the system MUST reject the value if it does not start with `https://`, OR if it ends with `/`, OR if it contains a query string or fragment
- **AND** when rejected, the response MUST be HTTP 400 with a message identifying the format rule violated

#### Scenario: Defaults applied when settings are unset

- **WHEN** a setting is unset (not present in the table)
- **THEN** `oidc_provider.access_token_ttl_seconds` defaults to `3600`
- **AND** `oidc_provider.id_token_ttl_seconds` defaults to `3600`
- **AND** `oidc_provider.refresh_token_ttl_seconds` defaults to `2592000` (30 days)
- **AND** `oidc_provider.code_ttl_seconds` defaults to `600` (10 minutes)
- **AND** `oidc_provider.enabled` defaults to `false`
- **AND** `oidc_provider.sso_cookie_domain` defaults to empty (host-only cookie)

### Requirement: Error Response Format

The system SHALL emit OIDC/OAuth2 standards-compliant error codes for all OIDC endpoint failure paths, redirecting errors back to the registered `redirect_uri` whenever it is safe to do so.

#### Scenario: Authorize errors prefer redirect when client and redirect_uri are valid

- **WHEN** authorization fails for a reason other than invalid client_id or invalid redirect_uri
- **THEN** the system MUST redirect 302 to `<redirect_uri>?error=<code>&error_description=<text>&state=<state>` using one of the codes: `invalid_request`, `unauthorized_client`, `access_denied`, `unsupported_response_type`, `invalid_scope`, `server_error`, `temporarily_unavailable`

#### Scenario: Authorize errors return JSON when redirect is unsafe

- **WHEN** authorization fails because `client_id` is unknown or `redirect_uri` does not match a registered URI
- **THEN** the system MUST respond HTTP 400 with JSON `{error: "invalid_client" | "invalid_request", error_description: ...}` and MUST NOT redirect

#### Scenario: Token endpoint always returns JSON errors

- **WHEN** any token endpoint request fails
- **THEN** the response MUST be HTTP 4xx with JSON of shape `{error, error_description?, error_uri?}` using one of: `invalid_request`, `invalid_client`, `invalid_grant`, `unauthorized_client`, `unsupported_grant_type`, `invalid_scope`

#### Scenario: Internal errors do not leak

- **WHEN** an OIDC endpoint encounters a database, signing, or other internal failure
- **THEN** the response MUST use error code `server_error` with a generic `error_description`
- **AND** the underlying root cause MUST NOT appear in the response body
- **AND** the underlying root cause MUST be logged with a trace identifier sufficient for operator diagnosis

