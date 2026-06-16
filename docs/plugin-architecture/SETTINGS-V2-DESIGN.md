# SETTINGS-V2-DESIGN (Detailed Design — Designer 产出)

> 角色：把 Curator（`SETTINGS-V2-CURATE.md`）的 5 个决策 + 12 个 Step 大纲细化到「下游 Implementer 直接 copy-paste 进代码 / proto / SQL」的精确 spec。
>
> 上游回引：
> - `docs/plugin-architecture/SETTINGS-V2-CURATE.md`（Curator 决策表）
> - `docs/plugin-architecture/SETTINGS-V2-INSPECT.md`（Inspector B — 现状审计）
> - `docs/plugin-architecture/SETTINGS-V2-INDUSTRY.md`（Inspector A — 业界对位）
>
> 本文档不动业务代码。Implementer 必须严格按本文中所有 SQL / proto / Go struct / TS 类型 / errcode / i18n key 的字面值实现，禁止改名、改顺序、改默认值。

---

## 0. 设计概览（Curator 决策回引）

| Curator 决策 | 本文档对位章节 |
|---|---|
| 决策 1：Path B 选 Option a — 保留 `PluginRecord.Config` 作 host-only ops，从 SDK 删 `ctx.Config()`，proto 标 reserved | §3（SDK 改动）+ §附录 A（proto diff）的 `plugin.proto` 部分 |
| 决策 2.1：`x-visibility: frontend\|backend\|secret` + Backstage fail-fast 合并冲突 | §4（Host service）+ §5（UI widget map：`SecretWidget`）+ §7（i18n）+ §附录 F（errcode：`PLUGIN_SETTINGS_SCHEMA_INVALID_VISIBILITY`） |
| 决策 2.2：`x-deprecated: "<msg>"` | §4（`extractDeprecatedFields`）+ §5（`DeprecatedDecorator`）+ §7（i18n）|
| 决策 2.3：`x-requires-reload: true` | §4（PluginManager 订阅 reload 事件）+ §5（`RequiresReloadBadge`）+ §7（i18n）|
| 决策 2.4：`schema_version` 列 + manifest 字段 + version mismatch error | §1（DB migration）+ §2（proto diff）+ §3（SDK manifest + `ErrSchemaVersionMismatch`）|
| 决策 2.5：Grafana 风格 secret 写入语义 | §4（`applySecretWriteSemantics` 在 admin handler）+ §6（默认值读时填充）|
| 决策 3：UI 渲染器保留手写 + widget map 抽象化 | §5（UI widget map）|
| 决策 4：4 阶段迁移 | §10（上线 + 回滚剧本）|
| 决策 5：本次 PR 边界（必做 / 下次做 / 永不做）| 全文都是「必做」；下次做和永不做不展开 |

---

## 1. 设计 1：DB Migration（对应 Curator Step 1）

### 1.1 文件名 + 命名约定回引

观测：`backend/migrations/` 现有 102 个 migration，最大序号 `102_plugin_settings.sql`，命名模式严格 `NNN_<snake_case_description>.sql`（少数 ` 006_*` 同序号双文件出现是历史遗留，新增不允许冲突）。下一个空闲序号是 **`103`**。

**决定**：文件名 = `backend/migrations/103_plugin_settings_v2.sql`。**禁止用** `103_settings_v2.sql` / `103_plugin_settings_marker_columns.sql` 之类自创变体。

观测：`backend/migrations/migrations.go`（INSPECT §5.8 提示）按 filename 字典序读入并应用，`README.md` 已纪录此约定。新增文件不需要任何额外注册。

### 1.2 完整 up SQL（直接 copy-paste 到 `103_plugin_settings_v2.sql`）

**重要约束**：
- 所有 `ALTER TABLE … ADD COLUMN` 都带 `NOT NULL DEFAULT` 以兼容现有行（INSPECT §5「friendliness verdict」已论证）；`NULL` 列只在 `properties_meta` 这种「整体可为空」语义上使用。
- `schema_version` 列采用 `TEXT NOT NULL DEFAULT '0'`，约定：sentinel `'0'` = 「该行写入时尚未启用 V2 SettingsExtension」（详见 §1.5 老数据迁移规则）。
- `properties_meta` 用 `JSONB`，schema 见 §1.4。
- `schema_version_at_write` 加在 `plugin_settings`（值表），用于 V2 写入时记录是哪个 schema 版本写下的——升级 schema 时检测 stale 值的依据。

```sql
-- 103_plugin_settings_v2.sql
-- V5/W6 SETTINGS-V2: extend plugin settings schema metadata to support
-- visibility / deprecated / requires_reload markers and per-row schema
-- version stamping. See docs/plugin-architecture/SETTINGS-V2-DESIGN.md §1.

BEGIN;

-- 1. plugin_settings_schemas: per-plugin schema row.
-- schema_version mirrors Manifest.SettingsSchema.Version reported by the
-- plugin. Sentinel '0' means "plugin did not declare a version" — treated
-- as the lowest possible version when comparing against stored values.
ALTER TABLE plugin_settings_schemas
    ADD COLUMN IF NOT EXISTS schema_version TEXT NOT NULL DEFAULT '0';

-- properties_meta caches the marker extraction (visibility / deprecated /
-- requires_reload) keyed by top-level property name. Persisted so admin
-- API responses do not need to re-walk schema_json on every GET.
-- Layout is documented in SETTINGS-V2-DESIGN §1.4. NULL means
-- "schema has no declared markers"; the host writes '{}'::jsonb instead
-- so handlers never need to nil-check.
ALTER TABLE plugin_settings_schemas
    ADD COLUMN IF NOT EXISTS properties_meta JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN plugin_settings_schemas.schema_version IS
    'Plugin-declared schema version (Manifest.SettingsSchema.Version). ''0'' means undeclared.';
COMMENT ON COLUMN plugin_settings_schemas.properties_meta IS
    'Cached marker extraction: {<prop>: {visibility,deprecated,requires_reload,...}}. See SETTINGS-V2-DESIGN §1.4.';

-- 2. plugin_settings: per-(plugin,key) value row.
-- schema_version_at_write records the schema version that was active when
-- the row was last written. Used by the host to detect stale values when
-- a plugin upgrade ships a new schema_version.
ALTER TABLE plugin_settings
    ADD COLUMN IF NOT EXISTS schema_version_at_write TEXT NOT NULL DEFAULT '0';

COMMENT ON COLUMN plugin_settings.schema_version_at_write IS
    'Schema version active when this row was last written. Compared against plugin_settings_schemas.schema_version to detect stale values.';

-- 3. Index for "find all values written under an old schema version".
-- Used by future cleanup jobs; not on the hot read path.
CREATE INDEX IF NOT EXISTS idx_plugin_settings_schema_version_at_write
    ON plugin_settings (plugin_name, schema_version_at_write);

COMMIT;
```

### 1.3 完整 down SQL（保存为 git 注释，不入库）

观测：`backend/migrations/migrations.go` 当前**只支持单向 up**，没有 `Down()` 钩子。Implementer **不要**新增 down SQL 文件——回滚通过手工 `psql` 执行下面这段。把它作为 commit message 的尾部参考，或加到 PR 描述里。

```sql
-- ROLLBACK for 103_plugin_settings_v2.sql
-- Apply manually via psql when downgrading.
BEGIN;

DROP INDEX IF EXISTS idx_plugin_settings_schema_version_at_write;

ALTER TABLE plugin_settings
    DROP COLUMN IF EXISTS schema_version_at_write;

ALTER TABLE plugin_settings_schemas
    DROP COLUMN IF EXISTS properties_meta;

ALTER TABLE plugin_settings_schemas
    DROP COLUMN IF EXISTS schema_version;

COMMIT;
```

### 1.4 `properties_meta` JSONB shape（pin 死）

`properties_meta` 是一个对象，**键名 = schema 顶层 properties 中的 key**（不是 JSON Pointer，不是 dotted path——只有顶层属性需要 marker，嵌套 properties 一律忽略）。每个 value 是一个固定 shape：

```json
{
  "apiKey": {
    "visibility": "secret",
    "deprecated": null,
    "requires_reload": false
  },
  "defaultIntervalSec": {
    "visibility": "frontend",
    "deprecated": null,
    "requires_reload": true
  },
  "legacyTimeoutSec": {
    "visibility": "frontend",
    "deprecated": "use timeoutMs (milliseconds) instead",
    "requires_reload": false
  }
}
```

**字段约束（写入时校验）**：
- `visibility`：只能是 `"frontend"`、`"backend"`、`"secret"` 三个字符串之一。**默认值 `"frontend"`**（schema 节点没声明 `x-visibility` → host 写入 `"frontend"`）。
- `deprecated`：`null` 或非空字符串。**不允许空字符串**——空字符串和 null 在前端 truthy 判断不一致，强制规整。
- `requires_reload`：bool，默认 `false`。

**Implementer 必读**：上面 3 个字段是固定的，**禁止**添加其他字段（如 `format`、`order`、`scope`）。Curator 决策 5 已经把这些 punt 到下次 PR。

### 1.5 老数据迁移（schema_version='0' 行什么时候迁到真实 version？）

**问题**：现有 `plugin_settings` 行已经写过的 `schema_version_at_write` 默认是 `'0'`。它什么时候变成真实版本号？

**决定**：**永远不迁**。规则：
1. migration 103 跑完后，所有现有行的 `schema_version_at_write = '0'`。
2. 当某个 admin 操作触发 `SetByKey` 写入这一行的下一个 revision 时，host 会用当前 `plugin_settings_schemas.schema_version` 覆盖写入。
3. **不写 backfill 脚本**。理由：plugin v0.x → v1.0 升级时，plugin 自己也得通过 Manifest.SettingsSchema.Version 上报新版本，host 在 RegisterSchema 时更新 `plugin_settings_schemas.schema_version`，之后任何 admin 改动会自动滚动到新版本。stale 行（`schema_version_at_write` < `plugin_settings_schemas.schema_version`）由 plugin 自己处理（决策 2.4 落点 5：plugin SDK `GetTyped` 返回 `SchemaVersionMismatchError` 时让 plugin 自己 fallback）。

**理由 pin 死**：写 backfill 脚本要枚举每个 plugin 的"当前 schema_version 是什么"，跑全表 UPDATE，runtime 大、容错差。Curator 决策 5 已经把"schema_version migration hook"明确放到下次做。

---

## 2. 设计 2：proto 完整 diff（对应 Curator Step 2）

### 2.1 `plugin-sdk/proto/plugin.proto` 的改动

**约束**：
- `config = 2` 字段必须改成 `reserved 2;` + `reserved "config";`，**不能物理删字段**（INSPECT §1.1 + Curator 决策 1.3 已论证）。
- `ManifestResponse.settings_schema_json = 42`、`settings_defaults_json = 43` **保留不动**。
- 在 `ManifestResponse` 新增 3 个字段：`settings_schema_version` / `settings_properties_meta_json` 用 field number `44` / `45`。
- 当前 `ManifestResponse` 最大 field number = `43`（INSPECT §2.7 提示，本文 Read 校对：`migration_files=30`/`migrations=41`/`capabilities=40`/`settings_schema_json=42`/`settings_defaults_json=43`）。下一空闲号 = `44`。

**完整 diff（unified format）**：

```diff
--- a/plugin-sdk/proto/plugin.proto
+++ b/plugin-sdk/proto/plugin.proto
@@ -28,8 +28,12 @@ service PluginLifecycle {
 // ============================================================
 
 message PluginInitRequest {
   string sdk_address = 1;
-  map<string, string> config = 2;
+  // reserved: was `map<string, string> config = 2;` — host-only ops fields
+  // were dropped from the SDK in V5/W6 SETTINGS-V2 (see docs/plugin-
+  // architecture/SETTINGS-V2-DESIGN.md §3). The host still uses
+  // PluginRecord.Config for ops escape hatches but it is no longer wired
+  // through the plugin process. Do NOT reuse field number 2 or name "config".
+  reserved 2;
+  reserved "config";
   // plugin_name is the unique plugin identifier the core uses to scope SDK
   // resources (e.g. Redis key namespace). The SDK echoes it back to the core
   // via gRPC metadata on every SDK call so the server can apply per-plugin
   // policy without having to peer at TCP source addresses.
   string plugin_name = 3;
@@ -141,6 +145,30 @@ message ManifestResponse {
   // host applies defaults when a key has not been written yet so plugins
   // can rely on Settings.Get returning a value immediately after install.
   bytes settings_defaults_json = 43;
+
+  // settings_schema_version mirrors Manifest.SettingsSchema.Version (see
+  // SETTINGS-V2-DESIGN §3.3). Plugins should bump this whenever a property's
+  // type changes shape; the host stamps the value into
+  // plugin_settings.schema_version_at_write on every write so plugin SDKs
+  // can detect stale values via SchemaVersionMismatchError. Empty string is
+  // normalised to "0" host-side.
+  string settings_schema_version = 44;
+
+  // settings_properties_meta_json is a JSON object keyed by top-level
+  // schema property name. Each value is the marker triple
+  // {visibility, deprecated, requires_reload} — see SETTINGS-V2-DESIGN
+  // §1.4 for the precise shape. The plugin SDK derives this from the
+  // SettingsSchemaDoc.PropertyMeta map; plugins may also set the markers
+  // inline as JSON Schema vendor extensions (`x-visibility` etc.) and
+  // leave this field empty — the host will re-derive it from
+  // settings_schema_json. When both sources disagree, this field wins
+  // (it is the SDK's authoritative serialization).
+  bytes settings_properties_meta_json = 45;
 }
```

