# SETTINGS-V2-CURATE (Curator decision)

> **角色**：Planner + Designer 合并的 Curator。本文档是 Implementer 的执行单。模糊不行，每条决策落到代码层。
>
> **上游输入**：
> - `docs/plugin-architecture/SETTINGS-V2-INSPECT.md`（Inspector B — 现状审计）
> - `docs/plugin-architecture/SETTINGS-V2-INDUSTRY.md`（Inspector A — 业界调研）
>
> **总结一句话**：Path B 走 Option a（保留 `PluginRecord.Config` 作 host-only ops 字段，从 SDK 删 `Config()`，proto 字段保留但标 reserved）；schema v1 从 8 个候选 marker 中**必抽 4 个**（`x-visibility:secret` + Grafana secret 写入语义、`x-requires-reload`、`x-deprecated`、schema_version 列），**砍 4 个**（`format:password` 与 secure_value_jsonb 双列改用 `x-visibility:secret`，`scope:machine|window` 简化为不抽，markdownDescription 留 V6，`default` 读时填充已经按 §3 行 8 决策抄入）；UI 渲染器**保留手写 + widget map 抽象化**；迁移按 4 阶段（schema_version 列 → SDK manifest → host 服务 → UI 渲染）；本次 PR 边界**只做必抽的 4 marker + Path B 处置 + UI 抽象化**。

---

## 0. 执行摘要（给只想看 5 行的人）

1. **Path B 处置**：选 **Option a** — 保留 `PluginRecord.Config` 作 host-only ops 字段（仅装 `skip_migration`），从 SDK 删 `ctx.Config()`，proto `config = 2` 字段标 `reserved`（不删，避免 wire 不兼容）。
2. **schema v1 必抽 4 marker**：`x-visibility: "frontend"|"backend"|"secret"`、`x-requires-reload: true`、`x-deprecated: "<msg>"`、`schema_version` 列（plugin_settings_schemas 表 + plugin_settings 表镜像）。
3. **secret 写入语义**：抄 Grafana — admin PUT body 只发用户改过的 key，未提及保持原值，空串清除；GET 永不返回 `x-visibility:secret` 字段的明文值，只返回 keys 列表。
4. **UI 渲染器**：**保留手写**（vue-json-schema-form Vue 3 兼容差，造轮子但抽象化为 widget map）。
5. **本次 PR 边界**：必做 = §3 列出的 12 个 step；下次做 = markdownDescription、scope 二元化、orphan 行清理 job；永不做 = 多版本 schema conversion webhook、envelope encryption、org_id 多租户。

---

## 1. 核心三问审计（每个 marker 用同一把尺过）

> **审计标准**（来自 V5-CURATE 的方法论）：
>
> - **半年 API 稳定**：抽出来的 API/字段，半年内是否会再改一次？会 → 砍。
> - **双向复用**：是否有至少 2 个 plugin 或 2 个 host 路径会用？只有 1 个 → 砍。
> - **SDK 替代不了**：plugin 自己用 schema validator + 业务代码能不能搞定？能 → 砍。
> - **不抽的代价**：不抽会怎样？代价能接受 → 砍。
>
> 每个候选 marker 用这 4 把尺审一遍，结论列在最后。

| # | 候选 marker | 半年 API 稳定？ | 双向复用？ | SDK 替代不了？ | 不抽的代价 | **结论** |
|---|---|---|---|---|---|---|
| 1 | `x-visibility: frontend\|backend\|secret`（Backstage） | 稳定（Backstage 用了 5+ 年没改） | 双向（plugin 声明 + host 渲染过滤 + admin UI 渲染） | 替代不了（host 必须知道哪些字段不能返给 frontend） | secret 字段被前端拿到明文 → 安全漏洞 | **必抽** |
| 2 | `format: password` + `writeOnly`（VS Code） | 稳定 | 单向（仅 UI 用） | 可被 `x-visibility:secret` 完全覆盖 | 与 `x-visibility:secret` 重复，二选一 | **砍**（被 #1 覆盖） |
| 3 | 双列存储 `value_json` + `secure_value_jsonb`（Grafana） | 稳定 | 双向（写入路径 + 读取路径） | 替代不了（明文/密文物理隔离） | secret 与 plain 同列，加密失败时整张表暴露 | **可抽**（但本次 PR **砍** — 单列 + `x-visibility:secret` + W5 加密 sealed blob 写回 value_json 即可，详见决策 2.3） |
| 4 | secret 写入语义"未提及保持原值，空串清除"（Grafana） | 稳定 | 双向（admin PUT + plugin 读取） | 替代不了（典型坑：mask 占位符回写覆盖明文） | 用户每次改其他字段都把 secret 用 `***` mask 覆盖成字符串 `"***"` | **必抽** |
| 5 | `deprecationMessage` + `markdownDescription`（VS Code） | 稳定 | `deprecationMessage` 双向；`markdownDescription` 单向 | 替代不了 deprecation；`markdown` 可后置 | 字段升级时无法软迁移 | **`x-deprecated` 必抽**；`markdownDescription` **砍**（V6 再做，前端无 markdown 组件） |
| 6 | `scope: machine\|window` 或 `requires_restart` | `requires_restart` 稳定；`scope` 多值不稳定 | 双向（plugin 声明 + host reconcile + UI 警示） | 替代不了（host 必须知道哪些改动要重启 plugin 进程） | 改 scheduler 间隔后旧 scheduler 还在跑 | **`x-requires-reload: true` 必抽**；`scope` 多值 **砍**（VS Code 6 个 scope 是 IDE 多 window 场景，我们单实例不需要） |
| 7 | schema 版本号 + storage version | 稳定 | 双向（schema 表 + values 表 + plugin 升级流程） | 替代不了（plugin 升级时 host 必须知道老值是哪个 schema 写的） | plugin v0.2 把 string 改 object 后 GetTyped 全报错 | **必抽**（仅 schema_version 列，不做 K8s 多版本 conversion） |
| 8 | `default` 读时填充而非启动期 seed（K8s 反例 / Backstage 正例） | 稳定 | 双向（host SettingService.GetForPlugin + plugin runtime） | 替代不了（schema 改 default 后老值不会自动更新 → 启动期 seed 是反模式） | schema 升级 default 时 backfill 数据库脏活 | **必抽**（实现层，不需要新 API/字段） |

