# SETTINGS-V2 - Deploy Playbook (test.clicodeplus.com)

> Companion to docs/plugin-architecture/SETTINGS-V2-DESIGN.md Section 10
> (Launch and rollback playbook) and docs/plugin-architecture/SETTINGS-V2-VERIFY.md
> Section 4 (E2E checklist).
>
> Target environment: test.clicodeplus.com (Test environment, port 8087,
> container sub2api-test, deploy directory /root/sub2api-plugin, image tag
> sub2api:test). Uses isolated PG/Redis containers - does not touch the
> shared db.clicodeplus.com cluster.
>
> Branch shipped: feat/plugin-system-fixes (head 9494f228 feat(settings-v2/W4-A):
> channel-management demo schema with V2 markers).
>
> Operate from the dev workstation; all commands run via SSH alias
> clicodeplus. The shared CLAUDE.md "Test 环境特例" notes apply.

---

## 0. Pre-flight (one-time per session)

```bash
# 1. Confirm SSH reachability
ssh clicodeplus "echo READY && uname -a"

# 2. Confirm test compose project exists with the expected names
ssh clicodeplus "docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}' | grep -E 'sub2api-test'"
# expected: sub2api-test (sub2api:test), sub2api-test-postgres (postgres),
#           sub2api-test-redis (redis)

# 3. Confirm port 8087 is the test app
ssh clicodeplus "ss -ltnp | grep ':8087' || echo '8087 free'"

# 4. Snapshot rollback target (current HEAD on test deploy)
ssh clicodeplus "cd /root/sub2api-plugin && git rev-parse HEAD" > /tmp/settings-v2-rollback-head.txt
cat /tmp/settings-v2-rollback-head.txt
# write this hash down - it is the rollback anchor for Section 3
```

If any pre-flight check fails, **stop**: do not begin Phase 1 until the
test environment is in a known good state.

---

## 1. Phase 1 - Apply commits + DB migration

The four DESIGN Section 10 logical phases (DB / SDK / host service / UI) are
**all already merged** into `feat/plugin-system-fixes` as 25 atomic commits
between `5f8f40b7` (W1-A migration) and `9494f228` (W4-A demo schema).
On the test environment we deploy them as a single image build because
the migration runner inside `docker-entrypoint.sh` applies SQL files in
order on every container start. The commit-by-commit boundaries are
preserved in git for rollback purposes (see Section 3).

### 1.1 Pull, build, restart

```bash
# 1) Sync code on the test server
ssh clicodeplus "cd /root/sub2api-plugin && \
    git fetch origin feat/plugin-system-fixes && \
    git reset --hard origin/feat/plugin-system-fixes && \
    git log --oneline -1"
# expected: 9494f228 feat(settings-v2/W4-A): channel-management demo schema ...

# 2) Build with the limited builder (CLAUDE.md mandates)
ssh clicodeplus "cd /root/sub2api-plugin && \
    docker buildx build --builder limited-builder --no-cache --load \
        -t sub2api:test -f Dockerfile ."
# Wait for "Successfully tagged sub2api:test". Failure -> stop and report.

# 3) Recreate the app container (PG / Redis containers stay up)
ssh clicodeplus "cd /root/sub2api-plugin/deploy && \
    docker compose -p sub2api-test --env-file .env up -d --force-recreate sub2api"
```

### 1.2 Verify migration 103 applied

```bash
# A) Migration row recorded
ssh clicodeplus "docker exec sub2api-test-postgres \
    psql -U plugin -d plugin -c \
    \"SELECT filename, applied_at FROM schema_migrations \
       WHERE filename LIKE '103_%' ORDER BY applied_at DESC LIMIT 1;\""
# expected: 103_plugin_settings_v2.sql ... <recent timestamp>

# B) New columns present
ssh clicodeplus "docker exec sub2api-test-postgres \
    psql -U plugin -d plugin -c \
    \"SELECT table_name, column_name, data_type, column_default \
       FROM information_schema.columns \
       WHERE table_name IN ('plugin_settings', 'plugin_settings_schemas') \
         AND column_name IN ('schema_version', 'schema_version_at_write', 'properties_meta') \
       ORDER BY table_name, column_name;\""
# expected 3 rows:
#   plugin_settings           | schema_version_at_write | text  | '0'::text
#   plugin_settings_schemas   | properties_meta         | jsonb | '{}'::jsonb
#   plugin_settings_schemas   | schema_version          | text  | '0'::text

# C) New index present
ssh clicodeplus "docker exec sub2api-test-postgres \
    psql -U plugin -d plugin -c \"\d plugin_settings\" | grep idx_plugin_settings_schema_version_at_write"
# expected: idx_plugin_settings_schema_version_at_write btree (plugin_name, schema_version_at_write)
```

