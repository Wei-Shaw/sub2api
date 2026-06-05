# SETTINGS-V2-INSPECT (Inspector B — current-state audit)

> Read-only inspection. All claims carry `file:line` references. Working tree: `feature+plugin-grpc`.
>
> **Headline finding**: Path B (`PluginRecord.Config` → `ctx.Config()`) is *almost entirely vestigial*. Zero plugins read it; only one host code-path reads one key (`skip_migration`); zero frontend code writes it. "Cutting B and merging into A" is a small, well-scoped change. The bigger work hides in **Path A schema v1 capability gaps** (no `secret`, no `requires_restart`, no `deprecated`, no schema version), not in Path B amputation.

---

## 1. Path B usage inventory

### 1.1 SDK surface (consumer side)

| Surface | File:Line | Status |
|---|---|---|
| Interface declaration | `plugin-sdk/context.go:35` (`Config() map[string]string`) | Exposed to plugins |
| Concrete impl | `plugin-sdk/runner.go:675-681` (`pluginCtx.Config()` returns a defensive copy) | Wired |
| Wire-in from gRPC | `plugin-sdk/runner.go:392-395` (copies `req.GetConfig()` into `cfgCopy`) | Active |
| Proto field | `plugin-sdk/proto/plugin.proto:32` (`map<string,string> config = 2;`) | Stable |

### 1.2 Plugin-side `ctx.Config()` callers — **NONE**

A repo-wide grep for `ctx.Config()` and `pctx.Config()` (Grep across all `*.go`):

- **0 matches** in `plugins/` (both `hello-world` and `channel-management`)
- **0 matches** in any external code

Neither built-in plugin reads its own `Config` map. Verified by `Grep` for `ctx\.(Config|Logger|DB|Redis)` showing only `Logger/DB/Redis/Secrets/Jobs/Settings` calls in:
- `plugins/hello-world/main.go:134, 214, 237` — only Logger/DB/Redis
- `plugins/channel-management/plugin.go:268, 286, 289, 298, 308, 313, 324, 326, 334, 340` — only DB/Redis/Logger/Secrets/Jobs/Settings

The only `ctx.Settings().GetTyped` reader is `plugins/hello-world/main.go:189` (Path A), confirming both built-ins use Path A.

### 1.3 Host-side `pluginConfig` consumers — **ONE key only**

`pluginConfig` is the host-internal name for the same `map[string]string`. Grep results:

| Reader | File:Line | Key used | Purpose |
|---|---|---|---|
| Migration runner escape hatch | `backend/internal/plugin/migration_proxy_server.go:121, 127-132` | `skip_migration` (string `"true"` strict-equal) | Operator escape hatch to bypass plugin migrations when broken |
| Manager calling that check | `backend/internal/plugin/manager.go:882-886` | (same) | Logs + skips `fetchAndRunPluginMigrations` |

The only reader is `shouldSkipPluginMigrations(pluginConfig)`. **No other host code path reads any other key from `PluginRecord.Config`.**

### 1.4 Path B write path

| Step | File:Line | Notes |
|---|---|---|
| HTTP route | `backend/internal/server/routes/admin.go:563` (`plugins.PUT("/:name/config", h.Admin.Plugin.UpdateConfig)`) | Registered but unused by frontend |
| Handler | `backend/internal/handler/admin/plugin_handler.go:140-157` (`PluginHandler.UpdateConfig`) | Body shape: `{"config": map[string]any}` |
| Manager | `backend/internal/plugin/manager.go:509-515` (`PluginManager.UpdateConfig`) | Persists only; **does not restart** |
| `any → string` coercion | `backend/internal/plugin/manager.go:519-538` (`configToString`) | Booleans/numbers via `fmt.Sprintf("%v")`; complex via `json.Marshal` (round-trip is lossy for booleans: `true → "true"`) |
| Repository | `backend/internal/plugin/repository.go:177-192` (`UpdateConfig`) | Whole-row JSONB replace |
| Initial seeding | `backend/internal/plugin/manager.go:272-276` (`Config: map[string]string{}`) on first discovery | Default is empty map |

### 1.5 Frontend Path B writers — **NONE**