**审计结论**：8 个候选中 **必抽 5 个**（#1, #4, #5 的 deprecated 部分, #6 的 requires-reload 部分, #7, #8）+ **砍 3 个**（#2, #3, #5 的 markdown 部分, #6 的 scope 部分）。

> 注：#3 双列存储原本是"可抽"，但综合"半年 API 稳定 + 不抽代价"两条决定本次 PR 砍掉，改用单列 `value_json` + W5 加密 sealed blob，详见决策 2.3。

---

## 2. 5 个决策

### 决策 1：Path B 处置 — **选 Option a（保留为 host-only ops 字段）**

**结论**：保留 `PluginRecord.Config` 但语义重新定位为 **host-only ops 字段**，从 SDK 删 `ctx.Config()` + `pluginCtx.config` + `cfgCopy` wiring，proto `config = 2` 标 `reserved`（不物理删字段，避免 wire 不兼容）。**不**新建 `ops_plugin_flags` 表。

**理由**（针对 Inspector B §6 Risk 1 + §3 实证）：

1. **`skip_migration` 是 host-side ops 钩子，不是 plugin 配置**。它在 `manager.go:882`（INSPECT §1.3）读取，**早于** `RegisterSchema` 的执行时机（INSPECT §6 Risk 1）。Path A 是 plugin-namespaced，schema 在 plugin 启动时注册，无法表达"启动前的 ops escape hatch"。
2. **新建 `ops_plugin_flags` 表是过度工程**。当前只有 1 个 key（`skip_migration`），新建表 + repository + migration + 启动时读 = 比保留更复杂。重新定位现有字段语义，0 行代码删除即可清理 SDK 表面。
3. **proto 字段不物理删**。INSPECT §1.1 显示 `plugin.proto:32 config = 2` 是 wire-stable 字段。物理删字段 + 改 field number 会破坏外部已编译的 proto descriptor。改为 `reserved 2;` + 注释标 `// reserved: was config map<string,string>, host-only ops, dropped from SDK in V5/W6 SETTINGS-V2`。
4. **删除清单**（SDK 侧，0 plugin 影响 — INSPECT §1.2 证明"0 个 plugin 调 ctx.Config()"）：
   - `plugin-sdk/context.go:35` — 删 `Config() map[string]string` 接口方法
   - `plugin-sdk/runner.go:392-395` — 删 `cfgCopy := make(map[string]string, len(req.GetConfig()))` + 复制循环
   - `plugin-sdk/runner.go:461` — 删 `pluginCtx.config = cfgCopy` 赋值
   - `plugin-sdk/runner.go:665` — 删 `pluginCtx.config` 字段
   - `plugin-sdk/runner.go:675-681` — 删 `pluginCtx.Config()` 方法实现
5. **保留清单**（host 侧）：
   - `backend/internal/plugin/repository.go:40` 的 `PluginRecord.Config map[string]string` — **保留**，加注释 "host-only ops flags; not exposed to plugin code"
   - `backend/internal/plugin/manager.go:882-886` 的 `shouldSkipPluginMigrations` — 保留
   - `backend/internal/plugin/manager.go:509-515` 的 `UpdateConfig` + handler — 保留（curl-only ops）
   - `backend/internal/server/routes/admin.go:563` 的 `PUT /:name/config` — 保留
6. **保留清单**（前端）：
   - `frontend/src/views/admin/PluginsView.vue:296-323` 只读 KV 表 — 保留显示，加注释 "host-only ops config; not plugin settings"

### 决策 2：schema v1 必抽 4 marker（每条带可执行落点）

#### 2.1 `x-visibility: "frontend" | "backend" | "secret"`

**落点 1（manifest Go struct，plugin-sdk 侧）**：

`plugin-sdk/manifest.go` 现有 `SettingsSchema` 结构（INSPECT §2.1 / §2.7 提到 `manifest.go:108, 120-133, 226-242`）保持不动。`x-visibility` 是 **schema vendor extension**（在 JSON Schema 内的字段），不需要 Go struct 字段。Plugin 作者直接在 schema JSON 里写：

```json
{
  "type": "object",
  "properties": {
    "apiKey": {
      "type": "string",
      "x-visibility": "secret"
    },
    "displayName": {
      "type": "string",
      "x-visibility": "frontend"
    },
    "internalCacheTTL": {
      "type": "integer",
      "x-visibility": "backend"
    }
  }
}
```

**落点 2（plugin_settings 表列）**：**不加列**。`x-visibility` 信息从 schema 解析时 in-memory 推导，不冗余存储。

**落点 3（host 服务 — `backend/internal/service/plugin_settings_service.go`）**：

`SchemaInfo` 函数（INSPECT §2.3 line 302-345）增加字段过滤逻辑，在 `Values` 返回前剥离 `x-visibility:secret` 字段值（替换为 `null` 或 sentinel `"***"`），同时增加 `SecretKeys []string` 字段告知前端"这些 key 已配置"（抄 Grafana `secureJsonFields` 模式 — INDUSTRY §1.1）。

新增方法签名：

```go
// 在 plugin_settings_service.go 中新增
func (s *PluginSettingsService) extractSecretKeys(schema *jsonschema.Schema) []string
func (s *PluginSettingsService) maskSecretValues(values map[string]json.RawMessage, secretKeys []string) map[string]json.RawMessage
```

**落点 4（admin handler — `backend/internal/handler/admin/plugin_settings_handler.go`）**：

`Get`（INSPECT §2.5 line 66-86）返回结构增加：

```go
type PluginSettingsResponse struct {
    Schema      json.RawMessage              `json:"schema"`
    Defaults    map[string]json.RawMessage   `json:"defaults"`
    Values      map[string]json.RawMessage   `json:"values"`         // secret keys 在此 map 里值是 null
    SecretKeys  []string                     `json:"secret_keys"`    // 新增：已配置的 secret key 列表（前端知道这些有值，但拿不到）
    UpdatedAt   *time.Time                   `json:"updated_at"`
    SchemaVersion string                     `json:"schema_version"` // 新增（决策 2.4）
}
```

