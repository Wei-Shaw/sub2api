# Native Feishu enterprise sign-in

Feishu is disabled by default. Configure the server and apply database migrations before enabling it:

```yaml
feishu_connect:
  enabled: true
  client_id: cli_example
  redirect_url: https://your-service.example/api/v1/auth/oauth/feishu/callback
  allowed_tenant_keys:
    - your_verified_tenant_key
```

Set `FEISHU_CONNECT_CLIENT_SECRET` in the server environment, not a browser environment variable. The YAML example intentionally omits the secret; YAML shell variable interpolation is not required. Never commit real credentials. Set the exact same redirect URI in the Feishu developer console. HTTPS is required except for loopback development. At least one allowed tenant is required; missing/foreign tenant identities fail closed.

The login page reads only `feishu_oauth_enabled`; no secret, app configuration or tenant identifiers are sent in public settings. Existing native registration, invitation, email-verification, MFA, browser-bound pending-session, account-binding and last-login-method policies continue to apply. This feature does not bypass closed registration or grant migration-specific credit.

Authorization uses Feishu v1 authorize, v2 JSON token exchange and v1 user-info endpoints. Redirects from token/user-info endpoints are refused and responses are bounded. Provider response bodies and credentials are excluded from errors.

Identity keys use the configured client ID and an opaque SHA-256 of tenant key, NUL separator and the verified stable user identifier. `union_id` is preferred for new identities; verified `open_id`/`user_id` candidates may resolve existing identities. Multiple different owners fail for administrator reconciliation. Provider email is never used to automatically claim a local account. Linking an existing local account uses the existing authenticated/password-proven pending binding flow.

A deployment replacing an existing identity system must pre-map stable identities to target users before enabling sign-in. Numeric IDs or equal email addresses are not identity proof. Back up and reconcile the target database; do not repeatedly initialize balances on login or deployment.

Relevant upstream endpoints are documented at https://open.feishu.cn/document/authentication-management/access-token/get-user-access-token and https://open.feishu.cn/document/authentication-management/login-state-management/get .