- Grep `'/admin/plugins/.*/config'` across `frontend/`: **0 matches**
- Grep `UpdateConfig|updateConfig` for plugin admin: **0 matches**
- `frontend/src/views/admin/PluginsView.vue:296-323` only **renders** `detail.config` as a read-only key/value table (`detailConfigEntries` computed, no save button)
- `frontend/src/api/admin/pluginSettings.ts` only talks to W3 endpoints (`/admin/plugin-settings/...`); no `/admin/plugins/:name/config` client

So `PluginRecord.Config` is, in practice, write-only for ops via direct curl — the UI never writes to it and only displays it.

### 1.6 PluginRecord.Config GET/exposure to admin API

`backend/internal/plugin/manager.go:436-447, 451-460` — `mergeRecordIntoInfo` + `configToAny` projects the map into `PluginInfo.Config` as `map[string]any`, served by `GET /admin/plugins/:name`. PluginsView read-only table consumes this.

### 1.7 Per-plugin Path B usage matrix

| Plugin | Calls `ctx.Config()`? | Keys read | Default values defined? | Set via UI? |
|---|---|---|---|---|
| `hello-world` (built-in, `plugins/hello-world/main.go:88-127`) | No | — | n/a | n/a |
| `channel-management` (built-in, `plugins/channel-management/plugin.go:105-260, 265-342`) | No | — | n/a | n/a |
| External plugins | None present in tree (only `plugins/hello-world` and `plugins/channel-management` exist) | — | — | — |
| **Host-internal reader** | `shouldSkipPluginMigrations` reads `skip_migration` | `skip_migration` (one key) | Not seeded; absent ⇒ false | Only via curl |

**Net: Path B has exactly one observable consumer, `skip_migration`, and it is a host-side ops escape hatch — not a plugin-facing config.**

---

## 2. Path A capability inventory

### 2.1 SDK surface (`plugin-sdk/settings.go`)

| API | File:Line | Behavior |
|---|---|---|
| `SettingsClient.Get(ctx, key) (json.RawMessage, error)` | `settings.go:75, 184-215` | Cached 30s (`settingsCacheTTL` const, line 43); 5s RPC timeout (line 47); returns `ErrSettingNotFound` (line 38) on miss |
| `SettingsClient.GetTyped(ctx, key, out) error` | `settings.go:80, 217-226` | `Get` + `json.Unmarshal` |
| `SettingsClient.Watch(ctx, key) (<-chan SettingsChange, func(), error)` | `settings.go:87, 228-252` | Empty key = whole namespace; auto-cleanup on ctx done; bounded channel (buffer 8, line 232) — drops events on full |
| `nilSettingsClient` | `settings.go:127-139` | Used when capability not approved; every method returns explicit error (no silent no-op) |
| Reconnect loop | `settings.go:270-303` (`runWatchLoop`) | Exponential backoff 1s → 30s (constants line 53-56); reset on successful open |

**No `SetTyped`/`Set` exists** — `settings.go:69-71` documents this is intentional: writes always go through admin UI to "make configuration drift from runtime code impossible".

### 2.2 Database (`backend/migrations/102_plugin_settings.sql`)

| Table | Columns / Constraints | Notes |
|---|---|---|
| `plugin_settings` | `plugin_name VARCHAR(64)`, `key TEXT`, `value_json JSONB`, `revision BIGINT NOT NULL DEFAULT 1`, `updated_at TIMESTAMPTZ`. PK `(plugin_name, key)`. Index `idx_plugin_settings_updated_at` | Per-key row, monotonic revision |
| `plugin_settings_schemas` | `plugin_name VARCHAR(64) PRIMARY KEY`, `schema_json JSONB`, `defaults_json JSONB DEFAULT '{}'::jsonb`, `updated_at TIMESTAMPTZ` | One row per plugin; **no schema version column**, **no historical schemas** |

The migration file is 28 lines total — there is no flag column for "secret", "deprecated", or "kind". Storage is "key as string, value as opaque JSONB blob". Both tables are V5/W3-fresh.

### 2.3 Host service (`backend/internal/service/plugin_settings_service.go`)