**落点 5（前端 — `frontend/src/components/admin/PluginSettingsForm.vue` + `frontend/src/api/admin/pluginSettings.ts`）**：

`PluginSettingsForm.vue:118-137` 的 `PropDescriptor` TypeScript 类型扩展：

```typescript
interface PropDescriptor {
  key: string
  type: 'boolean' | 'string' | 'number' | 'integer' | 'enum' | 'json' | 'secret'  // 新增 'secret'
  title: string
  description: string
  enumValues?: Array<{ value: unknown; label: string }>
  visibility?: 'frontend' | 'backend' | 'secret'   // 新增
  isConfigured?: boolean                            // 新增（来自后端 secret_keys 列表）
  requiresReload?: boolean                          // 新增（决策 2.3）
  deprecated?: string                               // 新增（决策 2.2）
}
```

`buildPropDescriptor` 函数（约 line 118-137）读 schema 节点的 `x-visibility`，决定是否走 secret 渲染分支（`<input type="password" placeholder="(已配置，留空保持原值)">`）。

**落点 6（schema 合并冲突 fail-fast — 抄 Backstage INDUSTRY §3 行 4）**：

`RegisterSchema`（`plugin_settings_service.go:132-183`）解析 schema 时，对每个 property 校验 `x-visibility` 合法性：

- 值不在 `["frontend", "backend", "secret"]` → 启动失败，返回 `ErrInvalidSchemaVisibility`
- 同字段被父级和子级标不同 visibility（要求 secret 必须不被 frontend 覆盖）→ 启动失败

新增错误：

```go
var ErrInvalidSchemaVisibility = errors.New("plugin settings schema: x-visibility must be one of frontend|backend|secret")
```

#### 2.2 `x-deprecated: "<message>"`（VS Code 风格）

**落点 1（manifest）**：同 `x-visibility`，schema vendor extension，无 Go struct 字段。

**落点 2（DB）**：**不加列**。运行时从 schema 解析。

**落点 3（host 服务）**：`SchemaInfo` 返回的 `Values` map 中**仍包含** deprecated 字段（向后兼容），但前端通过新增的 `SchemaInfo.deprecatedFields` 知道哪些字段要标 deprecated 样式。

新增字段：

```go
type PluginSettingsSchemaInfo struct {
    // ... 现有字段
    DeprecatedFields map[string]string `json:"deprecated_fields"` // key → deprecation message
}
```

**落点 4（前端 PluginSettingsForm.vue）**：

`PropDescriptor.deprecated?: string`（见决策 2.1 落点 5）。渲染层在字段 label 旁显示删除线 + `<el-tag type="warning" size="small">deprecated: <msg></el-tag>`。

**落点 5（行为）**：

- deprecated 字段**仍可读写**（向后兼容）
- 未配置过的 deprecated 字段在 UI 中**隐藏**（VS Code 风格）— `PluginSettingsForm.vue` 渲染时跳过 `deprecated && !isConfigured` 字段
- 已配置过的 deprecated 字段**显示**（提示用户清理）

#### 2.3 `x-requires-reload: true`

**落点 1（manifest）**：schema vendor extension。

**落点 2（DB）**：不加列。

**落点 3（host 服务 — `plugin_settings_service.go`）**：

`SetByKey`（INSPECT §2.3 line 261-298）写入成功后，检查该 key 在 schema 中是否标 `x-requires-reload: true`，若是则推送一个特殊事件 `SettingsChange{Key: key, RequiresReload: true}`。

新增字段：

```go
type SettingsChange struct {
    Plugin         string
    Key            string
    Value          json.RawMessage
    Revision       int64
    RequiresReload bool  // 新增
}
```

**落点 4（host PluginManager — `backend/internal/plugin/manager.go`）**：

PluginManager 订阅 `PluginSettingsService.Subscribe`（已有机制 — INSPECT §2.4 line 115-149），收到 `RequiresReload: true` 事件后调用现有的 reconcile 逻辑重启 plugin 进程。

新增方法（PluginManager 上）：

```go
// manager.go 新增
func (m *PluginManager) reloadPlugin(ctx context.Context, pluginName string, reason string) error
```

**落点 5（前端 PluginSettingsForm.vue）**：

`PropDescriptor.requiresReload?: boolean`。渲染层在字段 description 下面加红色提示：`<div class="reload-hint">{{ t('pluginSettings.requiresReload') }}</div>`，保存按钮 hover tooltip 显示 "保存后将重启 plugin 进程"。

**i18n key 新增**（`frontend/src/i18n/locales/{en,zh}.ts`）：

```typescript
pluginSettings: {
  requiresReload: '修改后将重启此插件 / Saving will restart this plugin'
}
```

#### 2.4 `schema_version` — 必加 DB 列 + manifest 字段

**落点 1（manifest — `plugin-sdk/manifest.go`）**：

`SettingsSchema` 结构新增字段：

```go
// plugin-sdk/manifest.go (在现有 SettingsSchema 结构内添加)
type SettingsSchema struct {
    // ... 现有字段（JSONSchema, Defaults 等）
    Version string `json:"version,omitempty"` // 例 "1.0.0"，缺省视作 "0"
}
```

对应 proto 字段（`plugin-sdk/proto/plugin.proto`，参考 INSPECT §2.5 line 96-146 manifest 区块）新增：

```proto
message SettingsSchemaProto {
  // 现有字段
  string version = N;  // 新增 field number（取下一个可用）
}
```

**落点 2（DB — `backend/migrations/`）**：新增 migration 文件 `103_plugin_settings_v2.sql`：

```sql
-- 103_plugin_settings_v2.sql
ALTER TABLE plugin_settings_schemas
  ADD COLUMN schema_version TEXT NOT NULL DEFAULT '0';

ALTER TABLE plugin_settings
  ADD COLUMN schema_version TEXT NOT NULL DEFAULT '0';

CREATE INDEX idx_plugin_settings_schema_version
  ON plugin_settings (plugin_name, schema_version);
```

**落点 3（host 服务）**：