### 2.2 `plugin-sdk/proto/sdk.proto` 的改动

**约束**：
- `SettingsExtension` service 不增减 RPC（保持 `Get` + `Watch` 两个 RPC）。
- `SettingsGetResponse` 新增 2 个字段：`stored_schema_version` (string) + `current_schema_version` (string)，field number `4` + `5`（当前最大 = `3`）。
- `SettingsChangeEvent` 新增 1 个字段：`requires_reload` (bool)，field number `4`（当前最大 = `3`）。
- **不**新增 `WatchEventKind` enum——Curator 决策没有要求。schema 升级时 host 直接关流让 SDK 重连即可（详见 §4.4）。

```diff
--- a/plugin-sdk/proto/sdk.proto
+++ b/plugin-sdk/proto/sdk.proto
@@ -451,12 +451,28 @@ message SettingsGetRequest {
 message SettingsGetResponse {
   // value_json holds the JSON-encoded current value when exists=true.
   bytes value_json = 1;
   // exists is false when the key is not yet persisted (no admin save and
   // no default) — callers should fall back to the schema default in that
   // case rather than treating it as an error.
   bool exists = 2;
   // revision is monotonically increasing per (plugin, key); callers can
   // use it to deduplicate rapid updates.
   int64 revision = 3;
+
+  // stored_schema_version is plugin_settings.schema_version_at_write — the
+  // schema version that was active when the row was last written. Empty
+  // when exists=false. See SETTINGS-V2-DESIGN §1.5 for semantics.
+  string stored_schema_version = 4;
+
+  // current_schema_version is plugin_settings_schemas.schema_version — the
+  // schema version the plugin most recently registered. The plugin SDK
+  // compares stored vs current to raise SchemaVersionMismatchError when
+  // they disagree. Empty string is normalised to "0" by both sides.
+  string current_schema_version = 5;
 }
 
 message SettingsWatchRequest {
   // key="" subscribes to the entire plugin namespace.
   string key = 1;
 }
 
 message SettingsChangeEvent {
   string key = 1;
   bytes value_json = 2;
   int64 revision = 3;
+
+  // requires_reload mirrors the plugin's `x-requires-reload` schema marker
+  // for this key. The host PluginManager subscribes and triggers a plugin
+  // process reload when it receives a true. Plugins themselves may ignore
+  // this field — it is only meaningful to the host. See SETTINGS-V2-DESIGN
+  // §4.4 for the reload state machine.
+  bool requires_reload = 4;
 }
```

### 2.3 重新生成 .pb.go 的命令（pin 死）

观测 `plugin-sdk/proto/` 周边没有 Makefile target，但 INSPECT §6 提示 SDK 有自己的生成流程。实施步骤：

1. 修改 `.proto` 文件后立即运行：
   ```bash
   cd plugin-sdk/proto
   protoc --go_out=. --go_opt=paths=source_relative \
          --go-grpc_out=. --go-grpc_opt=paths=source_relative \
          plugin.proto sdk.proto
   ```
2. `git diff plugin-sdk/proto/pluginsdk/*.pb.go`：必须只看到「新字段对应的 getter/setter 增加」+「reserved 字段被注释/移除」，不允许有"无关"改动。
3. 跑 `cd backend && go build ./...` + `cd plugin-sdk && go build ./...` 验证。

---

## 3. 设计 3：Go SDK 改动（对应 Curator Step 3-5）

### 3.1 `plugin-sdk/context.go` — 删除 1 行

**改前（line 33-35）**：

```go
	// Config returns the plain-string configuration map the core supplied in
	// the Init request. The map is a copy; mutating it has no effect.
	Config() map[string]string
```

**改后**：完全删除上述 3 行（注释 + 方法签名）。

### 3.2 `plugin-sdk/runner.go` — 删除 / 保留逐行 spec

观测当前文件：
- line 392-395：`cfgCopy := make(map[string]string, len(req.GetConfig()))` + 复制循环 — **删除**（删除 `req.GetConfig()` 也行，proto 已 reserved）
- line 461：`config: cfgCopy,` — **删除**
- line 665：`config map[string]string,` (struct field) — **删除**
- line 675-681：整个 `(c *pluginCtx) Config() map[string]string { ... }` 方法 — **删除**

**保留**：line 393 处的 `req.GetConfig()` 调用所在 codegen 残留（如果 `.pb.go` 还生成 `GetConfig()` getter——proto 标 `reserved` 后不会再生成 getter，所以 SDK 这一侧的代码也要全删）。

**Implementer 验收命令（Curator §4.1 已列）**：

```bash
grep -rn "ctx\.Config()" --include="*.go" plugins/ plugin-sdk/  # 期望 0
grep -rn "pctx\.Config()" --include="*.go" plugins/             # 期望 0
grep -rn "\.config\b" plugin-sdk/runner.go | grep -v "//"       # 期望 0
```

### 3.3 `plugin-sdk/manifest.go` — 新增字段

#### 3.3.1 `SettingsSchemaDoc` 扩展

**改前（line 120-133）**：

```go
type SettingsSchemaDoc struct {
	Schema   json.RawMessage
	Defaults json.RawMessage
}
```

**改后（完整新结构）**：

```go
// SettingsSchemaDoc bundles a JSON Schema and its default values into the
// shape the SDK ships in the manifest. Both fields are stored as raw JSON so
// plugins can compose them however they like (literal strings, embed.FS
// bytes, generated structs marshalled at startup).
//
// V5/W6 SETTINGS-V2 added Version and PropertyMeta to support marker-driven
// admin UI features (visibility / deprecated / requires_reload). See
// docs/plugin-architecture/SETTINGS-V2-DESIGN.md §3.3 for the full spec.
type SettingsSchemaDoc struct {
	// Schema is a JSON Schema Draft-07 document (see existing comment).
	Schema json.RawMessage

	// Defaults mirrors the schema top-level properties (see existing comment).
	Defaults json.RawMessage

	// Version is the plugin's self-declared schema version, e.g. "1.0.0".
	// The host stamps it into plugin_settings.schema_version_at_write on
	// every write so the plugin SDK's GetTyped can detect stale values
	// (SchemaVersionMismatchError). Empty is normalised to "0" host-side.
	//
	// Semantic versioning is RECOMMENDED but not enforced — the host
	// treats it as an opaque string and only checks for equality. The
	// plugin owns the comparison logic when it wants ordering.
	Version string `json:"version,omitempty"`

	// PropertyMeta carries per-property markers (visibility / deprecated /
	// requires_reload) keyed by the top-level property name. The SDK
	// serialises this map into ManifestResponse.settings_properties_meta_json.
	//
	// Plugins may declare the markers inline as JSON Schema vendor extensions
	// (`x-visibility`, `x-deprecated`, `x-requires-reload`) on the schema
	// node itself instead of populating this map. Both work; when both are
	// present this map wins (per SETTINGS-V2-DESIGN §3.3.2).
	PropertyMeta map[string]PropertyMetadata `json:"property_meta,omitempty"`
}

// PropertyMetadata is the marker triple SETTINGS-V2 attaches to one
// top-level schema property. All three fields default to the zero value;
// `Visibility == ""` is normalised to "frontend" by the host.
type PropertyMetadata struct {
	// Visibility is one of "frontend" | "backend" | "secret". Empty defaults
	// to "frontend" host-side. See SETTINGS-V2-DESIGN §4.2 for read/write
	// semantics.
	Visibility string `json:"visibility,omitempty"`

	// Deprecated, if non-empty, marks the field as deprecated. The string
	// is the human-readable migration message ("use foo instead"). Admin
	// UI renders strikethrough + warning tag. Empty means not deprecated.
	Deprecated string `json:"deprecated,omitempty"`

	// RequiresReload=true means the host should reload the plugin process
	// after admin saves this key. See SETTINGS-V2-DESIGN §4.4 for the
	// reload state machine.
	RequiresReload bool `json:"requires_reload,omitempty"`
}
```

**字段验证规则（SDK 在 `toProto()` 时执行，违反返回 error 让 plugin 启动失败）**：

```go
// In manifest.go — new helper called from toProto()
func validatePropertyMeta(meta map[string]PropertyMetadata) error {
	for prop, m := range meta {
		switch m.Visibility {
		case "", "frontend", "backend", "secret":
			// ok
		default:
			return fmt.Errorf("pluginsdk: SettingsSchemaDoc.PropertyMeta[%q].Visibility=%q must be one of frontend|backend|secret", prop, m.Visibility)
		}
	}
	return nil
}
```

#### 3.3.2 `Manifest.toProto()` 扩展

**改前（line 226-242）**：保持现有 `if m.SettingsSchema != nil { … }` 块。

**改后（在该 block 末尾追加）**：

```go
		// V5/W6 SETTINGS-V2: ship version + per-property markers so the
		// host does not need to re-walk the schema for every admin GET.
		resp.SettingsSchemaVersion = m.SettingsSchema.Version

		if len(m.SettingsSchema.PropertyMeta) > 0 {
			metaBytes, err := json.Marshal(m.SettingsSchema.PropertyMeta)
			if err != nil {
				// Falling back to nil keeps the host able to derive markers
				// from schema_json alone; the plugin author saw the error
				// in their logs via slog.Default since toProto runs at
				// manifest send time.
				resp.SettingsPropertiesMetaJson = nil
			} else {
				resp.SettingsPropertiesMetaJson = metaBytes
			}
		}
```

> 字段名 `SettingsSchemaVersion` / `SettingsPropertiesMetaJson` 与 §2.1 proto 字段 `settings_schema_version` / `settings_properties_meta_json` 由 protoc 自动生成的 PascalCase 一致。Implementer 不要手写改名。

### 3.4 `plugin-sdk/settings.go` — schema mismatch 处理

#### 3.4.1 新增 error 类型

**位置**：紧跟现有 `var ErrSettingNotFound = errors.New(...)` 声明之后（line 38 之后）。

```go
// ErrSchemaVersionMismatch is the sentinel returned by SettingsClient.GetTyped
// when the stored value was written under a schema_version that differs from
// the schema_version the plugin currently declares. Callers should treat it
// as "the plugin upgraded its schema and the stored value may not unmarshal
// into the current Go type" — typically the right reaction is to fall back
// to the schema default and trigger a one-time migration.
//
// Use errors.Is to detect it; the concrete type SchemaVersionMismatchError
// carries the two version strings if the caller wants to log them.
var ErrSchemaVersionMismatch = errors.New("pluginsdk: settings schema version mismatch")

// SchemaVersionMismatchError is returned wrapped in ErrSchemaVersionMismatch
// when a Get / GetTyped finds a value written under an older schema version.
// Inspect its fields to log the precise drift; use errors.Is to branch.
type SchemaVersionMismatchError struct {
	Key                  string
	StoredSchemaVersion  string
	CurrentSchemaVersion string
	UnderlyingErr        error
}

func (e *SchemaVersionMismatchError) Error() string {
	return fmt.Sprintf(
		"pluginsdk: settings %q stored under schema_version=%q, current schema_version=%q (underlying: %v)",
		e.Key, e.StoredSchemaVersion, e.CurrentSchemaVersion, e.UnderlyingErr,
	)
}

func (e *SchemaVersionMismatchError) Unwrap() error { return e.UnderlyingErr }

// Is implements errors.Is so callers can `if errors.Is(err, ErrSchemaVersionMismatch)`.
func (e *SchemaVersionMismatchError) Is(target error) bool {
	return target == ErrSchemaVersionMismatch
}
```

#### 3.4.2 `cachedSetting` 扩展（保存 stored vs current schema_version）

**位置**：line 110-115 处的 `cachedSetting` struct。

```go
type cachedSetting struct {
	value                json.RawMessage
	revision             int64
	exists               bool
	fetchedAt            time.Time
	storedSchemaVersion  string // V5/W6 SETTINGS-V2: schema_version active when written
	currentSchemaVersion string // V5/W6 SETTINGS-V2: schema_version the plugin currently declares
}
```

#### 3.4.3 `Get` 缓存填充修改

**位置**：line 207-214 处。

**改前**：
```go
	val := append(json.RawMessage(nil), resp.GetValueJson()...)
	c.cache.Store(key, &cachedSetting{
		value:     val,
		revision:  resp.GetRevision(),
		exists:    true,
		fetchedAt: time.Now(),
	})
	return val, nil
```

**改后**：
```go
	val := append(json.RawMessage(nil), resp.GetValueJson()...)
	c.cache.Store(key, &cachedSetting{
		value:                val,
		revision:             resp.GetRevision(),
		exists:               true,
		fetchedAt:            time.Now(),
		storedSchemaVersion:  resp.GetStoredSchemaVersion(),
		currentSchemaVersion: resp.GetCurrentSchemaVersion(),
	})
	return val, nil
```

#### 3.4.4 `GetTyped` 在 unmarshal 失败时检查 schema mismatch

**位置**：line 217-226 处的 `GetTyped`。

**改前**：
```go
func (c *settingsClient) GetTyped(ctx context.Context, key string, out any) error {
	raw, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("pluginsdk: settings unmarshal %q: %w", key, err)
	}
	return nil
}
```

**改后**（**注意**：现有 `Get` 已经把 schema_version 缓存到 `cachedSetting`，这里只需读取缓存即可——不需要再发 RPC）：