| Function | Line | Behavior |
|---|---|---|
| `RegisterSchema` | 132-183 | Compiles schema via `jsonschema.CompileString` (line 449-450), upserts into `plugin_settings_schemas`, seeds defaults (line 178) |
| `seedDefaults` | 187-205 | `INSERT … ON CONFLICT DO NOTHING` per default key; preserves existing values |
| `UnregisterSchema` | 210-216 | Clears in-memory cache only; **stored values left intact** for re-enable |
| `GetByKey` / `GetAll` | 218-256 | Plain SELECT; missing key → `sql.ErrNoRows` |
| `SetByKey` | 261-298 | Validates against compiled schema (line 274); upserts with `revision+1`; fans out via `notify` |
| `SchemaInfo` | 302-345 | Returns `{schema, defaults, values, updated_at}`; lazy-loads schema from DB if cache cold |
| `validateAgainst` | 427-443 | Wraps single key into `{key: value}` and runs full `Schema.Validate` — meaning top-level `required` arrays still enforce on partial saves only when the partial object happens to satisfy them |
| `compileSchema` | 448-450 | `santhosh-tekuri/jsonschema/v5` — accepts Draft 07 + Draft 2020-12 (per import & SETTINGS_API.md:91-94) |

Compiler: `santhosh-tekuri/jsonschema/v5` supports the full standard keyword set (`type`, `enum`, `const`, `format`, `pattern`, `minimum`/`maximum`/`exclusiveMinimum`, `minLength`/`maxLength`, `minItems`/`maxItems`, `uniqueItems`, `required`, `properties`, `additionalProperties`, `anyOf`/`oneOf`/`allOf`/`not`, `$ref`, etc.). **All validation server-side is full Draft-07.** The bottleneck is the UI renderer, not the validator.

### 2.4 gRPC bridge (`backend/internal/plugin/settings_extension_server.go`)

| RPC | Line | Behavior |
|---|---|---|
| `Get` | 74-98 | Resolves caller via metadata; rejects anonymous with `PermissionDenied`; returns `Exists=false` on miss (no error) |
| `Watch` (server-streaming) | 115-149 | Sends snapshot on attach (lines 126, 153-184), then pumps `Subscribe()` events |
| `sendSnapshot` | 153-184 | Single-key or full-namespace snapshot |
| `ResolveCaller` | 191-193 | Re-exports `SDKServer.resolveCaller` for wiring |

Trust boundary: `x-sub2api-plugin` metadata header (`plugin-sdk/runner.go:36`) — same as every other SDK service.

### 2.5 Admin REST surface

`backend/internal/handler/admin/plugin_settings_handler.go`:

| Method | Path | Line | Behavior |
|---|---|---|---|
| `List` | `GET /api/v1/admin/plugin-settings` | 51-64 | Lists registered plugins |
| `Get` | `GET /api/v1/admin/plugin-settings/:plugin` | 66-86 | Returns `{schema, defaults, values, updated_at}` (404 when no schema) |
| `Update` | `PUT /api/v1/admin/plugin-settings/:plugin/:key` | 92-133 | Body `{"value": <raw JSON>}`; 422 on validation, 409 on missing schema, 200 with new revision |

### 2.6 Admin UI renderer (`frontend/src/components/admin/PluginSettingsForm.vue`)

| Schema keyword | Supported? | Where (file:line) |
|---|---|---|
| `type: "boolean"` | Yes (checkbox) | `PluginSettingsForm.vue:27-32` |
| `type: "string"` | Yes (text input) | `PluginSettingsForm.vue:54-60` |
| `type: "number"` / `"integer"` | Yes (number input; `step=1` for integer, `step=any` otherwise) | `PluginSettingsForm.vue:45-52` |
| `enum` | Yes (`<select>`; serializes back via `String(opt)` match — line 178-181) | `PluginSettingsForm.vue:34-43`; descriptor build at `127, 134` |
| `title`, `description` | Yes (rendered as label and small description) | `PluginSettingsForm.vue:13-22` |
| `default` | **Read by host on `seedDefaults`, NOT by UI**; UI only displays whatever `values` already contains | UI does not look at schema `default` — `seedDefaults` does (`plugin_settings_service.go:187-205`) |
| `type: "object"` / `"array"` | Falls through to `<textarea>` raw-JSON editor | `PluginSettingsForm.vue:62-68, 191-204` (`onJsonInput`) |
| `minimum` / `maximum` / `pattern` / `format` / `minLength` / `maxLength` / `multipleOf` | **Validated server-side** (jsonschema lib accepts them); **NOT surfaced in UI** (no client-side hint, no `min`/`max`/`pattern` attribute pushed onto the inputs) | `PluginSettingsForm.vue` has no constraint reads beyond `enum` |
| `required` | Validated server-side per-key (somewhat awkwardly; see §4) | `plugin_settings_service.go:427-443` |
| Vendor keywords (`x-*`, `format: password`, `writeOnly`, `secret`, `deprecated`, `requiresRestart`) | **None** | Grep across `settings.go`, `plugin_settings_service.go`, `PluginSettingsForm.vue`, `settings_extension_server.go` returns 0 matches for any of those terms |