- `RegisterSchema`（`plugin_settings_service.go:132-183`）upsert 时写入 `schema_version`。
- `SetByKey`（line 261-298）写入新 row 时把当前 `schema_version` 写入 `plugin_settings.schema_version`。
- `seedDefaults`（line 187-205）写入新 row 时同样写当前 schema_version。

**落点 4（gRPC bridge — `settings_extension_server.go`）**：

`Get` 响应（INSPECT §2.4 line 74-98）增加 `SchemaVersion` 字段，让 plugin 端 `SettingsClient.GetTyped` 在反序列化失败时知道 schema 版本不匹配（plugin 自己处理 fallback）。

proto 改动：

```proto
// plugin-sdk/proto/sdk.proto SettingsGetResponse 新增字段
message SettingsGetResponse {
  // ... 现有字段
  string stored_schema_version = N;
  string current_schema_version = N+1;
}
```

**落点 5（plugin SDK — `plugin-sdk/settings.go`）**：

`SettingsClient.GetTyped`（INSPECT §2.1 line 80, 217-226）反序列化失败时，错误包含两个 schema_version，让 plugin 知道是 schema 升级冲突：

```go
var ErrSchemaVersionMismatch = errors.New("plugin settings: stored value uses different schema version")

// GetTyped 反序列化失败时返回的 error 包装：
type SchemaVersionMismatchError struct {
    Key                 string
    StoredSchemaVersion string
    CurrentSchemaVersion string
    UnderlyingErr       error
}
func (e *SchemaVersionMismatchError) Error() string { ... }
func (e *SchemaVersionMismatchError) Is(target error) bool { return target == ErrSchemaVersionMismatch }
```

**落点 6（前端）**：

`PluginSettingsResponse.schema_version`（决策 2.1 落点 4），`PluginsView.vue` 详情面板显示 schema 版本号，便于排障。

#### 2.5 secret 写入语义（抄 Grafana — INDUSTRY §3 行 11）

**落点 1（admin handler — `plugin_settings_handler.go`）**：

`Update`（INSPECT §2.5 line 92-133）的 body 解析逻辑改造：

- body 中**未出现**的 secret key → 保持现值（不操作）
- body 中**显式空字符串** `""` 的 secret key → 删除 row（DELETE FROM plugin_settings WHERE plugin_name=$1 AND key=$2）
- body 中**非空字符串**的 secret key → 正常 upsert

非 secret 字段保持原有"传什么写什么"语义。

**新增 handler 辅助方法签名**：

```go
// plugin_settings_handler.go
func (h *PluginSettingsHandler) applySecretWriteSemantics(
    ctx context.Context,
    pluginName string,
    body map[string]json.RawMessage,
    secretKeys []string,
) (effectiveWrites map[string]json.RawMessage, deletes []string, err error)
```

**落点 2（host 服务）**：新增 `DeleteByKey` 方法：

```go
// plugin_settings_service.go
func (s *PluginSettingsService) DeleteByKey(ctx context.Context, plugin, key string) error
```

实现：`DELETE FROM plugin_settings WHERE plugin_name=$1 AND key=$2`，并 fanout 一个 `SettingsChange{Value: nil}` 事件。

**落点 3（前端）**：

`PluginSettingsForm.vue` 的 secret 字段保存逻辑：

- 用户**不输入** → 不发送该 key
- 用户输入空白后保存 → 显式发空字符串
- 用户输入新值 → 发新值

UI 提示：secret 字段 placeholder 显示 `(已配置，留空保持原值，输空格清除)`。

### 决策 3：UI 渲染器何去何从 — **保留手写 + widget map 抽象化**

**结论**：**保留手写**，但把 `PluginSettingsForm.vue:91-220` 的 130 行 v-if 链重构为 widget map 抽象。

**理由**（针对 INSPECT §6 Risk 3）：

1. **vue-json-schema-form 风险高**：核心维护 `crickford/vue-json-schema-form` Vue 3 兼容性差（INSPECT §6 Risk 3 提到 SETTINGS_API.md 提及但 package.json 0 引用），切到这个库需要测试 Element Plus + el-form 集成 + i18n 集成 + 自定义 widget 注册，引入风险大。
2. **造轮子但抽象化** = 折中方案。用 widget map 模式（一个 `widgets: Record<WidgetType, Component>` map + 一个 `resolveWidget(schema, descriptor): WidgetType` 函数），新增 marker 时只需注册新 widget，不再加 v-if 分支。
3. **本次 PR 抽象的目标**：能加 secret widget、deprecated 装饰器、requires-reload 装饰器，不用改 130 行 v-if。

**落点 1（新增文件 `frontend/src/components/admin/plugin-settings-widgets/`）**：

```
frontend/src/components/admin/plugin-settings-widgets/
├── index.ts                  # widget map + resolveWidget()
├── BooleanWidget.vue         # type=boolean
├── StringWidget.vue          # type=string, !secret
├── SecretWidget.vue          # type=string, x-visibility=secret  (新增)
├── NumberWidget.vue          # type=number|integer
├── EnumWidget.vue            # enum
├── JsonWidget.vue            # type=object|array (raw JSON textarea)
└── decorators/
    ├── DeprecatedDecorator.vue  # 包裹层，加删除线 + tag (新增)
    └── RequiresReloadHint.vue   # 字段下方红字提示 (新增)
```

**落点 2（重构 `PluginSettingsForm.vue`）**：

130 行 v-if 链替换为：

```vue
<template>
  <div v-for="prop in propDescriptors" :key="prop.key">
    <DeprecatedDecorator v-if="prop.deprecated" :message="prop.deprecated">
      <component
        :is="resolveWidget(prop)"
        :prop="prop"
        :model-value="values[prop.key]"
        @update:model-value="onValueChange(prop.key, $event)"
      />
      <RequiresReloadHint v-if="prop.requiresReload" />
    </DeprecatedDecorator>
    <template v-else>
      <component :is="resolveWidget(prop)" ... />
      <RequiresReloadHint v-if="prop.requiresReload" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { resolveWidget } from './plugin-settings-widgets'
// ...
</script>
```

**落点 3（widget map 实现 — `plugin-settings-widgets/index.ts`）**：