```go
func (c *settingsClient) GetTyped(ctx context.Context, key string, out any) error {
	raw, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		// Look up cached schema_version to attach precise drift info.
		var stored, current string
		if v, ok := c.cache.Load(key); ok {
			entry := v.(*cachedSetting)
			stored = entry.storedSchemaVersion
			current = entry.currentSchemaVersion
		}
		// Normalise empty strings to "0" so callers can compare without
		// special-casing pre-V2 hosts.
		if stored == "" {
			stored = "0"
		}
		if current == "" {
			current = "0"
		}
		underlying := fmt.Errorf("pluginsdk: settings unmarshal %q: %w", key, err)
		if stored != current {
			return &SchemaVersionMismatchError{
				Key:                  key,
				StoredSchemaVersion:  stored,
				CurrentSchemaVersion: current,
				UnderlyingErr:        underlying,
			}
		}
		return underlying
	}
	return nil
}
```

**行为决策（pin 死）**：
- schema mismatch 时**不透明重试** — plugin 拿到错误自己决定 fallback。理由：透明重试只会重新拿到同一个 stored 值（老 schema 写的），unmarshal 还是会失败。让 plugin 自己处理。
- schema 一致但 unmarshal 失败 → 返回普通包装 error（plugin 可能写错了 schema 默认值或类型）。

### 3.5 与 `applyEvent` 的兼容（缓存 schema_version 留空）

**位置**：line 305-341 的 `applyEvent`。Watch 推送过来的 `SettingsChangeEvent` **没有** schema_version 字段（§2.2 only 加了 `requires_reload`）。`applyEvent` 写缓存时 schema_version 字段保持上一次 Get 的值即可——`SettingsChange` 事件只代表"value 变了"，不代表 schema 变了。如果 plugin 想刷新 schema_version，下次 `GetTyped` 触发的 Get RPC 会重填。

**改前**：line 309-315。

**改后**（保留旧 schema_version）：

```go
func (c *settingsClient) applyEvent(evt *pb.SettingsChangeEvent) {
	if evt == nil {
		return
	}
	val := append(json.RawMessage(nil), evt.GetValueJson()...)
	// Preserve existing stored/current schema_version cache: change events
	// do not carry schema metadata. A subsequent Get will refresh them.
	prevStored := ""
	prevCurrent := ""
	if v, ok := c.cache.Load(evt.GetKey()); ok {
		entry := v.(*cachedSetting)
		prevStored = entry.storedSchemaVersion
		prevCurrent = entry.currentSchemaVersion
	}
	c.cache.Store(evt.GetKey(), &cachedSetting{
		value:                val,
		revision:             evt.GetRevision(),
		exists:               true,
		fetchedAt:            time.Now(),
		storedSchemaVersion:  prevStored,
		currentSchemaVersion: prevCurrent,
	})
	change := SettingsChange{Key: evt.GetKey(), Value: val, Revision: evt.GetRevision()}
	// ... 后续 fan-out 逻辑保持不变
```

---

## 4. 设计 4：Host service 改动（对应 Curator Step 6-9）

### 4.1 `RegisterSchema` 签名 + 行为

**约束**：
- **不**新增 `schemaVersion string` 参数到方法签名。理由：plugin 通过 manifest 传递所有元数据（schema_json + defaults_json + schema_version + properties_meta_json），把 4 个参数堆到 RegisterSchema 不优雅。改成接受一个聚合 struct。
- 使用聚合 struct `RegisterSchemaInput`（新增）。

**新签名**：

```go
// RegisterSchemaInput is the V5/W6 SETTINGS-V2 envelope for plugin schema
// registration. Plugins build it from their Manifest.SettingsSchema and the
// host PluginManager builds it from ManifestResponse fields.
type RegisterSchemaInput struct {
	PluginName       string
	SchemaJSON       []byte // Manifest.SettingsSchema.Schema bytes
	DefaultsJSON     []byte // Manifest.SettingsSchema.Defaults bytes
	SchemaVersion    string // Manifest.SettingsSchema.Version (empty → "0")
	PropertiesMeta   []byte // serialised PropertyMeta map; nil → derive from schema
}

func (s *PluginSettingsService) RegisterSchema(
	ctx context.Context, in RegisterSchemaInput,
) error
```

**调用方迁移**（`backend/internal/plugin/manager.go:833-845`）：

```go
// 改前
err := m.settingsService.RegisterSchema(regCtx, inst.Name,
    manifest.GetSettingsSchemaJson(), manifest.GetSettingsDefaultsJson())

// 改后
err := m.settingsService.RegisterSchema(regCtx, service.RegisterSchemaInput{
    PluginName:     inst.Name,
    SchemaJSON:     manifest.GetSettingsSchemaJson(),
    DefaultsJSON:   manifest.GetSettingsDefaultsJson(),
    SchemaVersion:  manifest.GetSettingsSchemaVersion(),
    PropertiesMeta: manifest.GetSettingsPropertiesMetaJson(),
})
```

**RegisterSchema 实现伪代码**：

```go
func (s *PluginSettingsService) RegisterSchema(ctx context.Context, in RegisterSchemaInput) error {
	if in.PluginName == "" {
		return errors.New("plugin_settings: empty plugin name")
	}
	if len(in.SchemaJSON) == 0 {
		// Existing "delete schema row" behaviour preserved.
		return s.deleteSchema(ctx, in.PluginName)
	}

	// 1. Compile schema (existing).
	compiled, err := compileSchema(in.SchemaJSON)
	if err != nil {
		return fmt.Errorf("plugin_settings: compile schema for %s: %w", in.PluginName, err)
	}

	// 2. Resolve properties_meta:
	//    a) if in.PropertiesMeta is non-empty, parse it (SDK authoritative)
	//    b) else extract from schema vendor extensions (x-visibility, etc.)
	meta, err := s.resolvePropertiesMeta(in.SchemaJSON, in.PropertiesMeta)
	if err != nil {
		return err
	}

	// 3. Validate visibility values (Backstage fail-fast — Curator 决策 2.1 落点 6).
	if err := validateVisibilities(meta); err != nil {
		return err
	}

	// 4. Normalise schema_version: empty → "0".
	schemaVersion := in.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = "0"
	}

	// 5. Marshal meta back to bytes for storage.
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("plugin_settings: marshal properties_meta: %w", err)
	}

	// 6. Persist.
	rawSchema := append(json.RawMessage(nil), in.SchemaJSON...)
	rawDefaults := append(json.RawMessage(nil), in.DefaultsJSON...)
	if len(rawDefaults) == 0 {
		rawDefaults = json.RawMessage("{}")
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO plugin_settings_schemas
		    (plugin_name, schema_json, defaults_json, schema_version, properties_meta, updated_at)
		VALUES ($1, $2::jsonb, $3::jsonb, $4, $5::jsonb, NOW())
		ON CONFLICT (plugin_name) DO UPDATE
		   SET schema_json     = EXCLUDED.schema_json,
		       defaults_json   = EXCLUDED.defaults_json,
		       schema_version  = EXCLUDED.schema_version,
		       properties_meta = EXCLUDED.properties_meta,
		       updated_at      = NOW()
	`, in.PluginName, string(rawSchema), string(rawDefaults), schemaVersion, string(metaJSON)); err != nil {
		return fmt.Errorf("plugin_settings: persist schema: %w", err)
	}

	// 7. Update in-memory cache.
	s.mu.Lock()
	s.compiledSchemas[in.PluginName] = compiled
	s.rawSchemas[in.PluginName] = rawSchema
	s.defaults[in.PluginName] = rawDefaults
	s.schemaVersions[in.PluginName] = schemaVersion
	s.propertiesMeta[in.PluginName] = meta
	s.mu.Unlock()

	// 8. Seed defaults — but DO NOT seed when read-time fill is enabled
	//    (see §6). For SETTINGS-V2 we KEEP startup seeding for backward
	//    compatibility but layer read-time fill on top in GetByKey.
	if err := s.seedDefaults(ctx, in.PluginName, schemaVersion, rawDefaults); err != nil {
		s.logger.Warn("plugin_settings: seed defaults failed",
			"plugin", in.PluginName, "error", err)
	}
	return nil
}
```

> Implementer **必须**保留 `deleteSchema` 路径行为不变（INSPECT §2.3 RegisterSchema 现状的 "len(schemaJSON) == 0 → 清除" 分支）。

### 4.2 visibility / deprecated / requires_reload 提取与校验

**新方法签名**：

```go
// extractMetaFromSchema 解析 JSON Schema 中所有 x-visibility / x-deprecated /
// x-requires-reload vendor extension。返回 PropertiesMeta map（topLevelProp → metadata）。
// 实现：json.Unmarshal 到 map[string]any 后遍历 properties；不依赖 jsonschema 库。
func (s *PluginSettingsService) extractMetaFromSchema(schemaJSON []byte) (map[string]service.PropertyMetadata, error)

// resolvePropertiesMeta 优先用 sdk-authoritative meta（in.PropertiesMeta），
// 缺失时 fallback 到 extractMetaFromSchema。
func (s *PluginSettingsService) resolvePropertiesMeta(schemaJSON, sdkMeta []byte) (map[string]service.PropertyMetadata, error)

// validateVisibilities 校验所有 visibility 值合法（"frontend"|"backend"|"secret"|""），
// 不合法返回 ErrInvalidSchemaVisibility 让 RegisterSchema 失败。
func validateVisibilities(meta map[string]service.PropertyMetadata) error
```

**新增类型**：

```go
// PropertyMetadata mirrors plugin-sdk PropertyMetadata. Re-defined here to
// avoid cross-package import cycles between service and plugin-sdk.
type PropertyMetadata struct {
	Visibility     string `json:"visibility"`
	Deprecated     string `json:"deprecated"`
	RequiresReload bool   `json:"requires_reload"`
}