Specifically, `PluginSettingsForm.vue:118-137` builds a `PropDescriptor` with only `{key, type, title, description, enumValues}` — it ignores every other keyword.

### 2.7 Capability wire-up

| Constant | File:Line | Behavior |
|---|---|---|
| `CapabilitySettingsExtension` | `plugin-sdk/manifest.go:55` | Auto-promoted at `manifest.go:239-241` if plugin ships `SettingsSchema` |
| Built-in users | `plugins/hello-world/main.go:122-125`, `plugins/channel-management/plugin.go:143, 162-165` | Both built-ins use Path A |

### 2.8 Three-layer summary

| Layer | Capability | Limitation |
|---|---|---|
| **SDK** | Get / GetTyped / Watch (with cache + reconnect) | No SDK-side Set; no per-key TTL config; cache invalidation only on Watch events or 30s TTL |
| **DB** | Per-key JSONB storage with monotonic revision; per-plugin schema row | No schema version column; no per-key metadata (kind/secret/deprecated); no historical schema versions |
| **UI** | boolean checkbox, enum select, number/integer input, string text, object/array textarea | No `format: password`, no min/max constraint surface, no pattern hint, no required-mark, no vendor hint reading; saves are per-key (UI button per row) |

---

## 3. B → A migration cost (per-key)

There is exactly one Path B key in active use, plus zero plugin-facing keys. The migration table is therefore very short:

| B key | Reader | Currently startup-only? | Schema-shaped? | Default? | Secret? | Migration verdict |
|---|---|---|---|---|---|---|
| `skip_migration` | Host-internal — `migration_proxy_server.go:121-132` (read by host before plugin start, not by plugin) | Yes (read once at `manager.go:882`, before plugin binary spawns) | Trivial: `{type: "boolean", default: false}` | "false"/missing today | No | **Cannot move to Path A as-is.** Path A is per-plugin, scoped after `RegisterSchema` runs (which itself runs *during* plugin start). This key gates a step that happens before the plugin's manifest has been processed; a plugin-namespaced setting cannot, by construction, gate its own migration. Either keep Path B alive for this single host-side ops key, OR make the host read it from a **host-owned ops surface** (e.g., `ops_settings` table) rather than per-plugin config. |
| (anything else) | None | n/a | n/a | n/a | n/a | n/a |

### Implications

- **Plugin-facing migration cost: $0 work.** No plugin reads `ctx.Config()`. Removing `Config()` from `PluginContext` (`plugin-sdk/context.go:35`), `pluginCtx.config` (`runner.go:665`), `pluginCtx.Config()` (`runner.go:675-681`), and the `cfgCopy` wiring (`runner.go:392-395, 461`) compiles cleanly with zero plugin updates. Both built-ins compile unchanged (verified: zero `ctx.Config()` callers anywhere in the repo).
- **Host-facing migration cost: 1 key, special-cased.** `skip_migration` is read at `manager.go:882` *before* the plugin's `RegisterSchema` has happened — it is not a plugin setting in the V5/W3 sense, it's a host-side ops flag that happens to live on the plugin row. The cleanest cuts:
  - **Option a (preserve B for this only):** keep `PluginRecord.Config` as a host-internal ops escape hatch, drop `ctx.Config()` from the SDK, drop the proto's `config = 2` (or freeze it). Document that the column is **not for plugin use**.
  - **Option b (move to host ops table):** add an `ops_plugin_flags` row with `skip_migration` and read it where the host currently reads `pluginConfig`. Then `PluginRecord.Config` and the proto field can be deleted entirely.
- **Type weakness:** the existing storage is `map[string]string` (`repository.go:40`), and the admin API path goes `map[string]any → fmt.Sprintf("%v")` (`manager.go:519-538`), losing type information on round-trip (`true → "true"` is a string, not a bool). Path A's JSONB storage solves this by construction.
- **Defaults today:** B has none — initial Config is `map[string]string{}` at `manager.go:275`. The single `skip_migration` consumer treats absence as `false` (`migration_proxy_server.go:127-132`).
- **Secret keys:** **none today** — the only real config key is `skip_migration` which is not a secret. So "B → A means carrying over a secret" is not currently a problem; but see §4 for why A isn't ready for secrets either.