```typescript
import BooleanWidget from './BooleanWidget.vue'
import StringWidget from './StringWidget.vue'
import SecretWidget from './SecretWidget.vue'
import NumberWidget from './NumberWidget.vue'
import EnumWidget from './EnumWidget.vue'
import JsonWidget from './JsonWidget.vue'
import type { Component } from 'vue'
import type { PropDescriptor } from '../types'

export const widgets: Record<string, Component> = {
  boolean: BooleanWidget,
  string: StringWidget,
  secret: SecretWidget,
  number: NumberWidget,
  integer: NumberWidget,
  enum: EnumWidget,
  json: JsonWidget,
}

export function resolveWidget(prop: PropDescriptor): Component {
  if (prop.visibility === 'secret') return widgets.secret
  if (prop.enumValues && prop.enumValues.length > 0) return widgets.enum
  return widgets[prop.type] ?? widgets.json
}
```

### 决策 4：迁移路径 — 4 阶段，每阶段独立部署 + 回滚

**阶段 1**：加 `schema_version` 列（DB migration only）

- 改动：新增 `103_plugin_settings_v2.sql`（决策 2.4 落点 2）
- 部署：跑 migration，老 plugin 不动（默认值 `'0'` 让现有行通过）
- 回滚：`ALTER TABLE plugin_settings_schemas DROP COLUMN schema_version; ALTER TABLE plugin_settings DROP COLUMN schema_version;`
- 验收：DB schema 含新列 + 老 plugin 启动不报错（schema_version 写入 `'0'`）

**阶段 2**：改 SDK manifest + proto + plugin SDK

- 改动：决策 1（删 `ctx.Config()`）+ 决策 2.1/2.2/2.3/2.4 的 manifest/proto/SDK 部分
- 部署：plugin-sdk 升版本号（如 `v0.4.0`），重新编译两个 built-in plugin（`hello-world`, `channel-management`）
- 回滚：还原 plugin-sdk 到上一版本，重新编译 plugin
- 验收：plugin 启动 + 成功 RegisterSchema + INSPECT §1.2 grep `ctx.Config()` 仍为 0

**阶段 3**：改 host 服务

- 改动：决策 2.1/2.2/2.3/2.4/2.5 的 host service + handler 部分
- 部署：替换 `sub2api` 二进制
- 回滚：还原二进制到上一版本（DB schema 仍兼容，新列只是不被读）
- 验收：admin GET 返回 `secret_keys`/`deprecated_fields`/`schema_version` 字段；admin PUT 对未提及 secret key 保持原值

**阶段 4**：改 UI

- 改动：决策 2.1/2.2/2.3/2.5 的前端 + 决策 3 的 widget 重构
- 部署：替换前端 dist
- 回滚：还原前端 dist 到上一版本（后端 API 向后兼容）
- 验收：表单显示 secret/deprecated/requires-reload 三种装饰；secret 字段保存语义正确

**部署顺序约束**：阶段 1 → 阶段 2 → 阶段 3 → 阶段 4。**禁止跨阶段合并**。

**实际操作上**：因为我们是单仓 monorepo，4 个阶段会合在 1 个 PR 里，但 commit message 必须按阶段分开（4 个 commit），每个 commit 单独可 build 单独可回滚。

### 决策 5：本次 PR 边界

#### 必做（本次 PR 必须包含）

1. **决策 1 全部**：删 `ctx.Config()`、proto reserved 标注
2. **决策 2.1 全部**：`x-visibility` keyword + secret_keys 返回 + visibility 校验 fail-fast
3. **决策 2.2 全部**：`x-deprecated` keyword + deprecated_fields 返回 + UI 装饰
4. **决策 2.3 全部**：`x-requires-reload` keyword + reload 事件 + UI 提示
5. **决策 2.4 全部**：`schema_version` 列 + manifest/proto/SDK 字段 + version mismatch error
6. **决策 2.5 全部**：secret 写入语义（未提及保持，空串清除）
7. **决策 3 全部**：widget map 重构

#### 下次做（V6 或后续 PR）

- **markdownDescription / markdownEnumDescriptions**：前端无 markdown 组件基建，先延后
- **scope 二元化（instance vs plugin_global）**：当前单实例不需要
- **deprecated 字段的 orphan 行清理 job**：用户主动清理或写一次性脚本即可，不需要 host 自动 prune
- **schema_version migration hook**：让 plugin 注册 schema upgrade migration 函数，host 自动跑老值转新值。本次先把 version 信息暴露给 plugin，让 plugin 自己 fallback。

#### 永不做（明确拒绝）

- **K8s 多版本 schema conversion webhook**（INDUSTRY §1.5 / §3 行 5）：过度设计，我们 plugin schema 是内部消费，不需要多版本并存
- **envelope encryption（DEK + KEK）**（INDUSTRY §3 行 2）：当前没有 KMS 集成需求，V5 的 HKDF + AES-GCM 单层够用
- **org_id / tenant_id 列**（INDUSTRY §3 行 3）：当前是单租户系统
- **双列 `value_jsonb` + `secure_value_jsonb`**（INDUSTRY §3 行 1）：单列 + `x-visibility:secret` + W5 加密 sealed blob 写回 value_json 已经足够；双列在我们规模下增加复杂度且收益有限
- **vue-json-schema-form 库**（决策 3）：Vue 3 兼容差，widget map 抽象化已经够用
- **`format: password` + `writeOnly`**（INDUSTRY §3 行 1，VS Code 风格）：被 `x-visibility:secret` 完全覆盖

---

## 3. Implementer 执行清单（必做项 — 12 个 Step）

> **每步格式**：文件路径 / 函数定位 / 改什么 / 估时（基于"我是熟练 Go+Vue 工程师，但首次接触本仓"标准）

### Step 1：创建 DB migration（30 min）

**文件**：`backend/migrations/103_plugin_settings_v2.sql`（新建）

**内容**：决策 2.4 落点 2 的完整 SQL（4 行 ALTER + 1 行 CREATE INDEX）

**估时**：30 分钟（含本地 docker pg 验证）

### Step 2：proto 改动（45 min）

**文件 1**：`plugin-sdk/proto/plugin.proto`

- line 32：`map<string,string> config = 2;` → 改为 `reserved 2;` + 注释
- 在 `SettingsSchemaProto`（INSPECT §2.5 line 96-146 区域）新增 `string version = N;`