// ErrInvalidSchemaVisibility is returned by RegisterSchema when a property
// declares an x-visibility outside the allowed set.
var ErrInvalidSchemaVisibility = errors.New("plugin_settings: x-visibility must be one of frontend|backend|secret")
```

**`extractMetaFromSchema` 实现伪代码**：

```go
func (s *PluginSettingsService) extractMetaFromSchema(schemaJSON []byte) (map[string]PropertyMetadata, error) {
	var doc struct {
		Properties map[string]map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		return nil, fmt.Errorf("extract meta: unmarshal schema: %w", err)
	}
	out := make(map[string]PropertyMetadata, len(doc.Properties))
	for prop, node := range doc.Properties {
		var m PropertyMetadata
		if raw, ok := node["x-visibility"]; ok {
			_ = json.Unmarshal(raw, &m.Visibility)
		}
		if raw, ok := node["x-deprecated"]; ok {
			_ = json.Unmarshal(raw, &m.Deprecated)
		}
		if raw, ok := node["x-requires-reload"]; ok {
			_ = json.Unmarshal(raw, &m.RequiresReload)
		}
		// Normalise empty visibility to "frontend".
		if m.Visibility == "" {
			m.Visibility = "frontend"
		}
		out[prop] = m
	}
	return out, nil
}
```

### 4.3 `SetByKey` — visibility 校验 + schema_version 写入 + requires_reload 推送

**改前**：line 261-298（INSPECT §2.3）。

**改后伪代码**：

```go
func (s *PluginSettingsService) SetByKey(
	ctx context.Context, pluginName, key string, value json.RawMessage, source SetSource,
) (int64, error) {
	if pluginName == "" || key == "" {
		return 0, errors.New("plugin_settings: empty plugin or key")
	}

	s.mu.RLock()
	compiled, ok := s.compiledSchemas[pluginName]
	meta := s.propertiesMeta[pluginName]
	schemaVersion := s.schemaVersions[pluginName]
	s.mu.RUnlock()
	if !ok {
		return 0, ErrPluginSettingsSchemaMissing
	}

	// V5/W6 SETTINGS-V2: backend-only keys reject admin writes.
	if source == SetSourceAdmin {
		if propMeta, ok := meta[key]; ok && propMeta.Visibility == "backend" {
			return 0, &ErrPluginSettingsBackendOnly{Plugin: pluginName, Key: key}
		}
	}

	if err := validateAgainst(compiled, key, value); err != nil {
		return 0, err
	}
	if schemaVersion == "" {
		schemaVersion = "0"
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO plugin_settings (plugin_name, key, value_json, revision, schema_version_at_write, updated_at)
		VALUES ($1, $2, $3::jsonb, 1, $4, NOW())
		ON CONFLICT (plugin_name, key) DO UPDATE
		   SET value_json              = EXCLUDED.value_json,
		       revision                 = plugin_settings.revision + 1,
		       schema_version_at_write  = EXCLUDED.schema_version_at_write,
		       updated_at               = NOW()
		RETURNING revision
	`, pluginName, key, string(value), schemaVersion)
	var rev int64
	if err := row.Scan(&rev); err != nil {
		return 0, fmt.Errorf("plugin_settings: upsert: %w", err)
	}

	// V5/W6 SETTINGS-V2: attach RequiresReload from cached meta.
	requiresReload := false
	if propMeta, ok := meta[key]; ok {
		requiresReload = propMeta.RequiresReload
	}

	s.notify(PluginSettingsChange{
		Plugin:         pluginName,
		Key:            key,
		Value:          append(json.RawMessage(nil), value...),
		Revision:       rev,
		RequiresReload: requiresReload,
	})
	return rev, nil
}

// SetSource 表示写入来源。Admin handler 用 SetSourceAdmin，内部代码用 SetSourceInternal
// 跳过 backend-only 保护。
type SetSource int

const (
	SetSourceUnknown SetSource = iota
	SetSourceAdmin
	SetSourceInternal
)

// ErrPluginSettingsBackendOnly is returned when admin tries to write a
// key whose schema has x-visibility=backend.
type ErrPluginSettingsBackendOnly struct {
	Plugin string
	Key    string
}

func (e *ErrPluginSettingsBackendOnly) Error() string {
	return fmt.Sprintf("plugin_settings: %s/%s is backend-only and not writable via admin API", e.Plugin, e.Key)
}
```

**`PluginSettingsChange` 扩展**（service struct）：

```go
type PluginSettingsChange struct {
	Plugin         string
	Key            string
	Value          json.RawMessage
	Revision       int64
	RequiresReload bool // V5/W6 SETTINGS-V2: mirrors x-requires-reload
}
```

### 4.4 PluginManager 订阅 reload 事件

**新增方法**（`backend/internal/plugin/manager.go`）：

```go
// startSettingsReloadWatcher subscribes to PluginSettingsService events and
// triggers a plugin restart whenever a key with x-requires-reload=true is
// updated. Idempotent — calling it more than once is a no-op.
//
// This is launched once from PluginManager.Start (right after settings
// service is wired). The goroutine lives for the lifetime of the manager.
func (m *PluginManager) startSettingsReloadWatcher(ctx context.Context) {
	if m.settingsService == nil {
		return
	}
	// Subscribe to all plugins, all keys (empty plugin name selector means
	// "fan out to namespace-level subs"). The service's Subscribe currently
	// is per-plugin; we wrap by re-subscribing on every plugin enable.
	go m.runReloadWatcher(ctx)
}

func (m *PluginManager) runReloadWatcher(ctx context.Context) {
	// We cannot subscribe to "all plugins" in one call — current Subscribe
	// signature is (pluginName, key). Instead, register per-plugin watchers
	// inside spawnAndConnect after RegisterSchema succeeds. This function
	// then becomes a no-op placeholder. See spawnAndConnect changes below.
	<-ctx.Done()
}
```

**`spawnAndConnect` 修改**（`manager.go:833-845` 之后追加）：

```go
	// V5/W6 SETTINGS-V2: subscribe to RequiresReload events for this plugin.
	// The subscription cleans up when the plugin is stopped via the closure
	// captured below.
	if m.settingsService != nil && len(manifest.GetSettingsSchemaJson()) > 0 {
		ch, unsubscribe := m.settingsService.Subscribe(inst.Name, "")
		go m.handlePluginSettingsEvents(inst.Name, ch)
		inst.settingsUnsubscribe = unsubscribe
	}
```

**新增 PluginInstance 字段**（`backend/internal/plugin/instance.go` 或 manager.go 内部）：

```go
type PluginInstance struct {
	// ... existing fields
	settingsUnsubscribe func() // V5/W6 SETTINGS-V2: cleanup for reload watcher
}
```

**`stopInstance` 修改**：在 `cancelProc()` 之前调用 `inst.settingsUnsubscribe()`（如果非 nil）。

**`handlePluginSettingsEvents` 实现**（pin 死，避免循环重启）：

```go
// handlePluginSettingsEvents drains the per-plugin settings change channel
// and triggers a plugin reload when any change is marked RequiresReload.
//
// Reload coalescing: multiple RequiresReload events arriving within
// reloadCoalesceWindow are folded into a single reload. This protects
// against rapid-fire admin saves that would otherwise restart the plugin
// process every few seconds.
const reloadCoalesceWindow = 2 * time.Second

func (m *PluginManager) handlePluginSettingsEvents(pluginName string, ch <-chan service.PluginSettingsChange) {
	var pendingReason string
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for change := range ch {
		if !change.RequiresReload {
			continue
		}
		pendingReason = "settings_change:" + change.Key
		if timer == nil {
			timer = time.AfterFunc(reloadCoalesceWindow, func() {
				m.reloadPlugin(context.Background(), pluginName, pendingReason)
			})
		} else {
			timer.Reset(reloadCoalesceWindow)
		}
	}
}

// reloadPlugin stops the plugin (preserving the enabled flag) and re-spawns
// it. Mirrors RestartPlugin but with explicit reason logging.
func (m *PluginManager) reloadPlugin(ctx context.Context, name, reason string) {
	m.logger.Info("plugin reload triggered by settings change",
		"plugin", name, "reason", reason)
	if err := m.RestartPlugin(ctx, name); err != nil {
		m.logger.Error("plugin reload failed",
			"plugin", name, "reason", reason, "error", err)
	}
}
```

### 4.5 `Watch` schema 升级处理（关流让客户端重连）

**问题**：当 plugin 升级 schema_version（RegisterSchema 重新跑）时，已经在 watch 的 SDK 客户端怎么知道？

**决定**：**关流**（不发 special event）。当 RegisterSchema 检测到 schema_version 变化时，关掉所有该 plugin 的现有订阅，让 SDK 客户端通过现有的重连 + list-then-watch 机制重新拿全量。

**理由 pin 死**：
- 加 `WatchEventKind` enum 会让 proto 复杂化，且 SDK 客户端反正都要重连。
- INSPECT §2.4 已经描述了 sendSnapshot 在新订阅打开时发全量。重连后 SDK 自然拿到最新值 + 最新 schema_version。

**`RegisterSchema` 检测版本变化伪代码**：

```go
// 在 RegisterSchema 持久化前先读旧 version：
var prevVersion string
s.mu.RLock()
prevVersion = s.schemaVersions[in.PluginName]
s.mu.RUnlock()

// ... 持久化 + 缓存更新 ...

// 如果 schema_version 变了，让现有订阅重连。
if prevVersion != "" && prevVersion != schemaVersion {
	s.dropAllSubscribersForPlugin(in.PluginName)
	s.logger.Info("plugin_settings: schema_version changed; dropped subscribers to force resync",
		"plugin", in.PluginName,
		"prev", prevVersion, "current", schemaVersion)
}
```

**新方法 `dropAllSubscribersForPlugin`**（service 内部）：

```go
func (s *PluginSettingsService) dropAllSubscribersForPlugin(pluginName string) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	subs := s.subs[pluginName]
	for _, sub := range subs {
		close(sub.ch) // SDK runWatchLoop 收到 stream lost → 重连
	}
	delete(s.subs, pluginName)
}
```

> 注意：`gRPC Watch` 实现（`settings_extension_server.go:115-149`）的 `for { … case evt, ok := <-ch: if !ok { return nil } }` 已正确处理 channel close（return nil 让 stream 自然结束），SDK 端会收到 EOF 触发重连。

### 4.6 admin handler `Get/:plugin` 响应格式扩展

**改前** 现有 `PluginSettingsSchemaInfo` struct（line 79-85）：

```go
type PluginSettingsSchemaInfo struct {
	Plugin    string                     `json:"plugin"`
	Schema    json.RawMessage            `json:"schema"`
	Defaults  json.RawMessage            `json:"defaults"`
	Values    map[string]json.RawMessage `json:"values"`
	UpdatedAt time.Time                  `json:"updated_at"`
}
```

**改后**：

```go
type PluginSettingsSchemaInfo struct {
	Plugin    string                     `json:"plugin"`
	Schema    json.RawMessage            `json:"schema"`
	Defaults  json.RawMessage            `json:"defaults"`
	Values    map[string]json.RawMessage `json:"values"`           // secret keys → null
	UpdatedAt time.Time                  `json:"updated_at"`

	// V5/W6 SETTINGS-V2 fields:

	// SchemaVersion mirrors plugin_settings_schemas.schema_version.
	SchemaVersion string `json:"schema_version"`

	// PropertiesMeta is the marker triple per top-level property. Keys
	// match Schema's top-level properties; absent keys default to
	// {visibility:"frontend", deprecated:"", requires_reload:false}.
	PropertiesMeta map[string]PropertyMetadata `json:"properties_meta"`

	// SecretKeys is the list of properties with visibility=="secret"
	// that have a stored value (i.e. front-end can show "已配置"). Values
	// for these keys in the Values map are JSON null. Empty slice when
	// no secrets are configured.
	SecretKeys []string `json:"secret_keys"`
}
```

**`SchemaInfo` 实现修改**：在现有 line 335-344 处的 return 之前增加 mask + secret_keys 提取。

```go
	// 从缓存的 meta 抽 visibility / deprecated / requires_reload。
	s.mu.RLock()
	meta := s.propertiesMeta[pluginName]
	schemaVersion := s.schemaVersions[pluginName]
	s.mu.RUnlock()

	// Build SecretKeys: keys with visibility=secret AND a stored value.
	secretKeys := make([]string, 0)
	for prop, m := range meta {
		if m.Visibility == "secret" {
			if _, ok := values[prop]; ok {
				secretKeys = append(secretKeys, prop)
			}
		}
	}
	// Mask secret values to JSON null.
	for _, k := range secretKeys {
		values[k] = json.RawMessage("null")
	}
	sort.Strings(secretKeys) // deterministic order for tests + UI
```

**Implementer 注意**：`sort` 需要 import；secretKeys 顺序对前端 diff 友好。

### 4.7 admin handler `Update` (PUT) 改造（决策 2.5 secret 写入语义）

**约束**：现有 `PUT /api/v1/admin/plugin-settings/:plugin/:key` **是单 key 接口**，body shape `{"value": <raw JSON>}`（INSPECT §2.5）。

**问题**：决策 2.5 要求"未提及 secret key 保持原值"——但单 key 接口本身就是「这次只改这一个 key」，"未提及"语义已经天然满足。

**只需要新增**：
- 「显式空字符串 → 删除 row」语义（针对 secret 字段）

**改后 `Update` 行为**：

```go
func (h *PluginSettingsHandler) Update(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	pluginName, ok := h.parsePlugin(c)
	if !ok {
		return
	}
	key, ok := h.parseKey(c)
	if !ok {
		return
	}
	var req UpdatePluginSettingValueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// V5/W6 SETTINGS-V2: secret + empty string → delete row.
	visibility, _ := h.service.PropertyVisibility(c.Request.Context(), pluginName, key)
	if visibility == "secret" && isEmptyJSONString(req.Value) {
		if err := h.service.DeleteByKey(c.Request.Context(), pluginName, key); err != nil {
			response.InternalError(c, err.Error())
			return
		}
		response.Success(c, gin.H{
			"plugin":   pluginName,
			"key":      key,
			"deleted":  true,
		})
		return
	}

	rev, err := h.service.SetByKey(c.Request.Context(), pluginName, key, req.Value, service.SetSourceAdmin)
	if err != nil {
		var ve *service.ErrPluginSettingsValidation
		var be *service.ErrPluginSettingsBackendOnly
		switch {
		case errors.As(err, &ve):
			response.ErrorWithDetails(c, http.StatusUnprocessableEntity,
				"plugin settings validation failed", ve.Reason, map[string]string{
					"plugin": pluginName,
					"key":    key,
				})
		case errors.As(err, &be):
			response.ErrorWithDetails(c, http.StatusForbidden,
				"plugin settings key is backend-only", be.Error(), map[string]string{
					"plugin": pluginName,
					"key":    key,
				})
		case errors.Is(err, service.ErrPluginSettingsSchemaMissing):
			response.Error(c, http.StatusConflict, "plugin schema not registered; restart the plugin first")
		default:
			response.InternalError(c, err.Error())
		}
		return
	}
	response.Success(c, gin.H{
		"plugin":   pluginName,
		"key":      key,
		"value":    req.Value,
		"revision": rev,
	})
}

// isEmptyJSONString returns true when raw is exactly the JSON string `""`.
// All other values (null, "abc", numbers, objects) return false.
func isEmptyJSONString(raw json.RawMessage) bool {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return false
	}
	return s == ""
}
```

**新增 service 方法**：

```go
// PropertyVisibility returns the visibility marker for the named key, or
// "frontend" when no marker is declared. Returns ("", false) when the
// plugin has no registered schema.
func (s *PluginSettingsService) PropertyVisibility(ctx context.Context, plugin, key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	meta, ok := s.propertiesMeta[plugin]
	if !ok {
		return "", false
	}
	if m, ok := meta[key]; ok {
		if m.Visibility == "" {
			return "frontend", true
		}
		return m.Visibility, true
	}
	return "frontend", true
}

// DeleteByKey removes one (plugin, key) row and fans out a tombstone
// PluginSettingsChange so subscribers know the value is gone.
func (s *PluginSettingsService) DeleteByKey(ctx context.Context, plugin, key string) error {
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM plugin_settings WHERE plugin_name = $1 AND key = $2
	`, plugin, key); err != nil {
		return fmt.Errorf("plugin_settings: delete: %w", err)
	}
	s.notify(PluginSettingsChange{
		Plugin: plugin,
		Key:    key,
		Value:  nil, // tombstone
	})
	return nil
}
```

**Implementer 注意**：tombstone 事件 SDK 端 `applyEvent` 收到 `value=nil` 时会缓存为「存在但值是 null」，下次 Get 会刷成"不存在"。这是可接受的行为——secret 删除是低频操作。

---

## 5. 设计 5：UI Widget Map（对应 Curator Step 10-11）

### 5.1 目录结构（pin 死）

```
frontend/src/components/admin/plugin-settings-widgets/
├── index.ts                    # widget map + resolveWidget()
├── types.ts                    # PropDescriptor + Widget interface
├── widgets/
│   ├── BooleanCheckbox.vue
│   ├── EnumSelect.vue
│   ├── NumberInput.vue
│   ├── IntegerInput.vue
│   ├── StringInput.vue
│   ├── JsonTextarea.vue
│   └── SecretInput.vue
└── decorators/
    ├── DeprecatedBadge.vue
    └── RequiresReloadBadge.vue