If any verify fails, **rollback** per Section 3 immediately and triage in dev.

### 1.3 Container health

```bash
ssh clicodeplus "docker logs sub2api-test --tail 80"
# look for:
#   - 'plugin "channel-management" registered' or equivalent
#   - 'plugin_settings: registered schema' (the new RegisterSchema path)
# do not proceed if you see ERROR / panic lines
ssh clicodeplus "docker ps --filter name=sub2api-test --format '{{.Names}}: {{.Status}}'"
# expected: sub2api-test: Up X seconds (healthy)
```

---

## 2. Phase 2 - Functional verification (admin API + UI)

> Reads ADMIN_API_KEY from the local .env. Set BASE and KEY once.

```bash
source .env
export BASE="https://test.clicodeplus.com"   # or http://<ip>:8087
export KEY="$ADMIN_API_KEY"
```

### 2.1 GET schema_version + properties_meta

```bash
curl -s "${BASE}/api/v1/admin/plugin-settings/channel-management" \
    -H "x-api-key: ${KEY}" \
  | jq '.data | {schema_version, properties_meta_keys: (.properties_meta // {} | keys), secret_keys}'
# Expected:
#   - schema_version == "1.0.0"
#   - properties_meta_keys includes:
#       defaultIntervalSec / dailyRollupHourUTC / internalUpstreamProbeKey
#       / _internalCacheTTLSec
#   - secret_keys initially [] (no value PUT yet)
```

### 2.2 Secret PUT round-trip

```bash
curl -X PUT "${BASE}/api/v1/admin/plugin-settings/channel-management/internalUpstreamProbeKey" \
    -H "x-api-key: ${KEY}" -H "Content-Type: application/json" \
    -d '{"value": "test-secret-123"}' -w "\nHTTP %{http_code}\n"
# expected: HTTP 200

curl -s "${BASE}/api/v1/admin/plugin-settings/channel-management" \
    -H "x-api-key: ${KEY}" \
  | jq '{value: .data.values.internalUpstreamProbeKey, secret_keys: .data.secret_keys}'
# expected: { "value": null, "secret_keys": ["internalUpstreamProbeKey"] }
```

### 2.3 Backend-only field is rejected

```bash
curl -X PUT "${BASE}/api/v1/admin/plugin-settings/channel-management/_internalCacheTTLSec" \
    -H "x-api-key: ${KEY}" -H "Content-Type: application/json" \
    -d '{"value": 30}' -w "\nHTTP %{http_code}\n"
# expected: HTTP 403 with body containing
#   "metadata":{"code":"PLUGIN_SETTINGS_BACKEND_ONLY"}
```

### 2.4 Secret cleared deletes the row

```bash
curl -X PUT "${BASE}/api/v1/admin/plugin-settings/channel-management/internalUpstreamProbeKey" \
    -H "x-api-key: ${KEY}" -H "Content-Type: application/json" \
    -d '{"value": ""}' -w "\nHTTP %{http_code}\n"
# expected: HTTP 200

curl -s "${BASE}/api/v1/admin/plugin-settings/channel-management" \
    -H "x-api-key: ${KEY}" | jq '.data.secret_keys'
# expected: []   (key removed)
```

### 2.5 requires_reload trigger

```bash
ssh clicodeplus "docker logs sub2api-test --since 1m | wc -l" > /tmp/_before.txt

curl -X PUT "${BASE}/api/v1/admin/plugin-settings/channel-management/defaultIntervalSec" \
    -H "x-api-key: ${KEY}" -H "Content-Type: application/json" \
    -d '{"value": 120}' -w "\nHTTP %{http_code}\n"
# expected: HTTP 200

sleep 3
ssh clicodeplus "docker logs sub2api-test --since 30s | grep -E 'plugin reload triggered|requires_reload|coalesce'"
# expected: at least one log line referencing reload / channel-management
```

### 2.6 UI manual smoke

Navigate to ${BASE}/admin/plugins, select the **channel-management**
card and switch to the SETTINGS tab. Confirm:

| Property | Expected widget / decorator |
|----------|-----------------------------|
| `defaultIntervalSec` | NumberInput + orange `RequiresReloadBadge` (i18n: 需要重启插件 / Requires plugin reload) |
| `templateMaxBodyKB` | NumberInput, no decorators |
| `dailyRollupHourUTC` | strikethrough text + yellow `DeprecatedBadge` (i18n: 已废弃) |
| `internalUpstreamProbeKey` | SecretInput (input type=password); placeholder switches to "已配置, 留空保持原值" once a value has been PUT |
| `_internalCacheTTLSec` | **NOT rendered** (visibility=backend filtered out by the widget map) |
| Form title | shows `schema_version 1.0.0` badge |