**文件 2**：`plugin-sdk/proto/sdk.proto`

- `SettingsGetResponse` 新增 `string stored_schema_version` + `string current_schema_version`
- `SettingsChange`（推送事件）新增 `bool requires_reload`

**重新生成**：跑 `protoc` 生成 `.pb.go`（项目应有 makefile target 如 `make proto`）

**估时**：45 分钟

### Step 3：删 SDK 的 ctx.Config()（30 min）

**文件**：`plugin-sdk/context.go`、`plugin-sdk/runner.go`

**改动**：见决策 1 删除清单 5 条

**编译验证**：`cd backend && go build ./...` + `cd plugins/hello-world && go build` + `cd plugins/channel-management && go build`

**估时**：30 分钟

### Step 4：SDK manifest 加 SchemaVersion 字段（30 min）

**文件**：`plugin-sdk/manifest.go`

**改动**：

- `SettingsSchema` 结构新增 `Version string \`json:"version,omitempty"\``
- 序列化到 proto 时把 `Version` 写入 `SettingsSchemaProto.version`
- 从 proto 反序列化时读 `version` 写回 `Version`

**估时**：30 分钟

### Step 5：SDK SettingsClient 处理 schema version mismatch（45 min）

**文件**：`plugin-sdk/settings.go`

**改动**：

- 新增 `var ErrSchemaVersionMismatch = errors.New(...)` 和 `SchemaVersionMismatchError` struct（决策 2.4 落点 5）
- 修改 `GetTyped`（line 217-226）：unmarshal 失败时检查响应中的 `stored_schema_version` 与 `current_schema_version`，不一致则返回 `SchemaVersionMismatchError`，一致则返回原 error

**估时**：45 分钟

### Step 6：host 服务 — 增加 schema_version 处理（60 min）

**文件**：`backend/internal/service/plugin_settings_service.go`

**改动**：

1. `RegisterSchema`（line 132-183）：解析 `manifest.SettingsSchema.Version`，upsert 时写入 `plugin_settings_schemas.schema_version`
2. `seedDefaults`（line 187-205）：写入 row 时附带 `schema_version`
3. `SetByKey`（line 261-298）：写入 row 时附带 `schema_version`
4. `SchemaInfo`（line 302-345）：返回值新增 `SchemaVersion` 字段
5. 新增 `DeleteByKey` 方法（决策 2.5 落点 2）

**估时**：60 分钟

### Step 7：host 服务 — 增加 visibility/deprecated/requires_reload 处理（90 min）

**文件**：`backend/internal/service/plugin_settings_service.go`

**改动**：

1. 新增 `extractSecretKeys(schema *jsonschema.Schema) []string` — 遍历 schema 节点，找 `x-visibility:secret`
2. 新增 `extractDeprecatedFields(schema *jsonschema.Schema) map[string]string` — 找 `x-deprecated`
3. 新增 `extractRequiresReloadKeys(schema *jsonschema.Schema) map[string]bool` — 找 `x-requires-reload:true`
4. 新增 `maskSecretValues(values, secretKeys) map[string]json.RawMessage` — 把 secret key 的值替换为 `null`
5. `RegisterSchema` 增加 `validateVisibility(schema)` 调用，违反规则返回 `ErrInvalidSchemaVisibility`（决策 2.1 落点 6）
6. `SchemaInfo` 返回结构增加 `DeprecatedFields` + `SecretKeys` 字段
7. `SetByKey` 写入成功后，若该 key 被标 `x-requires-reload:true`，事件 `RequiresReload` 字段置 `true`

**估时**：90 分钟（schema 遍历逻辑 + 单测）

### Step 8：host PluginManager 订阅 reload 事件（60 min）

**文件**：`backend/internal/plugin/manager.go`

**改动**：

1. 启动时订阅 `PluginSettingsService.Subscribe(ctx, pluginName, "")`（全 namespace）
2. 收到 `SettingsChange{RequiresReload: true}` 事件 → 调用新增 `reloadPlugin(ctx, pluginName, "settings_change:" + key)`
3. 新增方法 `reloadPlugin(ctx, name, reason)`：调用现有的 `stopPlugin` + `spawnAndConnect`（参考 line 851 周边逻辑）

**估时**：60 分钟（含 reload 幂等保护，避免循环重启）

### Step 9：admin handler 改 Get + Update（60 min）

**文件**：`backend/internal/handler/admin/plugin_settings_handler.go`

**改动**：

1. `Get`（line 66-86）返回结构按决策 2.1 落点 4 改造，包含 `secret_keys`、`deprecated_fields`、`schema_version`
2. `Update`（line 92-133）实现决策 2.5 secret 写入语义：
   - 调用 `applySecretWriteSemantics(ctx, plugin, body, secretKeys)`
   - 对 `effectiveWrites` 调 `SetByKey`
   - 对 `deletes` 调 `DeleteByKey`

**估时**：60 分钟

### Step 10：前端 PropDescriptor 类型扩展 + API client（30 min）

**文件 1**：`frontend/src/api/admin/pluginSettings.ts`（line 38-58 周边）

**改动**：response 类型新增 `secret_keys`、`deprecated_fields`、`schema_version`

**文件 2**：`frontend/src/components/admin/plugin-settings-widgets/types.ts`（新建）

**内容**：决策 2.1 落点 5 的 `PropDescriptor` interface

**估时**：30 分钟

### Step 11：前端 widget map 重构（180 min）

**文件 1-7**：决策 3 落点 1 列出的 7 个 `.vue` widget 文件 + 1 个 `index.ts`（共 8 文件，全部新建）

**文件 8**：`frontend/src/components/admin/PluginSettingsForm.vue`

**改动**：

- 130 行 v-if 链替换为决策 3 落点 2 的 `<component :is="resolveWidget(prop)">` 模式
- `buildPropDescriptor` 函数（line 118-137）扩展，从 schema 节点读 `x-visibility`、`x-deprecated`、`x-requires-reload`，从 props 接收 `secretKeys`、`deprecatedFields` 后填到对应 PropDescriptor 字段

**保存按钮逻辑**：