```

**Implementer 必读**：所有文件名严格按上面写。**禁止**改成 `BooleanWidget.vue` / `EnumWidget.vue` 等含 "Widget" 后缀的名字（避免和 `Widget` 类型混淆）。

### 5.2 `types.ts`（pin 死）

```typescript
import type { Component } from 'vue'

// PropertyMetadata mirrors backend service.PropertyMetadata. Keep in sync
// with backend/internal/service/plugin_settings_service.go.
export interface PropertyMetadata {
  visibility: 'frontend' | 'backend' | 'secret'
  deprecated: string
  requires_reload: boolean
}

// PropDescriptor is the V5/W6 SETTINGS-V2 form descriptor. Built by
// PluginSettingsForm.vue:buildPropDescriptor() from the schema node + the
// PropertiesMeta map returned by the admin GET response.
export interface PropDescriptor {
  key: string
  type: 'boolean' | 'string' | 'number' | 'integer' | 'enum' | 'json' | 'secret'
  title: string
  description: string

  // Optional schema constraints — only enum currently surfaces in widgets;
  // others are server-validated.
  enumValues?: Array<{ value: unknown; label: string }>

  // V5/W6 SETTINGS-V2 markers:
  visibility: 'frontend' | 'backend' | 'secret'
  deprecated: string                // empty when not deprecated
  requiresReload: boolean

  // For secret fields only: whether the key currently has a stored value
  // (admin should see "已配置" placeholder instead of empty).
  isConfigured?: boolean
}

// Widget is a Vue component bound to one PropDescriptor.type (or 'secret').
// All widgets receive identical props for uniformity.
export interface WidgetProps {
  prop: PropDescriptor
  modelValue: unknown
}

// Widgets emit update:modelValue with the new value (pre-typed as much as
// possible — number for numeric widgets, string for string widgets, etc.).
// PluginSettingsForm.vue handles dirty tracking + save dispatching.

// Decorator is a wrapper component that conditionally injects deprecated /
// requires-reload affordances around the inner widget.
export type Widget = Component
export type Decorator = Component
```

### 5.3 `index.ts`（widget map + resolveWidget — pin 死）

```typescript
import type { Component } from 'vue'

import BooleanCheckbox from './widgets/BooleanCheckbox.vue'
import EnumSelect from './widgets/EnumSelect.vue'
import IntegerInput from './widgets/IntegerInput.vue'
import JsonTextarea from './widgets/JsonTextarea.vue'
import NumberInput from './widgets/NumberInput.vue'
import SecretInput from './widgets/SecretInput.vue'
import StringInput from './widgets/StringInput.vue'

import DeprecatedBadge from './decorators/DeprecatedBadge.vue'
import RequiresReloadBadge from './decorators/RequiresReloadBadge.vue'

import type { PropDescriptor, Widget } from './types'

export type WidgetKey =
  | 'boolean'
  | 'string'
  | 'secret'
  | 'number'
  | 'integer'
  | 'enum'
  | 'json'

// Single source of truth: WidgetKey → component. Resolution is via
// resolveWidget() — never index this map directly from outside.
const widgets: Record<WidgetKey, Widget> = {
  boolean: BooleanCheckbox,
  string: StringInput,
  secret: SecretInput,
  number: NumberInput,
  integer: IntegerInput,
  enum: EnumSelect,
  json: JsonTextarea,
}

// resolveWidget picks the rendering component for a property descriptor.
// Order of precedence (higher wins):
//   1. visibility=='secret'        → SecretInput
//   2. enumValues non-empty        → EnumSelect
//   3. type matches a widget       → corresponding widget
//   4. fallback                    → JsonTextarea
export function resolveWidget(prop: PropDescriptor): Widget {
  if (prop.visibility === 'secret') return widgets.secret
  if (prop.enumValues && prop.enumValues.length > 0) return widgets.enum
  return widgets[prop.type as WidgetKey] ?? widgets.json
}

export { DeprecatedBadge, RequiresReloadBadge }
export type { PropDescriptor, PropertyMetadata, Widget } from './types'
```

### 5.4 7 个 widget 实现要点（每个 ≤ 25 行）

> 通用约束：每个 widget 用 `defineProps<WidgetProps>()` + `defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()`。所有 i18n 通过 `useI18n()`，禁止硬编码中文。Element Plus + Tailwind 二选一保持一致——观测现有 `PluginSettingsForm.vue` 使用 Tailwind utility class，新 widget 沿用 Tailwind。

#### 5.4.1 `BooleanCheckbox.vue`

```vue
<template>
  <input
    type="checkbox"
    class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
    :checked="Boolean(modelValue)"
    @change="(e) => emit('update:modelValue', (e.target as HTMLInputElement).checked)"
  />
</template>

<script setup lang="ts">
import type { WidgetProps } from '../types'
defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()
</script>
```

#### 5.4.2 `EnumSelect.vue`

```vue
<template>
  <select
    class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
    :value="String(modelValue ?? '')"
    @change="(e) => onChange((e.target as HTMLSelectElement).value)"
  >
    <option v-for="opt in prop.enumValues" :key="String(opt.value)" :value="String(opt.value)">
      {{ opt.label }}
    </option>
  </select>
</template>

<script setup lang="ts">
import type { WidgetProps } from '../types'
const props = defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()
function onChange(raw: string) {
  const match = props.prop.enumValues?.find((o) => String(o.value) === raw)
  emit('update:modelValue', match ? match.value : raw)
}
</script>
```

#### 5.4.3 `NumberInput.vue` + 5.4.4 `IntegerInput.vue`

两者只差 `step="any"` 与 `step="1"` + parse 函数。可以合并实现，但**文件保持分开**（Curator §5.1 明确列了 7 个 widget）。

```vue
<!-- NumberInput.vue -->
<template>
  <input
    type="number"
    class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
    step="any"
    :value="numberValue"
    @input="(e) => onInput((e.target as HTMLInputElement).value)"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { WidgetProps } from '../types'
const props = defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()
const numberValue = computed(() => {
  const v = props.modelValue
  if (typeof v === 'number') return String(v)
  if (typeof v === 'string') return v
  return ''
})
function onInput(raw: string) {
  const n = Number.parseFloat(raw)
  emit('update:modelValue', Number.isNaN(n) ? raw : n)
}
</script>
```

`IntegerInput.vue` 与上同，差异：`step="1"`，`onInput` 用 `Number.parseInt(raw, 10)`。

#### 5.4.5 `StringInput.vue`

```vue
<template>
  <input
    type="text"
    class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
    :value="String(modelValue ?? '')"
    @input="(e) => emit('update:modelValue', (e.target as HTMLInputElement).value)"
  />
</template>

<script setup lang="ts">
import type { WidgetProps } from '../types'
defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()
</script>
```

#### 5.4.6 `JsonTextarea.vue`

```vue
<template>
  <textarea
    rows="4"
    class="block w-full rounded-md font-mono text-xs border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
    :value="rawJson"
    @input="(e) => onInput((e.target as HTMLTextAreaElement).value)"
  />
  <p v-if="parseError" class="mt-1 text-xs text-red-600 dark:text-red-400">
    {{ parseError }}
  </p>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { WidgetProps } from '../types'
const props = defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()
const parseError = ref('')
const rawJson = computed(() => {
  if (props.modelValue === undefined) return ''
  try { return JSON.stringify(props.modelValue, null, 2) } catch { return String(props.modelValue) }
})
function onInput(raw: string) {
  if (raw.trim() === '') { parseError.value = ''; emit('update:modelValue', ''); return }
  try { const parsed = JSON.parse(raw); parseError.value = ''; emit('update:modelValue', parsed) }
  catch (err) { parseError.value = err instanceof Error ? err.message : String(err) }
}
</script>
```

#### 5.4.7 `SecretInput.vue`

```vue
<template>
  <input
    type="password"
    class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
    :placeholder="placeholder"
    :value="String(modelValue ?? '')"
    @input="(e) => emit('update:modelValue', (e.target as HTMLInputElement).value)"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WidgetProps } from '../types'
const props = defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()
const { t } = useI18n()
const placeholder = computed(() =>
  props.prop.isConfigured
    ? t('admin.pluginSettings.secretConfigured')
    : t('admin.pluginSettings.secretEmpty')
)
</script>
```

### 5.5 2 个 decorator 实现

#### 5.5.1 `DeprecatedBadge.vue`

```vue
<template>
  <div class="space-y-1">
    <div class="flex items-center gap-2">
      <span class="line-through opacity-60"><slot name="label" /></span>
      <span class="inline-flex items-center rounded bg-yellow-100 px-1.5 py-0.5 text-xs font-medium text-yellow-800 dark:bg-yellow-900/40 dark:text-yellow-200">
        {{ t('admin.pluginSettings.deprecated') }}
      </span>
    </div>
    <p class="text-xs text-yellow-700 dark:text-yellow-300">{{ message }}</p>
    <slot />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ message: string }>()
const { t } = useI18n()
</script>
```

#### 5.5.2 `RequiresReloadBadge.vue`

```vue
<template>
  <p class="mt-1 text-xs text-orange-600 dark:text-orange-400">
    {{ t('admin.pluginSettings.requiresReload') }}
  </p>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>
