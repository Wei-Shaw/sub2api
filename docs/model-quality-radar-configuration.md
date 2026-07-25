# Model Quality Radar Configuration

Model Quality Radar uses dedicated evaluation identities. Do not reuse a customer user, group, or API key. Apply database migrations before enabling Radar; migration `190_add_radar_route_evidence.sql` adds the evaluation-key flag and evidence table.

## Server configuration

Set all six variables on every API server instance:

```bash
RADAR_ENABLED=true
RADAR_CONTEXT_SIGNING_KEY=<at-least-32-random-bytes>
RADAR_EVIDENCE_HASH_KEY=<different-at-least-32-random-bytes>
RADAR_REGION=cn-east
RADAR_ROUTE_PROFILE_VERSION=route-v42
RADAR_MAX_CONTEXT_TTL_SECONDS=300
```

`RADAR_CONTEXT_SIGNING_KEY` verifies evaluator-issued request contexts. `RADAR_EVIDENCE_HASH_KEY` creates stable HMAC references for account and channel IDs; it must be different from the signing key. Store both in the deployment secret manager, never in source control or evaluator output. The legacy names `RADAR_SIGNING_SECRET` and `RADAR_HASHING_SECRET` remain accepted, but the names above take precedence.

`RADAR_MAX_CONTEXT_TTL_SECONDS` must be between 1 and 900. `RADAR_REGION` and `RADAR_ROUTE_PROFILE_VERSION` are required when Radar is enabled and become evidence dimensions. Restart all API instances after changing these values. Rotating the signing key invalidates outstanding evaluation tokens; rotating the evidence hash key changes future redacted resource references.

## Provision an isolated identity

The examples use the existing API conventions. Replace the host, credentials, limits, and IDs for the deployment. Keep the nonzero limits: an unlimited evaluation key defeats isolation.

1. Create a dedicated exclusive group as an administrator:

```bash
curl -fsS -X POST "$SUB2API_URL/api/v1/admin/groups" \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "model-quality-radar",
    "description": "Dedicated model quality evaluation traffic",
    "platform": "openai",
    "rate_multiplier": 1,
    "is_exclusive": true,
    "subscription_type": "standard",
    "rpm_limit": 10
  }'
```

Record the returned group ID as `RADAR_GROUP_ID`. Attach only the upstream accounts/channels intended for this route profile using the normal group/account administration APIs. Do not add customer keys to this group.

2. Create a dedicated user and grant only that group. User concurrency and RPM are independent safeguards:

```bash
curl -fsS -X POST "$SUB2API_URL/api/v1/admin/users" \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H 'Content-Type: application/json' \
  -d "{
    \"email\": \"radar-evaluator@example.invalid\",
    \"password\": \"$RADAR_USER_INITIAL_PASSWORD\",
    \"username\": \"radar-evaluator\",
    \"notes\": \"Dedicated model quality evaluation identity\",
    \"role\": \"user\",
    \"balance\": 10,
    \"concurrency\": 1,
    \"rpm_limit\": 10,
    \"allowed_groups\": [$RADAR_GROUP_ID]
  }"
```

Record the returned user ID as `RADAR_USER_ID`. Authenticate as this user through the deployment's normal sign-in flow and store the resulting JWT as `RADAR_USER_JWT`.

3. Create one key as the dedicated user. The API configures its group, quota, expiry, and rolling spend limits:

```bash
curl -fsS -X POST "$SUB2API_URL/api/v1/keys" \
  -H "Authorization: Bearer $RADAR_USER_JWT" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: radar-key-provision-v1" \
  -d "{
    \"name\": \"model-quality-radar\",
    \"group_id\": $RADAR_GROUP_ID,
    \"quota\": 10,
    \"expires_in_days\": 30,
    \"rate_limit_5h\": 1,
    \"rate_limit_1d\": 2,
    \"rate_limit_7d\": 5
  }"
```

Record the returned key ID as `RADAR_API_KEY_ID` and the generated key in the evaluator's secret manager. Do not send an inference request yet.

4. The public API intentionally cannot grant evaluation status. Before first use, mark the exact key in PostgreSQL and reassert every isolation limit in one transaction:

```sql
BEGIN;

UPDATE api_keys AS k
SET is_evaluation = TRUE,
    group_id = :radar_group_id,
    quota = 10.00000000,
    quota_used = 0,
    rate_limit_5h = 1.00000000,
    rate_limit_1d = 2.00000000,
    rate_limit_7d = 5.00000000,
    usage_5h = 0,
    usage_1d = 0,
    usage_7d = 0,
    updated_at = NOW()
FROM users AS u, groups AS g
WHERE k.id = :radar_api_key_id
  AND k.user_id = :radar_user_id
  AND u.id = k.user_id
  AND u.email = 'radar-evaluator@example.invalid'
  AND u.status = 'active'
  AND u.concurrency = 1
  AND g.id = :radar_group_id
  AND g.is_exclusive = TRUE
  AND g.status = 'active'
RETURNING k.id, k.user_id, k.group_id, k.is_evaluation,
          k.quota, k.rate_limit_5h, k.rate_limit_1d, k.rate_limit_7d;

COMMIT;
```

The `UPDATE ... RETURNING` must return exactly one row. If it returns none or more than one, roll back and correct the IDs. Because evaluation status is changed directly in SQL, perform this step before the key has ever been authenticated. If an existing key is converted, restart the API servers after the transaction so no authentication cache can retain the old flag.

Verify the final isolation state:

```sql
SELECT k.id, k.is_evaluation, k.status, k.quota, k.quota_used,
       k.rate_limit_5h, k.rate_limit_1d, k.rate_limit_7d,
       u.id AS user_id, u.concurrency, u.rpm_limit,
       g.id AS group_id, g.is_exclusive, g.rpm_limit
FROM api_keys AS k
JOIN users AS u ON u.id = k.user_id
JOIN groups AS g ON g.id = k.group_id
WHERE k.id = :radar_api_key_id;
```

## Evaluation requests

The evaluator signs a short-lived context bound to `RADAR_API_KEY_ID` and sends it in `X-Sub2API-Evaluation-Token` with the normal API-key credential. Claims include a unique run ID, sample ID, dataset version, expected public model alias, route profile, API key ID, issue time, and expiry time. The server generates the route trace ID; clients cannot supply it.

A normal key carrying evaluation headers is rejected. An evaluation key without a valid, unexpired token bound to that key is rejected before inference. Do not retry either rejection as ordinary traffic.

Route evidence is best-effort and contains routing identifiers only as HMAC references. It must never contain prompts, completions, hidden reasoning, credentials, raw account IDs, raw channel IDs, or arbitrary upstream error text. Operational access to `evaluation_route_evidence` should be restricted to the Radar reader and database administrators.