---

## 4. Schema v1 gaps (industry standard vs us)

| Gap | Industry / VS Code / Backstage / Grafana | sub2api today | Severity (must / should / nice) |
|---|---|---|---|
| **Secret hint** (mark a field as a credential, redact in UI, return write-only/masked GET) | VS Code uses `format: password` + write-only protocol fields; Grafana plugins use `secureJsonData`. Backstage tags via `visibility: secret` | **Missing.** No `format: password`, no `writeOnly`, no `secret` keyword, no `x-sensitive`, no UI redaction. Saved value is returned plaintext via `SchemaInfo.values` (`plugin_settings_service.go:339-344` → `PluginSettingsSchemaInfo.Values map[string]json.RawMessage`) and the form would render it in a plain `<input type="text">` (`PluginSettingsForm.vue:54-60`). Even if a plugin set `format: password` today the UI ignores it and the API returns the raw value. | **Must** if we plan to receive any real secret via Path A. Currently SecretEncryption capability exists (`plugin-sdk/secrets.go:28-38`) but there's no link from a settings field to it. |
| **`requires_restart` hint** | VS Code: `scope: machine`/`window` + restart prompt; JetBrains: `restart-required`; Grafana: hot-reload semantics documented per field | **Missing.** Schema declarations have no per-key restart marker; settings_extension server's Watch contract assumes hot-reload (`settings_extension_server.go:115-149`). Plugin code has no signal that a restart is required for a particular field. | **Must** for the proposed "no Path B" model — without it, you can no longer say "config X needs a restart, set it then restart" because everything goes through the same surface. |
| **`deprecated` marker** | JSON Schema 2019-09 standard `deprecated: true`; VS Code shows strikethrough + warning | **Missing.** No keyword, no UI handling. Old keys silently linger in `plugin_settings` after a schema upgrade because `seedDefaults` is `INSERT … ON CONFLICT DO NOTHING` (`plugin_settings_service.go:194-204`), and `RegisterSchema` never deletes orphaned rows. | **Should** — without it, every plugin schema change leaves dead data in the table forever. |
| **Schema versioning** | OpenAPI servers / Backstage carry an explicit version; allows live migration of stored values | **Missing.** `plugin_settings_schemas` has only `(plugin_name PK, schema_json, defaults_json, updated_at)` (`102_plugin_settings.sql:22-27`) — when a plugin v0.2 changes a key from `string` to `object`, existing rows in `plugin_settings` keep the old shape and `Get()` returns an `string` blob to a typed read. The plugin must do its own version detection. | **Should** — without it, `Settings.GetTyped` failures during plugin upgrade are inevitable. SETTINGS_API.md:96-101 explicitly punts on this ("the plugin is responsible for graceful degradation"). |
| **Constraint surfacing in UI** (min/max/pattern/format) | VS Code, all major form renderers honour Draft-07 constraints in the UI | **Server-side validates them**, **UI ignores them**. `PluginSettingsForm.vue:118-137` extracts only `{type, title, description, enum}` — `minimum`/`maximum`/`pattern`/`format` never reach the input element. Server returns 422; user has no client-side feedback until they click Save. | **Should** for UX. Not a security concern. |
| **`required` on partial save** | Most form renderers stage the whole object and validate on submit | Validates per-key by wrapping `{key: value}` (`plugin_settings_service.go:427-443`). If the schema has top-level `required: ["a", "b"]`, saving only key `a` *passes* (because the wrapper has `a` and only `a`'s schema fragment runs against it — but we still pass the whole compiled schema, so `required` may reject). Tested at `monitor/settings/settings_schema.json:38` (`required: ["enabled"]`) — saving just `defaultIntervalSec` will fail with 422. | **Nice** — surface this as a known limitation before plugin authors discover it the hard way. |
| **Schema-default vs UI-displayed value gap** | Most form UIs pre-fill from the schema `default` keyword | Defaults are only seeded **server-side at plugin start** (`seedDefaults`, line 187-205). The UI shows `props.info.values[key]` which is whatever's in the table — fine in steady state, but a brand-new key added in a plugin update only shows after the plugin restarts and seeds. There's no "show schema default if value is missing" path in the UI. | **Nice** |
| **Cross-plugin / namespaced refs** | OpenAPI `$ref`, JSON Schema `$ref` via `$id` registries | Single-namespace only — both proto and service. `validateAgainst` uses one compiled schema per plugin (`plugin_settings_service.go:427-450`). Per-plugin namespacing is intentional (proto comment at `sdk.proto:430`). | Nice — out of scope |

### Spec-level summary

| Required for "cut B and merge into A" | Have? |
|---|---|
| Path A can express types Path B writes today (booleans, strings, ints, JSON objects) | **Yes** — JSONB is a strict superset of B's stringly-typed map |
| Path A has a default mechanism | **Yes** — `seedDefaults` (line 187) |
| Path A can flag "needs restart" so admins know | **No** — must add (Curator must decide whether the marker is in schema vendor extension `x-requires-restart` or a sibling DB column) |
| Path A can hide secrets | **No** — must add for any real-world secret to flow through A |
| Path A handles plugin schema upgrades | **Partial** — schema row is replaced, but stored values are not migrated and not flagged as orphaned. No version metadata to drive a migration. |

---

## 5. `plugin_settings` table migration friendliness

`backend/migrations/102_plugin_settings.sql` defines a flat key/value/revision table. To support v2 features, the columns we'd need to add (all backward-compatible — `ALTER TABLE … ADD COLUMN … DEFAULT …`):

| Column to add | Type | Why | Backward compatible? |
|---|---|---|---|
| `kind` | `TEXT NOT NULL DEFAULT 'plain'` | Distinguish secret-encrypted blob from JSON | Yes — old rows default to `plain` |
| `cipher_blob` | `BYTEA` | Store SecretEncryption-sealed payload when `kind='secret'` (`value_json` becomes nullable or empty for these rows) | Yes if `value_json` allows NULL or is set to literal `null::jsonb` for secret rows |
| `deprecated_at` | `TIMESTAMPTZ` | Marks orphaned rows after a schema-upgrade prune pass | Yes — nullable |
| `schema_version` | `TEXT` (mirrored from `plugin_settings_schemas.schema_version`) | Supports per-row migration during plugin version bump | Yes — nullable |

For `plugin_settings_schemas`:

| Column to add | Type | Why |
|---|---|---|
| `schema_version` | `TEXT` (e.g. plugin's `Manifest.Version` snapshot) | Versioning |
| `previous_schema_json` | `JSONB` (nullable) | Optional retention for migration logic |

**Friendliness verdict:** the existing schema is **friendly enough** for v2 — every gap is additive. No PK changes needed. Existing `revision` column already supports optimistic concurrency. The only structural awkwardness is that `value_json` is `NOT NULL` (`102_plugin_settings.sql:13`); for `kind='secret'` rows you'd either need to drop that constraint or store something like `'{"_encrypted":true}'` as a placeholder.

---

## 6. Top-3 risks for Curator (if "cut B, merge into A")

### Risk 1 — `skip_migration` is read **before** the plugin is up; Path A cannot represent it (HIGH)

**Where:** `backend/internal/plugin/manager.go:882` reads the key inside `spawnAndConnect`, after `Init` has been sent to the plugin (line 851 `Config: pluginConfig` is the last reference to the data) but **before** `fetchAndRunPluginMigrations` runs. Path A's `RegisterSchema` runs in the same flow and stores the schema — so by the time settings are queryable through W3, migrations either have already run or the gate has already happened.

The implication: if you delete `PluginRecord.Config` outright, you must move `skip_migration` somewhere host-owned (e.g., a tiny `ops_plugin_flags(plugin_name, key, value)` table read directly by the manager), because a plugin-namespaced W3 setting cannot gate the plugin's own migration.

**Why this matters for Curator:** "砍 B 并 A" sounds like 0-cost for plugins (it is), but is **not** a no-op host-side. Either Curator carves out `PluginRecord.Config` as "host-only ops storage with one key" (and freezes the proto field instead of removing it), or commits to building a tiny ops surface to replace the escape hatch. Pretending the field can just go away breaks the operator-recovery story for broken migrations.

### Risk 2 — schema v1 has no `secret` / `requires_restart` / `deprecated` / `version` (HIGH)

**Where:** §4 details every gap, with file:line cites — `PluginSettingsForm.vue:118-137` ignores every keyword beyond `{type, title, description, enum}`; `plugin_settings_schemas` has no `schema_version` column (`102_plugin_settings.sql:22-27`); no link from settings to `SecretEncryption` capability.

**Why this matters for Curator:** Grafana / VS Code / Backstage's "one schema-driven path" model **depends** on `requires_restart`, `secret`, and `deprecated` markers. Without them, "merge B into A" doesn't actually replace B's expressiveness:
- "ops bypass for broken migration" needs `requires_restart` semantics (or it must stay on the host-only side of the line).
- Any future plugin that wants to store an API key (a likely first request after this refactor) has nowhere safe to put it — the value will end up in `plugin_settings.value_json` plaintext, displayed in the form as a normal `<input type="text">` (`PluginSettingsForm.vue:54-60`), and returned in the GET response (`plugin_settings_service.go:336-344`).
- Schema upgrades silently leave orphan rows because `RegisterSchema` never prunes (`plugin_settings_service.go:132-183`).

The "砍 B 并 A" decision implicitly commits to building these markers before the cut happens, or accepting that the merged surface is strictly worse than the sum of its parts for any non-trivial config.

### Risk 3 — UI renderer is hand-rolled, not vue-json-schema-form (MEDIUM)

**Where:** `frontend/src/components/admin/PluginSettingsForm.vue:91-220` is ~130 lines of manual `v-if`/`v-else-if` over a 5-field `PropDescriptor`. The project's own SETTINGS_API.md:96 mentions `vue-json-schema-form` as the intended renderer, but **the actual code does not import it** — grep `vue-json-schema-form` across `frontend/`: 0 matches in `package.json`, `pnpm-lock.yaml`, or any `.vue` file (only mentioned in comments / docs).

**Why this matters for Curator:** every new schema feature you add (secret hint, deprecated badge, requires-restart label, min/max constraint surface, format-aware widgets like dates and URLs) is a hand-coded branch in this single component. The "industry standard schema-driven UX" assumption falls apart at the renderer boundary. Curator should either (a) commit to a library swap before adding markers, or (b) plan the marker spec to keep the hand-rolled renderer's complexity bounded (e.g., one binary `secret: true` toggle that maps to `<input type="password">`, not a generic `format` keyword).

---

## Appendix A — exact entry points table (for downstream agents)

| Concept | File:Line(s) |
|---|---|
| Path B SDK accessor | `plugin-sdk/context.go:35`, `plugin-sdk/runner.go:675-681` |
| Path B proto field | `plugin-sdk/proto/plugin.proto:32` |
| Path B host write path | `backend/internal/handler/admin/plugin_handler.go:140-157` → `backend/internal/plugin/manager.go:509-515` → `repository.go:177-192` |
| Path B host read path (only one consumer) | `backend/internal/plugin/manager.go:882` → `migration_proxy_server.go:127-132` |
| Path B initial seeding (empty map) | `backend/internal/plugin/manager.go:272-276` |
| Path B route registration | `backend/internal/server/routes/admin.go:563` |
| Path B frontend (read-only display) | `frontend/src/views/admin/PluginsView.vue:296-323, 402-410` |
| Path A SDK | `plugin-sdk/settings.go:71-88` (interface), `144-152` (constructor), `184-303` (impl) |
| Path A proto | `plugin-sdk/proto/sdk.proto:417-472`; `plugin-sdk/proto/plugin.proto:96-146` (manifest schema fields) |
| Path A manifest declaration | `plugin-sdk/manifest.go:108, 120-133, 226-242` (auto-promotes capability) |
| Path A DB tables | `backend/migrations/102_plugin_settings.sql:10-27` |
| Path A host service | `backend/internal/service/plugin_settings_service.go:113-450` |
| Path A gRPC server | `backend/internal/plugin/settings_extension_server.go:30-193` |
| Path A admin handler | `backend/internal/handler/admin/plugin_settings_handler.go:51-167` |
| Path A admin frontend (form) | `frontend/src/components/admin/PluginSettingsForm.vue:91-220` (logic), `frontend/src/api/admin/pluginSettings.ts:38-58` (client) |
| Built-in users of Path A | `plugins/hello-world/main.go:67-86, 122-125, 188-191`; `plugins/channel-management/plugin.go:41-51, 143, 162-165, 317` |
| `skip_migration` constant | `backend/internal/plugin/migration_proxy_server.go:121` |