- secret 字段未输入 → 不发送
- 用户清空后保存 → 发空字符串
- 其他字段维持当前逻辑

**i18n**：`frontend/src/i18n/locales/{en,zh}.ts` 新增 `pluginSettings.requiresReload`、`pluginSettings.deprecated`、`pluginSettings.secretPlaceholder` 三个 key

**估时**：180 分钟（widget 拆 7 个文件 + 重构主组件 + i18n + Element Plus 样式调整）

### Step 12：built-in plugin 升级 schema（45 min）

**目标**：让 `channel-management` plugin 演示新 marker，作为冒烟测试。

**文件**：`plugins/channel-management/plugin.go`（INSPECT §2.7 line 41-51, 143, 162-165）

**改动**：

1. 现有 SettingsSchema 中给某个字段加 `x-deprecated`（找一个真的废弃了的字段，或者加一个新字段然后立刻标 deprecated 作为 demo）
2. SettingsSchema 顶层加 `Version: "1.0.0"`
3. 找一个改了要重启 plugin 的字段标 `x-requires-reload: true`（如 `defaultIntervalSec`）

**估时**：45 分钟

---

**总估时**：30 + 45 + 30 + 30 + 45 + 60 + 90 + 60 + 60 + 30 + 180 + 45 = **705 分钟 ≈ 12 小时**（不含调试 + CI 修复）

---

## 4. 验收清单

### 4.1 Grep 必须为零

```bash
# Path B SDK 表面已清除
grep -rn "ctx\.Config()" --include="*.go" plugins/ plugin-sdk/  # 期望 0
grep -rn "pctx\.Config()" --include="*.go" plugins/             # 期望 0
grep -rn "\.config\b" plugin-sdk/runner.go | grep -v "//"       # 期望 0（pluginCtx.config 字段已删）

# proto 字段已 reserved
grep -n "config = 2" plugin-sdk/proto/plugin.proto              # 期望 0
grep -n "reserved 2" plugin-sdk/proto/plugin.proto              # 期望 1
```

### 4.2 Build 必须过

```bash
# Go 全量编译
cd backend && go build ./...

# Plugin 全量编译
cd plugins/hello-world && go build
cd plugins/channel-management && go build

# proto 重新生成
cd plugin-sdk && make proto && git diff --exit-code  # 重新生成与 commit 一致

# 单测
cd backend && make test-unit

# golangci-lint
cd backend && golangci-lint run --timeout=5m

# 前端
cd frontend && pnpm build
cd frontend && pnpm lint
```

### 4.3 手测场景（按优先级排序）

**场景 1（P0）— Path B 删除不影响 plugin 运行**

1. 启动 sub2api，确认两个 built-in plugin（hello-world, channel-management）正常注册 + 启动
2. 在 admin UI 看 plugin 详情，能看到 plugin status = active
3. `curl -X POST /api/v1/admin/plugins/channel-management/config -d '{"config":{"skip_migration":"true"}}'` 仍能写入 PluginRecord.Config
4. 重启 sub2api，确认 `skip_migration` 仍生效（migration 被跳过 + 日志有 `shouldSkipPluginMigrations=true`）

**场景 2（P0）— x-visibility:secret 字段不暴露明文**

1. 在 channel-management plugin schema 加一个 `apiKey` 字段标 `x-visibility:secret`（临时改）
2. admin UI 写入一个值 `mysecret123`
3. `curl /api/v1/admin/plugin-settings/channel-management` 返回 body 的 `values.apiKey === null`，`secret_keys: ["apiKey"]`
4. 前端表单 apiKey 字段渲染为 `<input type="password">`，placeholder = "(已配置，留空保持原值)"
5. plugin 自己通过 `ctx.Settings().GetTyped("apiKey", &out)` 仍能拿到明文 `"mysecret123"`

**场景 3（P0）— secret 写入语义正确**

1. 已有 secret apiKey = `mysecret123`（场景 2）
2. admin UI 改其他字段后保存，**不传** apiKey → DB 中 apiKey 仍是 `mysecret123`
3. admin UI 在 apiKey 输入空格后保存 → DB 中 apiKey row 被 DELETE，`secret_keys: []`
4. admin UI 输入新值 `newvalue` → DB 中 apiKey = `newvalue`

**场景 4（P1）— x-requires-reload 触发 plugin 重启**

1. channel-management plugin schema 把 `defaultIntervalSec` 标 `x-requires-reload:true`
2. 改 `defaultIntervalSec` 从 60 → 120 保存
3. 看日志：`PluginManager: reloadPlugin name=channel-management reason=settings_change:defaultIntervalSec`
4. plugin 进程 restart，新 `defaultIntervalSec=120` 生效

**场景 5（P1）— x-deprecated 字段在 UI 渲染**

1. channel-management plugin schema 给某字段加 `x-deprecated: "use newField instead"`
2. 该字段已配置过 → UI 显示删除线 + warning tag
3. 该字段未配置过 → UI 隐藏

**场景 6（P1）— schema_version 不匹配时 plugin 能 fallback**

1. plugin v1.0.0 写入字段 `count` 类型 `string`，存入值 `"42"`
2. plugin 升级到 v2.0.0，schema 改 `count` 为 `integer`，version="2.0.0"
3. plugin 启动后 `ctx.Settings().GetTyped("count", &n)` 返回 `SchemaVersionMismatchError{Stored:"1.0.0", Current:"2.0.0"}`
4. plugin 用 errors.Is 检测到，fallback 重新解析为 string 再 parse int

**场景 7（P2）— Backstage 风格 fail-fast**

1. plugin schema 写一个非法 `x-visibility: "invalid"`
2. `RegisterSchema` 返回 `ErrInvalidSchemaVisibility`
3. plugin status = ErrorBadSettingsSchema，gateway 摘流

---

## 5. 给 Implementer 的 Sanity Check（开工前必须 verify）

### 5.1 工作树状态

```bash
# 必须在 feature+plugin-grpc 工作树
pwd  # 期望以 .claude/worktrees/feature+plugin-grpc 结尾

# 必须基于已 commit 的 INSPECT/INDUSTRY 报告
git log --oneline -5
# 期望前 3 个 commit 包含：
#   docs(plugin-architecture): SETTINGS-V2 industry research
#   docs(plugin-architecture): SETTINGS-V2 current-state inspection
#   fix(plugin/core): allow settings_extension capability
```