Optional: re-run Section 2.5 from the UI by changing `defaultIntervalSec`
from 60 to 120 via the form, save, and watch the container log - same
reload expectation.

### 2.7 Watch-stream resilience (optional)

From the admin UI, restart the channel-management plugin (the stop/start
buttons on the plugin card). After it comes back, repeat Section 2.1:
response should be unchanged and no `SchemaVersionMismatchError` line
should appear in the container log (the SDK `sendSnapshot` rehydration
is W3-B and should be silent on a clean reconnect).

---

## 3. Phase 3 - Rollback

The release is a single image (`sub2api:test`) plus a forward-only DDL
(migration 103). Two rollback levels exist:

### 3.1 Code-only rollback (no DDL change)

Use this if the new code is misbehaving but the DB extension is
harmless. Migration 103 is forward-compatible: the old code ignores the
new columns; nothing breaks if the columns simply sit unused.

```bash
PREV_HEAD=$(cat /tmp/settings-v2-rollback-head.txt)

ssh clicodeplus "cd /root/sub2api-plugin && \
    git fetch origin && \
    git reset --hard ${PREV_HEAD} && \
    git log --oneline -1"

ssh clicodeplus "cd /root/sub2api-plugin && \
    docker buildx build --builder limited-builder --no-cache --load \
        -t sub2api:test -f Dockerfile ."

ssh clicodeplus "cd /root/sub2api-plugin/deploy && \
    docker compose -p sub2api-test --env-file .env up -d --force-recreate sub2api"

ssh clicodeplus "docker ps --filter name=sub2api-test --format '{{.Names}}: {{.Status}}'"
ssh clicodeplus "docker logs sub2api-test --tail 40"
```

### 3.2 Full rollback (also drop the new columns)

Required only if migration 103 itself is the smoking gun (e.g. a column
type collides with downstream code in a future release branch). Apply
the documented `ROLLBACK SQL` from `SETTINGS-V2-DESIGN.md` Section 1.3.

Stage the rollback SQL to a file on the dev workstation first (avoiding
nested-heredoc shell-quoting issues), then scp + apply on the test server:

```bash
# 1) Stop the app first to prevent any new writes
ssh clicodeplus "cd /root/sub2api-plugin/deploy && \
    docker compose -p sub2api-test stop sub2api"

# 2) Stage the rollback SQL on the dev workstation
cat > /tmp/rollback_103.sql <<'SQL_EOF'
BEGIN;
DROP INDEX IF EXISTS idx_plugin_settings_schema_version_at_write;
ALTER TABLE plugin_settings        DROP COLUMN IF EXISTS schema_version_at_write;
ALTER TABLE plugin_settings_schemas DROP COLUMN IF EXISTS properties_meta;
ALTER TABLE plugin_settings_schemas DROP COLUMN IF EXISTS schema_version;
DELETE FROM schema_migrations WHERE filename = '103_plugin_settings_v2.sql';
COMMIT;
SQL_EOF

# 3) Push to test server and apply
scp /tmp/rollback_103.sql clicodeplus:/tmp/rollback_103.sql
ssh clicodeplus "docker exec -i sub2api-test-postgres psql -U plugin -d plugin < /tmp/rollback_103.sql"

# 4) Then perform Section 3.1 code rollback to PREV_HEAD and start the container
```

> **Warning**: Section 3.2 deletes the per-row `schema_version_at_write`
> data, so any version-drift detection that ran against the new code
> will not be re-derivable. This is acceptable for the test
> environment; production rollback would require a backup snapshot
> first.

---

## 4. Sign-off Checklist

Tick all items before declaring SETTINGS-V2 deployed to test:

- [ ] Pre-flight Section 0 - all green
- [ ] Phase 1.1 - image built, container recreated, no error in `docker logs`
- [ ] Phase 1.2 - migration 103 row present, 3 new columns present, new index present
- [ ] Phase 2.1 - `schema_version`, `properties_meta`, `secret_keys` returned by GET
- [ ] Phase 2.2 - secret round-trip works (value masked, key in `secret_keys`)
- [ ] Phase 2.3 - backend-only PUT returns 403 + `PLUGIN_SETTINGS_BACKEND_ONLY`
- [ ] Phase 2.4 - empty-string PUT removes secret from `secret_keys`
- [ ] Phase 2.5 - requires-reload PUT triggers a reload log line
- [ ] Phase 2.6 - UI shows all 5 expected widgets / decorators
- [ ] Phase 2.7 - (optional) watch-stream reconnect silent
- [ ] Section 0 step 4 rollback HEAD captured and stored

If any item fails, jump to Section 3 (start with Section 3.1; only
escalate to Section 3.2 if DDL is implicated).
