# OpenAI OAuth Same-Account Retries

In **Accounts > Edit**, OpenAI OAuth accounts expose **Same-account retry count**.
The setting is stored in `credentials.pool_mode_retry_count`; pool mode does not
need to be enabled. Spark shadow accounts do not expose this control.

- `0` disables eligible same-account retries, including deadline-based OAuth 429 retries.
- `1` allows one retry after the initial attempt; the supported range is 0 through 10.
- An unset field preserves existing defaults, including the OAuth 429 retry window.
- Leave the input blank to remove an override and restore the existing defaults.
- Saving unrelated fields does not implicitly create an override.
- Explicit positive overrides also cap OAuth 429 retries within their existing deadline.

This setting only caps retries already allowed by the gateway. It does not make
authentication, quota or other non-retryable errors retryable. It does not change
account switching, quota protection, concurrency or session affinity, and is not
a request-wide attempt budget. Account switching can cause additional upstream
attempts. Internal transport reconnects are outside this handler-level limit.

No database migration is required. The scheduler metadata cache preserves the
override without retaining OAuth access tokens.