### 5.2 上游依赖 verify（INSPECT §1.2 关键结论）

```bash
# 必须为 0 — 否则 Path B 删除会破坏 plugin
grep -rn "ctx\.Config()" --include="*.go" plugins/

# 必须为 0
grep -rn "pctx\.Config()" --include="*.go" plugins/

# 必须为 0（前端没有写 plugin config 的代码）
grep -rn "/admin/plugins/.*/config" frontend/src/
```

如果以上三个 grep **任何一个返回非空**，**停下来**，找上一轮 Inspector 重新审计 — INSPECT §1.2 的"0 个 plugin 调 ctx.Config()"结论已经失效，决策 1 的删除清单需要修订。

### 5.3 jsonschema 库 vendor extension 支持

```bash
# 必须支持 santhosh-tekuri/jsonschema/v5 解析 x-* keyword
cd backend && grep -rn "x-visibility\|x-deprecated\|x-requires-reload" vendor/ go.sum 2>/dev/null
# 应返回 0（库本身不识别我们的 keyword，但 Draft-07 spec 允许 x-* 作为 extension 通过 unmarshal 不报错）
```

实际上 `jsonschema/v5` 默认会把不认识的 keyword 忽略（不报 unknown keyword 错误），所以 `x-*` 字段会保留在原始 JSON 里。host 服务里通过 `json.Unmarshal` 到 `map[string]any` + 手动遍历提取，**不依赖 jsonschema 库**。

### 5.4 现有 ID 字段是否冲突

```bash
# 检查 SettingsSchemaProto 当前最大 field number
grep -n "= [0-9]" plugin-sdk/proto/plugin.proto | grep "SettingsSchemaProto" -A 20
```

新增 `version` 字段时取**当前最大值 + 1**，不要复用已 deprecated 的 number。

### 5.5 frontend 依赖检查

```bash
# 必须不能引入 vue-json-schema-form（决策 3 明确拒绝）
cd frontend && grep -n "vue-json-schema-form" package.json pnpm-lock.yaml
# 期望返回 0（INSPECT §6 Risk 3 已确认现状是 0）
```

### 5.6 i18n key 命名空间

```bash
# 找到现有 plugin 相关 i18n key 命名空间
cd frontend && grep -rn "pluginSettings" src/i18n/
# 确认现有 key 在 pluginSettings.* 命名空间下，新增 key 沿用此命名
```

### 5.7 channel-management plugin 真的有适合做 demo 的字段

```bash
# 找一个真的修改后会失效的字段做 x-requires-reload demo
grep -n "defaultIntervalSec\|interval\|cron" plugins/channel-management/plugin.go
```

如果没找到合适字段，Step 12 改为加一个新字段（不影响业务，仅做 schema marker demo），不要硬给已有字段加 reload。

### 5.8 Migration runner 兼容性

```bash
# 确认 103_plugin_settings_v2.sql 会被 backend 启动时自动 apply
grep -rn "schema_migrations\|migrate_up\|MigrateUp" backend/internal/migrations/ backend/cmd/
```

确认 backend 启动时会扫 `backend/migrations/*.sql` 并按 filename 数字顺序 apply。如果不是这种模式（例如需要在某个 list 里手动注册），Step 1 还需要补充注册步骤。

---

## 附录：关键 file:line 索引（给 Implementer 抄）

### 决策 1 改动点

| 文件 | 行号 | 改动 |
|---|---|---|
| `plugin-sdk/context.go` | 35 | 删 `Config() map[string]string` |
| `plugin-sdk/runner.go` | 392-395 | 删 `cfgCopy` 复制 |
| `plugin-sdk/runner.go` | 461 | 删 `pluginCtx.config = cfgCopy` |
| `plugin-sdk/runner.go` | 665 | 删 `pluginCtx.config` 字段 |
| `plugin-sdk/runner.go` | 675-681 | 删 `pluginCtx.Config()` 实现 |
| `plugin-sdk/proto/plugin.proto` | 32 | `map<string,string> config = 2;` → `reserved 2;` |

### 决策 2.1-2.5 改动点

| 文件 | 行号 | 改动 |
|---|---|---|
| `backend/migrations/103_plugin_settings_v2.sql` | 新建 | 加 schema_version 列 |
| `plugin-sdk/manifest.go` | 108-133 周边 | SettingsSchema 加 Version 字段 |
| `plugin-sdk/proto/plugin.proto` | 96-146 区域 | SettingsSchemaProto 加 version 字段 |
| `plugin-sdk/proto/sdk.proto` | 417-472 区域 | SettingsGetResponse + SettingsChange 加新字段 |
| `plugin-sdk/settings.go` | 217-226 | GetTyped 处理 SchemaVersionMismatchError |
| `backend/internal/service/plugin_settings_service.go` | 132-345 | RegisterSchema/SchemaInfo/SetByKey/seedDefaults 全改 + 新增 DeleteByKey + 新增 4 个 extract* 方法 |
| `backend/internal/plugin/settings_extension_server.go` | 74-149 | Get 响应 + Watch 推送加新字段 |
| `backend/internal/plugin/manager.go` | 851 周边 | 订阅 reload 事件 + 新增 reloadPlugin 方法 |
| `backend/internal/handler/admin/plugin_settings_handler.go` | 66-133 | Get + Update 响应/请求格式改造 |
| `frontend/src/api/admin/pluginSettings.ts` | 38-58 | response 类型扩展 |
| `frontend/src/components/admin/PluginSettingsForm.vue` | 91-220 | 用 widget map 替换 v-if 链 |
| `frontend/src/components/admin/plugin-settings-widgets/` | 新建 | 7 个 widget + index.ts |
| `frontend/src/i18n/locales/en.ts` | 现有 pluginSettings 区块 | 新增 3 个 key |
| `frontend/src/i18n/locales/zh.ts` | 现有 pluginSettings 区块 | 新增 3 个 key |
| `plugins/channel-management/plugin.go` | 41-51, 143 | 加 schema marker demo |