```

### 5.6 重构后的 `PluginSettingsForm.vue` 完整骨架（≤ 60 行）

> Implementer **完全替换**原有 220 行实现为下面骨架。`onChange` / `save` 等方法从原文件保留搬过来（约 50 行业务逻辑）。

```vue
<template>
  <div class="space-y-4">
    <div v-if="!schemaProperties.length" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.pluginSettings.emptySchema') }}
    </div>
    <div
      v-for="prop in schemaProperties"
      :key="prop.key"
      class="rounded-md border border-gray-200 dark:border-gray-700 p-4"
    >
      <component
        :is="prop.deprecated ? DeprecatedBadge : 'div'"
        :message="prop.deprecated"
      >
        <template #label>
          <label class="block text-sm font-medium text-gray-900 dark:text-gray-100">
            {{ prop.title || prop.key }}
          </label>
        </template>
        <p v-if="prop.description" class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {{ prop.description }}
        </p>
        <div class="mt-3">
          <component
            :is="resolveWidget(prop)"
            :prop="prop"
            :model-value="localValues[prop.key]"
            @update:model-value="(v) => onChange(prop, v)"
          />
          <RequiresReloadBadge v-if="prop.requiresReload" />
          <p v-if="errors[prop.key]" class="mt-1 text-xs text-red-600 dark:text-red-400">
            {{ errors[prop.key] }}
          </p>
        </div>
        <div class="mt-3 flex justify-end">
          <button
            type="button"
            class="inline-flex items-center rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50"
            :disabled="!dirty[prop.key] || saving === prop.key"
            @click="save(prop)"
          >
            {{ saving === prop.key ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </component>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { pluginSettingsApi, type PluginSettingsSchemaInfo } from '@/api/admin/pluginSettings'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { resolveWidget, DeprecatedBadge, RequiresReloadBadge, type PropDescriptor } from './plugin-settings-widgets'

const props = defineProps<{ info: PluginSettingsSchemaInfo }>()
const emit = defineEmits<{ (e: 'updated', key: string, value: unknown): void }>()
const { t } = useI18n()
const appStore = useAppStore()

const schemaProperties = computed<PropDescriptor[]>(() => buildPropDescriptors(props.info))
const localValues = reactive<Record<string, unknown>>({ ...(props.info.values ?? {}) })
const dirty = reactive<Record<string, boolean>>({})
const errors = reactive<Record<string, string>>({})
const saving = ref<string | null>(null)

watch(() => props.info, (next) => {
  Object.keys(localValues).forEach((k) => delete localValues[k])
  Object.assign(localValues, next.values ?? {})
  Object.keys(dirty).forEach((k) => delete dirty[k])
  Object.keys(errors).forEach((k) => delete errors[k])
})

function onChange(prop: PropDescriptor, v: unknown) {
  localValues[prop.key] = v
  dirty[prop.key] = true
  delete errors[prop.key]
}

async function save(prop: PropDescriptor) {
  if (errors[prop.key]) return
  saving.value = prop.key
  try {
    // V5/W6 SETTINGS-V2 secret semantics: empty input == do nothing.
    const value = localValues[prop.key]
    if (prop.visibility === 'secret' && value === '') {
      // empty string when previously configured → user explicitly clearing.
      // empty string when NOT configured → user did not type anything → skip.
      if (!prop.isConfigured) { dirty[prop.key] = false; return }
    }
    await pluginSettingsApi.update(props.info.plugin, prop.key, value)
    dirty[prop.key] = false
    emit('updated', prop.key, value)
    appStore.showSuccess(t('admin.pluginSettings.saveSuccess'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    saving.value = null
  }
}

// buildPropDescriptors — 见下面 §5.7
import { buildPropDescriptors } from './plugin-settings-widgets/buildPropDescriptors'
</script>
```

### 5.7 `buildPropDescriptors` 辅助函数

新增文件 `frontend/src/components/admin/plugin-settings-widgets/buildPropDescriptors.ts`：

```typescript
import type { PluginSettingsSchemaInfo } from '@/api/admin/pluginSettings'
import type { PropDescriptor, PropertyMetadata } from './types'

const DEFAULT_META: PropertyMetadata = {
  visibility: 'frontend',
  deprecated: '',
  requires_reload: false,
}

export function buildPropDescriptors(info: PluginSettingsSchemaInfo): PropDescriptor[] {
  const schema = info.schema as Record<string, unknown> | undefined
  if (!schema || typeof schema !== 'object') return []
  const propsMap = schema['properties'] as Record<string, Record<string, unknown>> | undefined
  if (!propsMap) return []
  const meta = (info.properties_meta ?? {}) as Record<string, PropertyMetadata>
  const secretSet = new Set(info.secret_keys ?? [])

  return Object.keys(propsMap).map((key) => {
    const node = propsMap[key] || {}
    const m = meta[key] ?? DEFAULT_META
    const enumVals = Array.isArray(node['enum']) ? (node['enum'] as unknown[]) : undefined
    const inferredType = (node['type'] as string) ?? 'json'
    const type: PropDescriptor['type'] =
      m.visibility === 'secret'
        ? 'secret'
        : enumVals
          ? 'enum'
          : (['boolean', 'string', 'number', 'integer'].includes(inferredType)
              ? (inferredType as PropDescriptor['type'])
              : 'json')

    return {
      key,
      type,
      title: typeof node['title'] === 'string' ? (node['title'] as string) : key,
      description: typeof node['description'] === 'string' ? (node['description'] as string) : '',
      enumValues: enumVals?.map((v) => ({ value: v, label: String(v) })),
      visibility: m.visibility,
      deprecated: m.deprecated,
      requiresReload: m.requires_reload,
      isConfigured: secretSet.has(key),
    }
  })
}
```

**Implementer 注意**：原来 `PluginSettingsForm.vue:124` 用 `Object.keys(propsMap).sort()` 字典序，按 INDUSTRY §3 行 10 决策**改用 schema 声明顺序**。`Object.keys` 在 ES2015 spec 下 string keys 保持插入顺序，新代码移除 `.sort()`。

---

## 6. 设计 6：默认值读时填充（对应 Curator Step 6/9）

### 6.1 现状 + 决策

**现状**（INSPECT §2.6 + INDUSTRY §3 行 8）：`seedDefaults` 在 RegisterSchema 时 `INSERT … ON CONFLICT DO NOTHING` 把 default 物理写入 `plugin_settings` 表。INDUSTRY 推荐改为「读时填充」（Backstage / VS Code 风格）。

**决定（pin 死）**：**保留启动期 seedDefaults + 增加读时填充的二级兜底**。理由：
- 完全去掉 seedDefaults 风险大：Watch 流的 sendSnapshot（INSPECT §2.4 line 153-184）依赖 `GetAll` 拿到行才能发出来。如果只读时填，Watch 不会广播 default 值。
- 二级兜底解决"plugin 升级 schema 加新字段，新字段没出现在 `seedDefaults` 之前 plugin 已经在跑 GetTyped"的窗口期问题。

**实现位置**：`PluginSettingsService.GetByKey`（line 219-232）+ admin handler 的 `Get` 间接通过 `SchemaInfo` 也走相同路径。

### 6.2 `GetByKey` 改造

```go
// GetByKey reads one key. If no row exists, it falls back to the schema default
// (V5/W6 SETTINGS-V2 read-time fill). Returns (nil, 0, sql.ErrNoRows) only when
// neither the row nor a default exists.
func (s *PluginSettingsService) GetByKey(
	ctx context.Context, pluginName, key string,
) (json.RawMessage, int64, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT value_json::text, revision FROM plugin_settings
		WHERE plugin_name = $1 AND key = $2
	`, pluginName, key)
	var raw string
	var rev int64
	if err := row.Scan(&raw, &rev); err == nil {
		return json.RawMessage(raw), rev, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, 0, err
	}

	// V5/W6 SETTINGS-V2: read-time default fallback.
	if def, ok := s.lookupDefault(pluginName, key); ok {
		return def, 0, nil // revision=0 indicates "synthetic default"
	}
	return nil, 0, sql.ErrNoRows
}

// lookupDefault returns the schema default for (plugin, key) from cached
// defaultsJSON. Returns (nil, false) when none.
func (s *PluginSettingsService) lookupDefault(plugin, key string) (json.RawMessage, bool) {
	s.mu.RLock()
	rawDefaults := s.defaults[plugin]
	s.mu.RUnlock()
	if len(rawDefaults) == 0 {
		return nil, false
	}
	var defaults map[string]json.RawMessage
	if err := json.Unmarshal(rawDefaults, &defaults); err != nil {
		return nil, false
	}
	if v, ok := defaults[key]; ok {
		return append(json.RawMessage(nil), v...), true
	}
	return nil, false
}
```

### 6.3 Race condition：admin 改 key=foo 时 plugin 改了 schema 的 foo.default

**场景**：
- 时间 T1：plugin schema `foo.default = 1`
- 时间 T2：admin PUT `foo = 5` （写入成功）
- 时间 T3：plugin 重启，schema 升级为 `foo.default = 2`，schema_version 从 "1.0.0" → "2.0.0"
- 读时：`GetByKey("foo")` 应该返回什么？

**决策（pin 死）**：DB 行存在 → 用 DB 行（5），无视新 default。理由：
- "admin 写过的值优先于 schema default" 是直觉行为（VS Code、Backstage 都这样）。
- 即使 `schema_version_at_write` 是旧版本（"1.0.0"），值本身 `5` 仍然是 admin 的真实意图，不应被 schema default 覆盖。
- plugin 拿到 `5` 后通过 `SchemaVersionMismatchError` 知道这是老 schema 写的，自己决定要不要 fallback。

**写入并发**：admin PUT 和 plugin RegisterSchema 之间没有共享锁——两者都串行通过 PluginSettingsService 的 `s.mu`。RegisterSchema 持 write lock 时，并发的 SetByKey 在 `s.mu.RLock()` 处等。**不需要新增锁**。

---

## 7. 设计 7：i18n + 错误信息

### 7.1 i18n keys（中英文成对）

**改文件**：
- `frontend/src/i18n/locales/en.ts` 现有 `admin.pluginSettings.*` 区块（line 975-981）
- `frontend/src/i18n/locales/zh.ts` 现有 `admin.pluginSettings.*` 区块（line 979-985）

**新增 key**（按字母序）：

| key | en | zh |
|---|---|---|
| `admin.pluginSettings.deprecated` | `Deprecated` | `已废弃` |
| `admin.pluginSettings.deprecatedHint` | `This field is deprecated and may be removed in a future plugin version.` | `此字段已废弃，未来版本可能移除` |
| `admin.pluginSettings.requiresReload` | `Saving this field will reload the plugin.` | `保存此字段将重启此插件` |
| `admin.pluginSettings.secretConfigured` | `(configured — leave empty to keep, type space to clear)` | `（已配置，留空保持原值，输空格清除）` |
| `admin.pluginSettings.secretEmpty` | `Enter a value to set the secret` | `输入新值以设置该密钥` |
| `admin.pluginSettings.backendOnly` | `This field is backend-only and cannot be edited from the admin UI.` | `此字段仅由后端修改，无法在管理后台编辑` |
| `admin.pluginSettings.schemaVersion` | `Schema version: {version}` | `Schema 版本：{version}` |
| `admin.pluginSettings.schemaVersionMismatch` | `Stored schema version differs from current; some values may be stale.` | `存储值的 schema 版本与当前不一致，部分值可能已过期` |

**Implementer 注意**：每个 key 必须在 en.ts + zh.ts 同步新增，缺一个 CI 不会报错但用户会看到 `admin.pluginSettings.xxx` 字面量。

### 7.2 后端 errcode + message 模板

> 现状（CLAUDE.md §13.4）：项目错误响应格式 `{"code": ..., "message": "...", "details": {...}}`，code 可以是数字或字符串 const。INSPECT 没有显示统一 errcode 表，本节给出 SETTINGS-V2 新增的 errcode（**字符串 const 形式**，沿用 service 包现有 `Err…` 变量风格）。

| Go const / variable | wire code (string) | English message | details fields |
|---|---|---|---|
| `service.ErrPluginSettingsSchemaMissing` | `PLUGIN_SETTINGS_SCHEMA_MISSING` | `plugin schema not registered` | `{"plugin":"<name>"}` |
| `service.ErrPluginSettingsValidation` | `PLUGIN_SETTINGS_VALIDATION_FAILED` | `plugin settings validation failed: <reason>` | `{"plugin":"<name>","key":"<key>","reason":"<jsonschema error>"}` |
| `service.ErrInvalidSchemaVisibility` | `PLUGIN_SETTINGS_SCHEMA_INVALID_VISIBILITY` | `plugin settings schema: x-visibility must be one of frontend\|backend\|secret` | `{"plugin":"<name>","property":"<prop>","value":"<actual>"}` |
| `service.ErrPluginSettingsBackendOnly` | `PLUGIN_SETTINGS_BACKEND_ONLY` | `plugin settings key is backend-only` | `{"plugin":"<name>","key":"<key>"}` |
| `pluginsdk.ErrSchemaVersionMismatch` | `PLUGIN_SETTINGS_SCHEMA_VERSION_MISMATCH` | `plugin settings schema version mismatch` | `{"key":"<key>","stored":"<v>","current":"<v>"}` (在 plugin 侧 log，不外暴) |

**HTTP 状态码映射**：
- `PLUGIN_SETTINGS_VALIDATION_FAILED` → 422
- `PLUGIN_SETTINGS_SCHEMA_MISSING` → 409
- `PLUGIN_SETTINGS_BACKEND_ONLY` → 403
- `PLUGIN_SETTINGS_SCHEMA_INVALID_VISIBILITY` → 启动期错误，不走 HTTP（plugin status=ErrorBadSettingsSchema）

---

## 8. 设计 8：测试矩阵

> 项目使用 `//go:build unit` 标签（CLAUDE.md §5）和 `make test-unit`/`make test-integration`。所有新增单测沿用此约定。

### 8.1 后端单测

**新文件 1**：`backend/internal/service/plugin_settings_service_v2_test.go`（**新建**，与现有 `*_test.go` 分开避免合并冲突）

```go
//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
)

// 必须的 case 列表 — Implementer 至少实现这些：

func TestRegisterSchema_PersistsSchemaVersion(t *testing.T)
func TestRegisterSchema_NormalisesEmptyVersionToZero(t *testing.T)
func TestRegisterSchema_RejectsInvalidVisibility(t *testing.T)
func TestRegisterSchema_DerivesMetaFromVendorExtensions(t *testing.T)
func TestRegisterSchema_PrefersSdkAuthoritativeMeta(t *testing.T)
func TestRegisterSchema_DropsSubscribersOnVersionChange(t *testing.T)

func TestSetByKey_StampsSchemaVersionAtWrite(t *testing.T)
func TestSetByKey_RejectsBackendOnlyFromAdminSource(t *testing.T)
func TestSetByKey_AllowsBackendOnlyFromInternalSource(t *testing.T)
func TestSetByKey_PropagatesRequiresReloadOnNotify(t *testing.T)

func TestGetByKey_FallsBackToSchemaDefault(t *testing.T)
func TestGetByKey_PrefersStoredValueOverNewDefault(t *testing.T)
func TestGetByKey_ReturnsErrNoRowsWhenNoDefault(t *testing.T)

func TestSchemaInfo_MasksSecretValues(t *testing.T)
func TestSchemaInfo_PopulatesSecretKeysOnlyForConfiguredSecrets(t *testing.T)
func TestSchemaInfo_IncludesSchemaVersionAndPropertiesMeta(t *testing.T)

func TestDeleteByKey_RemovesRowAndFanoutsTombstone(t *testing.T)

func TestPropertyVisibility_DefaultsToFrontend(t *testing.T)
func TestPropertyVisibility_ReturnsDeclared(t *testing.T)
```

**新文件 2**：`plugin-sdk/settings_v2_test.go`（**新建**）

```go
package pluginsdk

import "testing"

func TestGetTyped_ReturnsSchemaVersionMismatchOnDriftedRecord(t *testing.T)
func TestGetTyped_ReturnsPlainErrorOnUnmarshalWithMatchingVersion(t *testing.T)
func TestApplyEvent_PreservesCachedSchemaVersion(t *testing.T)
func TestSchemaVersionMismatchError_IsCompatibleWithSentinel(t *testing.T)
```

### 8.2 前端单测

**新文件**：`frontend/src/components/admin/plugin-settings-widgets/__tests__/PluginSettingsForm.spec.ts`

> 项目用 vitest（沿用现有 `__tests__/*.spec.ts` 约定，没找到已有配置时 Implementer 检查 `frontend/vitest.config.ts`）。

```typescript
describe('PluginSettingsForm widget map', () => {
  it('resolves SecretInput when visibility=secret', () => {})
  it('resolves EnumSelect when enum is non-empty', () => {})
  it('resolves NumberInput for type=number', () => {})
  it('resolves IntegerInput for type=integer', () => {})
  it('falls back to JsonTextarea for unknown type', () => {})

  it('renders DeprecatedBadge when deprecated is non-empty', () => {})
  it('hides DeprecatedBadge when deprecated is empty', () => {})

  it('renders RequiresReloadBadge when requires_reload=true', () => {})

  it('skips save dispatch when secret is empty and isConfigured=false', () => {})
  it('dispatches delete-equivalent (empty string) when secret cleared', () => {})

  it('builds prop descriptors in schema declaration order (no alphabetical sort)', () => {})
  it('uses property_meta.visibility before falling back to default frontend', () => {})
})
```

### 8.3 集成测（手测脚本，列在 PR 描述中）

**channel-management 端到端验证**：

1. 启动 `sub2api-plugin`（test 环境，端口 8087）
2. 在 channel-management plugin schema 中标某字段 `x-deprecated`、某字段 `x-requires-reload`、某字段 `x-visibility:secret`（详见 §9）
3. `curl /api/v1/admin/plugin-settings/channel-management` 验证响应包含 `schema_version` / `properties_meta` / `secret_keys`
4. PUT secret 字段，验证 GET 返回 secret 值为 `null` + `secret_keys` 包含该字段
5. PUT requires-reload 字段，观察日志出现 `plugin reload triggered by settings change`
6. PUT backend-only 字段，验证返回 403
7. PUT secret 字段空字符串，验证 row 被 DELETE

---

## 9. 设计 9：built-in plugin demo（对应 Curator Step 12）

### 9.1 channel-management schema 升级

**文件**：`plugins/channel-management/monitor/settings/settings_schema.json`

**完整 diff**：

```diff
--- a/plugins/channel-management/monitor/settings/settings_schema.json
+++ b/plugins/channel-management/monitor/settings/settings_schema.json
@@ -1,5 +1,6 @@
 {
   "$schema": "http://json-schema.org/draft-07/schema#",
+  "$id": "sub2api.io/schema/plugin-settings-v1",
   "title": "Channel Monitor",
   "description": "Periodic upstream-channel health checks (V5 W6).",
   "type": "object",
@@ -14,7 +15,8 @@
       "title": "Default check interval (seconds)",
       "description": "Pre-filled in the admin Create form. Per-monitor override still applies.",
       "minimum": 15,
       "maximum": 3600,
-      "default": 60
+      "default": 60,
+      "x-requires-reload": true
     },
     "templateMaxBodyKB": {
       "type": "integer",
@@ -27,11 +29,30 @@
     "dailyRollupHourUTC": {
       "type": "integer",
       "title": "Daily rollup hour (UTC, 0-23)",
       "description": "Informational — the host cron currently fires at 02:00 UTC. Reserved for future per-tenant overrides.",
       "minimum": 0,
       "maximum": 23,
-      "default": 2
+      "default": 2,
+      "x-deprecated": "Reserved for future use; current cron is hard-coded to 02:00 UTC. Will be repurposed in V7."
+    },
+    "internalUpstreamProbeKey": {
+      "type": "string",
+      "title": "Internal upstream probe key (DEMO secret)",
+      "description": "Demo of x-visibility:secret. Used only for SETTINGS-V2 smoke testing — leave empty in production.",
+      "x-visibility": "secret"
+    },
+    "_internalCacheTTLSec": {
+      "type": "integer",
+      "title": "Internal monitor cache TTL (seconds)",
+      "description": "Backend-only — controls in-memory monitor cache TTL. Not editable from admin UI. Demo of x-visibility:backend.",
+      "minimum": 5,
+      "maximum": 600,
+      "default": 60,
+      "x-visibility": "backend"
     }
   },
-  "required": ["enabled"]
+  "required": ["enabled"],
+  "x-schema-version": "1.0.0"
 }
```

**说明**：
- `$id` 添加按 INDUSTRY §3 行 4 决策（Backstage meta-schema URL 风格）。
- `x-schema-version` 是冗余记录——真正的 version 走 `Manifest.SettingsSchema.Version`（见下面 plugin.go 改动）。
- `internalUpstreamProbeKey` / `_internalCacheTTLSec` 是 demo 字段，channel-management 业务代码 **不读** 它们——目的是为 SETTINGS-V2 smoke test 提供测试面。

### 9.2 channel-management plugin.go 改动

**改前**（`plugins/channel-management/plugin.go:162-165`）：

```go
SettingsSchema: &pluginsdk.SettingsSchemaDoc{
    Schema:   monitorSettingsSchemaJSON,
    Defaults: monitorSettingsDefaultsJSON,
},
```

**改后**：

```go
SettingsSchema: &pluginsdk.SettingsSchemaDoc{
    Schema:   monitorSettingsSchemaJSON,
    Defaults: monitorSettingsDefaultsJSON,
    Version:  "1.0.0", // V5/W6 SETTINGS-V2 demo
    // PropertyMeta 留空 — 让 host 从 schema vendor extensions 反向推导，
    // 这样 plugin author 不需要重复声明（INDUSTRY §3 行 4 决策）。
},
```

### 9.3 hello-world 是否加 demo？

**决定**：**不加**。理由：
- hello-world 的目的是「最小可运行 plugin」，加入 SETTINGS-V2 demo 字段会污染它的教学清晰度。
- channel-management 已经覆盖所有 marker，足够 smoke test。

---

## 10. 设计 10：上线 + 回滚剧本（对应 Curator 决策 4）

> 4 阶段顺序约束：DB → SDK → host service → UI。**禁止合并阶段**。每个阶段独立 commit，独立可部署，独立可回滚。

### 阶段 1：DB migration（commit 1/4）

**部署**：
```bash
# 在 test 环境（sub2api-test）执行
ssh clicodeplus "cd /root/sub2api-plugin && git pull && \
    docker buildx build --builder limited-builder --no-cache --load -t sub2api:test -f Dockerfile . && \
    cd deploy && docker compose -p sub2api-test up -d --no-deps --force-recreate sub2api"
```

**验证**：
```bash
ssh clicodeplus "docker exec sub2api-test-postgres psql -U plugin -d plugin -c \
    \"SELECT column_name, data_type, column_default FROM information_schema.columns \
       WHERE table_name IN ('plugin_settings', 'plugin_settings_schemas') \
         AND column_name IN ('schema_version', 'schema_version_at_write', 'properties_meta');\""
```

期望输出包含 3 行（schema_version / schema_version_at_write / properties_meta）。

**回滚**：
```bash
# 应用 §1.3 的 ROLLBACK SQL
ssh clicodeplus "docker exec -i sub2api-test-postgres psql -U plugin -d plugin" < rollback_103.sql
```

### 阶段 2：SDK manifest + proto + plugin SDK（commit 2/4）

**部署**：commit 2 包含决策 1（删 `ctx.Config()`）+ 决策 2.1/2.2/2.3/2.4 的 manifest/proto/SDK 部分。重新构建镜像（plugin-sdk 是同 monorepo，channel-management 会重新编译）。

**验证**：
```bash
# 验证 plugin 启动 + 注册 schema 含 version
ssh clicodeplus "docker logs sub2api-test --tail 50 | grep -E 'plugin_settings.*registered|SettingsSchema'"

# Curator §4.1 grep
grep -rn "ctx\.Config()" --include="*.go" plugins/ plugin-sdk/  # 必须 0
```

**回滚**：还原到 commit 1。注意 **DB 已经 migrate**——这没关系，因为 commit 1 的 DB 迁移是向后兼容的（旧代码忽略新列）。

### 阶段 3：host service（commit 3/4）

**部署**：commit 3 包含决策 2.1-2.5 的 host service + admin handler 部分。

**验证**：
```bash
KEY="$ADMIN_API_KEY"
BASE="http://localhost:8087"

# 1. GET 返回新字段
curl -s "${BASE}/api/v1/admin/plugin-settings/channel-management" \
  -H "x-api-key: ${KEY}" | jq '.data | {schema_version, properties_meta, secret_keys}'

# 2. PUT secret，验证 GET 返回 null
curl -X PUT "${BASE}/api/v1/admin/plugin-settings/channel-management/internalUpstreamProbeKey" \
  -H "x-api-key: ${KEY}" -H "Content-Type: application/json" \
  -d '{"value": "test-secret-123"}'

curl -s "${BASE}/api/v1/admin/plugin-settings/channel-management" \
  -H "x-api-key: ${KEY}" | jq '.data.values.internalUpstreamProbeKey'  # → null
curl -s "${BASE}/api/v1/admin/plugin-settings/channel-management" \
  -H "x-api-key: ${KEY}" | jq '.data.secret_keys'  # → ["internalUpstreamProbeKey"]

# 3. PUT backend-only，验证 403
curl -X PUT "${BASE}/api/v1/admin/plugin-settings/channel-management/_internalCacheTTLSec" \
  -H "x-api-key: ${KEY}" -H "Content-Type: application/json" \
  -d '{"value": 30}' -w "\n%{http_code}\n"  # → 403
```

**回滚**：还原到 commit 2。前端在阶段 4 才用新字段，所以阶段 3 回滚时前端仍工作（只是没有 marker 装饰）。

### 阶段 4：UI（commit 4/4）

**部署**：commit 4 包含决策 2.1/2.2/2.3/2.5 的前端 + 决策 3 的 widget 重构。

**验证（手测）**：
1. 打开 admin UI → Plugin Settings → channel-management tab
2. `internalUpstreamProbeKey` 字段渲染为 `<input type="password">`，placeholder 显示中文 "已配置，留空保持原值"
3. `dailyRollupHourUTC` 字段显示删除线 + 黄色 "已废弃" badge
4. `defaultIntervalSec` 字段下方显示橙色 "保存此字段将重启此插件" 提示
5. `_internalCacheTTLSec` 字段渲染但保存按钮 disabled / 提交后弹错误 toast
6. 改 `defaultIntervalSec` 60 → 120 保存，观察容器日志出现 `plugin reload triggered`

**回滚**：还原到 commit 3。后端已支持新字段但前端不显示，admin UI 看到的 `properties_meta` 字段被忽略——可接受。

---

## 附录 A — 完整 proto diff

> 见 §2.1（plugin.proto）+ §2.2（sdk.proto）。本附录是 single-file copy-paste 版本。

### A.1 `plugin-sdk/proto/plugin.proto`

```diff
@@ line 28-32 @@
 message PluginInitRequest {
   string sdk_address = 1;
-  map<string, string> config = 2;
+  reserved 2;
+  reserved "config";
   string plugin_name = 3;
@@ line 145 末尾 @@
   bytes settings_defaults_json = 43;
+  string settings_schema_version = 44;
+  bytes settings_properties_meta_json = 45;
 }
```

### A.2 `plugin-sdk/proto/sdk.proto`

```diff
@@ message SettingsGetResponse @@
   bytes value_json = 1;
   bool exists = 2;
   int64 revision = 3;
+  string stored_schema_version = 4;
+  string current_schema_version = 5;
 }
@@ message SettingsChangeEvent @@
   string key = 1;
   bytes value_json = 2;
   int64 revision = 3;
+  bool requires_reload = 4;
 }
```

---

## 附录 B — 完整 SQL up/down

### B.1 up（已在 §1.2 给出，重复以便单文件参考）

```sql
-- 103_plugin_settings_v2.sql
BEGIN;
ALTER TABLE plugin_settings_schemas
    ADD COLUMN IF NOT EXISTS schema_version TEXT NOT NULL DEFAULT '0';
ALTER TABLE plugin_settings_schemas
    ADD COLUMN IF NOT EXISTS properties_meta JSONB NOT NULL DEFAULT '{}'::jsonb;
COMMENT ON COLUMN plugin_settings_schemas.schema_version IS
    'Plugin-declared schema version (Manifest.SettingsSchema.Version). ''0'' means undeclared.';
COMMENT ON COLUMN plugin_settings_schemas.properties_meta IS
    'Cached marker extraction: {<prop>: {visibility,deprecated,requires_reload,...}}. See SETTINGS-V2-DESIGN §1.4.';
ALTER TABLE plugin_settings
    ADD COLUMN IF NOT EXISTS schema_version_at_write TEXT NOT NULL DEFAULT '0';
COMMENT ON COLUMN plugin_settings.schema_version_at_write IS
    'Schema version active when this row was last written. Compared against plugin_settings_schemas.schema_version to detect stale values.';
CREATE INDEX IF NOT EXISTS idx_plugin_settings_schema_version_at_write
    ON plugin_settings (plugin_name, schema_version_at_write);
COMMIT;
```

### B.2 down（手工执行，不入库）

```sql
BEGIN;
DROP INDEX IF EXISTS idx_plugin_settings_schema_version_at_write;
ALTER TABLE plugin_settings DROP COLUMN IF EXISTS schema_version_at_write;
ALTER TABLE plugin_settings_schemas DROP COLUMN IF EXISTS properties_meta;
ALTER TABLE plugin_settings_schemas DROP COLUMN IF EXISTS schema_version;
COMMIT;
```

---

## 附录 C — 所有新增 Go struct

### C.1 `plugin-sdk/manifest.go`

```go
// 已在 §3.3.1 详细列出
type PropertyMetadata struct {
	Visibility     string `json:"visibility,omitempty"`
	Deprecated     string `json:"deprecated,omitempty"`
	RequiresReload bool   `json:"requires_reload,omitempty"`
}
```

### C.2 `plugin-sdk/settings.go`

```go
// 已在 §3.4.1 详细列出
var ErrSchemaVersionMismatch = errors.New("pluginsdk: settings schema version mismatch")

type SchemaVersionMismatchError struct {
	Key                  string
	StoredSchemaVersion  string
	CurrentSchemaVersion string
	UnderlyingErr        error
}
```

### C.3 `backend/internal/service/plugin_settings_service.go`

```go
// 已在 §4.1 / §4.3 / §4.6 详细列出
type RegisterSchemaInput struct {
	PluginName     string
	SchemaJSON     []byte
	DefaultsJSON   []byte
	SchemaVersion  string
	PropertiesMeta []byte
}

type PropertyMetadata struct {
	Visibility     string `json:"visibility"`
	Deprecated     string `json:"deprecated"`
	RequiresReload bool   `json:"requires_reload"`
}

type SetSource int

const (
	SetSourceUnknown SetSource = iota
	SetSourceAdmin
	SetSourceInternal
)

var ErrInvalidSchemaVisibility = errors.New("plugin_settings: x-visibility must be one of frontend|backend|secret")

type ErrPluginSettingsBackendOnly struct {
	Plugin string
	Key    string
}

// PluginSettingsChange 扩展 RequiresReload
type PluginSettingsChange struct {
	Plugin         string
	Key            string
	Value          json.RawMessage
	Revision       int64
	RequiresReload bool
}

// PluginSettingsSchemaInfo 扩展 SchemaVersion / PropertiesMeta / SecretKeys
type PluginSettingsSchemaInfo struct {
	Plugin         string                      `json:"plugin"`
	Schema         json.RawMessage             `json:"schema"`
	Defaults       json.RawMessage             `json:"defaults"`
	Values         map[string]json.RawMessage  `json:"values"`
	UpdatedAt      time.Time                   `json:"updated_at"`
	SchemaVersion  string                      `json:"schema_version"`
	PropertiesMeta map[string]PropertyMetadata `json:"properties_meta"`
	SecretKeys     []string                    `json:"secret_keys"`
}

// PluginSettingsService 内部新字段
type PluginSettingsService struct {
	// ... existing
	schemaVersions map[string]string // by plugin name (V5/W6 SETTINGS-V2)
	propertiesMeta map[string]map[string]PropertyMetadata
}
```

### C.4 `backend/internal/plugin/manager.go` 周边

```go
// PluginInstance 扩展
type PluginInstance struct {
	// ... existing
	settingsUnsubscribe func()
}
```

---

## 附录 D — 所有新增 TS 类型

### D.1 `frontend/src/api/admin/pluginSettings.ts`

```typescript
// 在 PluginSettingsSchemaInfo 之上新增
export interface PluginSettingsPropertyMetadata {
  visibility: 'frontend' | 'backend' | 'secret'
  deprecated: string
  requires_reload: boolean
}

export interface PluginSettingsSchemaInfo {
  plugin: string
  schema: JSONSchema
  defaults: Record<string, unknown>
  values: Record<string, unknown>
  updated_at?: string
  // V5/W6 SETTINGS-V2 fields:
  schema_version: string
  properties_meta: Record<string, PluginSettingsPropertyMetadata>
  secret_keys: string[]
}
```

### D.2 `frontend/src/components/admin/plugin-settings-widgets/types.ts`

> 已在 §5.2 完整列出，含 `PropertyMetadata`、`PropDescriptor`、`WidgetProps`、`Widget`、`Decorator`。

---

## 附录 E — i18n keys 表

> 见 §7.1 表格。Implementer 复制其 8 行新 key 到 `frontend/src/i18n/locales/{en,zh}.ts` 的 `admin.pluginSettings.*` 区块。**禁止**改 key 命名空间到 `pluginSettings.*` 顶层（现有项目已用 `admin.pluginSettings.*`）。

新增完整 zh.ts 区块：

```typescript
admin: {
  pluginSettings: {
    title: '插件设置',
    description: '查看并修改各插件已注册的可调配置项',
    noPlugins: '当前没有插件注册过设置 schema',
    emptySchema: '该插件未声明任何可调字段',
    saveSuccess: '设置已保存',
    // V5/W6 SETTINGS-V2 新增：
    deprecated: '已废弃',
    deprecatedHint: '此字段已废弃，未来版本可能移除',
    requiresReload: '保存此字段将重启此插件',
    secretConfigured: '（已配置，留空保持原值，输空格清除）',
    secretEmpty: '输入新值以设置该密钥',
    backendOnly: '此字段仅由后端修改，无法在管理后台编辑',
    schemaVersion: 'Schema 版本：{version}',
    schemaVersionMismatch: '存储值的 schema 版本与当前不一致，部分值可能已过期',
  },
  // ...
}
```

新增完整 en.ts 区块：

```typescript
admin: {
  pluginSettings: {
    title: 'Plugin Settings',
    description: 'Inspect and edit the runtime settings each plugin exposes.',
    noPlugins: 'No plugin has registered a settings schema yet.',
    emptySchema: 'This plugin declares no tunable fields.',
    saveSuccess: 'Settings saved.',
    // V5/W6 SETTINGS-V2 新增：
    deprecated: 'Deprecated',
    deprecatedHint: 'This field is deprecated and may be removed in a future plugin version.',
    requiresReload: 'Saving this field will reload the plugin.',
    secretConfigured: '(configured — leave empty to keep, type space to clear)',
    secretEmpty: 'Enter a value to set the secret',
    backendOnly: 'This field is backend-only and cannot be edited from the admin UI.',
    schemaVersion: 'Schema version: {version}',
    schemaVersionMismatch: 'Stored schema version differs from current; some values may be stale.',
  },
  // ...
}
```

---

## 附录 F — errcode 表

| errcode (string const) | HTTP | Go const | message template | details |
|---|---|---|---|---|
| `PLUGIN_SETTINGS_SCHEMA_MISSING` | 409 | `service.ErrPluginSettingsSchemaMissing` | `plugin schema not registered` | `{"plugin":"<n>"}` |
| `PLUGIN_SETTINGS_VALIDATION_FAILED` | 422 | `service.ErrPluginSettingsValidation` | `plugin settings validation failed: <reason>` | `{"plugin":"<n>","key":"<k>","reason":"<r>"}` |
| `PLUGIN_SETTINGS_SCHEMA_INVALID_VISIBILITY` | n/a (startup) | `service.ErrInvalidSchemaVisibility` | `plugin settings schema: x-visibility must be one of frontend\|backend\|secret` | `{"plugin":"<n>","property":"<p>","value":"<v>"}` |
| `PLUGIN_SETTINGS_BACKEND_ONLY` | 403 | `service.ErrPluginSettingsBackendOnly` | `plugin settings key is backend-only` | `{"plugin":"<n>","key":"<k>"}` |
| `PLUGIN_SETTINGS_SCHEMA_VERSION_MISMATCH` | n/a (SDK error) | `pluginsdk.ErrSchemaVersionMismatch` | `plugin settings schema version mismatch` | `{"key":"<k>","stored":"<v1>","current":"<v2>"}` |

---

## 附录 G — Implementer 验收清单（一站式）

打勾确认每一项。

### G.1 Build / Lint

- [ ] `cd backend && go build ./...` 通过
- [ ] `cd plugin-sdk && go build ./...` 通过
- [ ] `cd plugins/hello-world && go build` 通过（无需修改 hello-world 代码）
- [ ] `cd plugins/channel-management && go build` 通过
- [ ] `cd backend && make test-unit` 通过（含 §8.1 新单测全部 pass）
- [ ] `cd backend && golangci-lint run --timeout=5m` 通过
- [ ] `cd frontend && pnpm build` 通过
- [ ] `cd frontend && pnpm lint` 通过
- [ ] `cd frontend && pnpm test` 通过（含 §8.2 新单测全部 pass）

### G.2 Curator §4.1 grep 必为 0

- [ ] `grep -rn "ctx\.Config()" --include="*.go" plugins/ plugin-sdk/` → 0
- [ ] `grep -rn "pctx\.Config()" --include="*.go" plugins/` → 0
- [ ] `grep -rn "\.config\b" plugin-sdk/runner.go | grep -v "//"` → 0
- [ ] `grep -n "config = 2" plugin-sdk/proto/plugin.proto` → 0
- [ ] `grep -n "reserved 2" plugin-sdk/proto/plugin.proto` → 1

### G.3 proto 重新生成无意外 diff

- [ ] `cd plugin-sdk/proto && protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative plugin.proto sdk.proto`
- [ ] `git diff plugin-sdk/proto/pluginsdk/*.pb.go` 只包含 §2 列出的字段增量

### G.4 DB migration

- [ ] `103_plugin_settings_v2.sql` 文件名严格匹配
- [ ] 启动 backend 后查 `\d plugin_settings_schemas` 含 `schema_version` + `properties_meta`
- [ ] 启动 backend 后查 `\d plugin_settings` 含 `schema_version_at_write`
- [ ] 老数据 `SELECT schema_version_at_write FROM plugin_settings LIMIT 5` 全部为 `'0'`

### G.5 4 阶段 commit 顺序

- [ ] commit 1 = DB only（103_plugin_settings_v2.sql）
- [ ] commit 2 = SDK + proto（manifest.go / settings.go / runner.go / context.go / *.proto / *.pb.go）
- [ ] commit 3 = host service + admin handler（plugin_settings_service.go / plugin_settings_handler.go / manager.go）
- [ ] commit 4 = UI（PluginSettingsForm.vue / plugin-settings-widgets/* / pluginSettings.ts / i18n）+ channel-management schema demo

### G.6 上线门禁（test 环境）

- [ ] §10 阶段 4 手测 6 项全部通过
- [ ] `docker logs sub2api-test --tail 200 | grep -E 'ERROR|FAIL'` 无新错误
- [ ] admin UI 打开 channel-management settings tab，4 个 demo 字段（deprecated / requires-reload / secret / backend-only）符合预期渲染

---

## 附录 H — 给下游 Implementer 的常见 FAQ（pin 死答案）

**Q1**：i18n key 的命名空间用 `pluginSettings.*` 还是 `admin.pluginSettings.*`？
**A**：**严格用 `admin.pluginSettings.*`**。现有 i18n 文件已经用这个，新增 key 不能换命名空间，否则旧代码会被 break。

**Q2**：`x-visibility` 默认值是 `"frontend"` 还是 `""`？
**A**：在 schema 中**允许**省略（`""`），但 host 内部解析时 normalise 为 `"frontend"`。前端接收到的 `properties_meta` 字段值始终是非空的 `"frontend"|"backend"|"secret"` 之一。

**Q3**：admin 通过 PUT 写一个 `null` 值给 secret 字段会发生什么？
**A**：`isEmptyJSONString(req.Value)` 返回 false（null 不等于空字符串），走正常 SetByKey。但 jsonschema validation 会拒绝（secret 字段 type 是 `string`），返回 422。这是可接受行为。

**Q4**：plugin 升级 schema_version 后，旧 schema_version 的 plugin_settings 行什么时候清掉？
**A**：**永远不自动清**（Curator 决策 5 已 punt）。next admin 写入会自动 stamp 新 version；plugin 通过 `SchemaVersionMismatchError` 在读取时识别 stale 值并自己 fallback。

**Q5**：`schema_version` 比较是字符串相等还是 semver？
**A**：**字符串相等**。host 当作 opaque string。plugin 自己想要 semver 比较的话在 plugin 代码里做。

**Q6**：`requires_reload` 触发的 reload 在多实例部署下怎么协调？
**A**：本次 PR 只在单实例部署上验证。多实例时所有 host 实例都会订阅 settings change，每个 host 各自重启自己的 plugin 进程（plugin 是 host-local 的）——没有跨实例协调需要。

**Q7**：删 `ctx.Config()` 后，`migration_proxy_server.go:121` 读 `skip_migration` 怎么办？
**A**：`skip_migration` 走 host-side `PluginRecord.Config`（保留），与 SDK 的 `ctx.Config()` 无关。INSPECT §1.3 已证明只有 host-internal 路径读它。决策 1 删除清单仅删 SDK 侧。

**Q8**：前端 `properties_meta` 字段为 `{}` 时怎么渲染？
**A**：`buildPropDescriptors` 默认 `DEFAULT_META` 已处理（visibility=frontend / deprecated="" / requires_reload=false）。所有字段按 string 默认渲染，不显示任何 marker 装饰。

**Q9**：plugin 同时声明 `Manifest.SettingsSchema.PropertyMeta["foo"].Visibility = "secret"` **和** schema JSON 的 `properties.foo` 节点有 `x-visibility: "frontend"`，谁赢？
**A**：**SDK PropertyMeta 赢**（§4.1 `resolvePropertiesMeta` 已 pin 死优先级）。理由：SDK PropertyMeta 是程序化构造，schema vendor extension 是声明式构造——程序化通常代表更严格的 ground truth。

**Q10**：`admin.pluginSettings.schemaVersion` 在哪里展示？
**A**：本次 PR 不强制展示。如果 Implementer 时间充裕，可以在 `PluginSettingsForm.vue` 顶部加一行 `<p class="text-xs text-gray-400">{{ t('admin.pluginSettings.schemaVersion', { version: info.schema_version }) }}</p>`。但不在 §10 阶段 4 验收范围内。

---

## 附录 I — 已知不在本 PR 范围（防止 Implementer 误抢）

下面这些来自 Curator §决策 5 「下次做 / 永不做」，**禁止**在本 PR 实现：

- `markdownDescription` / `markdownEnumDescriptions`（前端无 markdown 组件基建）
- `scope: machine|window|application` 多值
- deprecated 字段的 orphan 行清理 job
- schema_version 的 K8s 风格 conversion webhook
- envelope encryption（DEK + KEK 两层）
- `org_id` / `tenant_id` 多租户列
- 双列 `value_jsonb` + `secure_value_jsonb`
- vue-json-schema-form 库引入

任何 Implementer 看到上述场景的"诱惑"，请**回到 Curator 决策 5** 复核。不增删。

---

**END OF SETTINGS-V2-DESIGN**
