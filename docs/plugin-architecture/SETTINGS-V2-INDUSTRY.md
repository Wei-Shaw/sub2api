# SETTINGS-V2 Industry Research (Inspector A)

> 角色：read-only Inspector，外部参照库。Curator 看完做"砍/留/改"决策。
> 本文不引用我们自己的代码，所有结论都给到一手 URL（Grafana 直接拉了 plugin_setting 迁移源码核对）。
> 调研日期：2026-04-27。
>
> **使用方式**：报告里每个 finding 末尾都有"对我们意味着什么"的具体抄/拒决策点，Curator 直接拍即可。

---

## 0. TL;DR — 7 个直接抄、3 个直接拒、2 个待决

### 直接抄（建议）

1. **Grafana `plugin_setting` 双列拆分（json_data + secure_json_data）+ org_id 多租户索引** → 我们 `plugin_settings` 表加 `secure_value_json` 列 + `(plugin_name, key)` 唯一索引（当前我们没有 org_id 概念，留作 V6 hook）。详见 §1.1 / §3 行 1。
2. **VS Code `scope: machine|window|resource|application` + `ignoreSync`** → 我们改用更窄的二元 `scope: "instance"|"plugin_global"`（暂不需要 sync 概念，但保留扩展位）。详见 §1.2 / §3 行 6。
3. **Backstage `visibility: frontend|backend|secret` 自定义 keyword + meta-schema `$id: backstage.io/schema/config-v1`** → 我们直接抄 `x-visibility` keyword 与 `$schema` 钉版本号机制（`sub2api.io/schema/plugin-settings-v1`）。详见 §1.3 / §3 行 4。
4. **Backstage 合并冲突规则：同一字段 `frontend` + `secret` 双标 = 启动失败** → 我们应用同一规则到 `x-visibility` 校验。详见 §1.3 / §3 行 4。
5. **K8s CRD `served / storage / deprecated / deprecationWarning` per-version 标记** → 我们 schema 加 `x-deprecated: "use foo instead"` 字段级标记（不做完整的多版本 conversion webhook，过度设计）。详见 §1.5 / §3 行 5。
6. **K8s CRD `default` 在 OpenAPI 内联 + 启动时注入到缺省值** → 我们让 plugin 在 schema 内 inline `default`，host 在写入 `plugin_settings` 时只存"用户实际改过的值"，未配置项启动时即时填默认（避免迁移 schema 时洗数据）。详见 §1.5 / §3 行 8。
7. **VS Code `markdownDescription` + `enumDescriptions` + `markdownDeprecationMessage` 富文本** → 我们 schema 至少支持 `description`（plain）与 `enumDescriptions`，markdown 留作 V2 增强。详见 §1.2 / §3 行 9。

### 直接拒（避免）

1. **WordPress 散表 + filter hook 模式**：每个字段独立 `register_setting()` 调用，UI 通过 `do_settings_sections()` 反射拼接。**做不了 schema-driven 表单**，所有 UI 渲染靠 PHP 侧 callback function。我们如果走这条路就重蹈"前端无法生成统一 settings 页"的覆辙。详见 §1.6。
2. **Strapi v4 `validator: (config) => { throw }` 函数式校验**：把校验逻辑写成 JS 函数，前端拿不到 schema 描述，无法生成表单。**与 schema-driven UI 直接冲突**。我们 plugin 的校验必须是 schema 而不是 callback。详见 §1.4。
3. **Terraform `schema.Resource{ Type: TypeString, Sensitive: true }` Go 结构体声明**：完全跳过 JSON Schema，靠反射生成 HCL 文档。**前端不可移植**（Terraform 没有"网页 UI"需求）。我们如果学这个会把 schema 锁死在 Go 进程内，admin 前端没法纯前端渲染表单。详见 §1.7。

### 待决问题（留给 Curator）

1. **是否引入 envelope encryption（DEK + KEK 两层）？** Grafana v9.0+ 走两层是因为它要支持 KMS 集成（Google Cloud KMS / AWS KMS）。我们当前 `secret_encryption capability` 只用 HKDF + AES-GCM 单层，**够不够**？参考 §1.1 "Encryption Mechanism" 段落。建议：V5 单层 OK，V6 再考虑 envelope。
2. **schema 演进策略选哪种？** K8s 走"多版本并存 + conversion webhook"；Backstage 走"meta-schema 钉 v1 url，未来 v2 完全替换"；VS Code 走"`deprecationMessage` 软迁移，旧 key 永远兼容"。三选一。建议：抄 VS Code（最简单，与我们的"plugin 自包含 schema"哲学最匹配），具体见 §3 行 5。

---

## 1. 调研基线

### 1.1 Grafana plugins

**一手源**：
- 文档：https://grafana.com/developers/plugin-tools/reference/plugin-json
- 文档：https://grafana.com/developers/plugin-tools/how-to-guides/data-source-plugins/add-authentication-for-data-source-plugins
- 文档：https://grafana.com/developers/plugin-tools/how-to-guides/app-plugins/add-authentication-for-app-plugins
- 文档：https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-database-encryption/
- 源码（直接拉到内容）：https://github.com/grafana/grafana/blob/main/pkg/services/sqlstore/migrations/plugin_setting.go

**Storage layout**：单张全局表 `plugin_setting`，每行 = (org_id, plugin_id) 元组。**双列拆分**：

```go
// pkg/services/sqlstore/migrations/plugin_setting.go (实际抓到的源码)
pluginSettingTable := Table{
    Name: "plugin_setting",
    Columns: []*Column{
        {Name: "id",                Type: DB_BigInt,   IsPrimaryKey: true, IsAutoIncrement: true},
        {Name: "org_id",            Type: DB_BigInt,   Nullable: true},
        {Name: "plugin_id",         Type: DB_NVarchar, Length: 190, Nullable: false},
        {Name: "enabled",           Type: DB_Bool,     Nullable: false},
        {Name: "pinned",            Type: DB_Bool,     Nullable: false},
        {Name: "json_data",         Type: DB_Text,     Nullable: true},  // 明文 KV，用户配置
        {Name: "secure_json_data",  Type: DB_Text,     Nullable: true},  // **加密后的 KV blob**
        {Name: "created",           Type: DB_DateTime, Nullable: false},
        {Name: "updated",           Type: DB_DateTime, Nullable: false},
    },
    Indices: []*Index{
        {Cols: []string{"org_id", "plugin_id"}, Type: UniqueIndex},
    },
}
// 后续 migration 加了 plugin_version 列（NVarchar 50, nullable）
```

**Schema 声明位置**：`plugin.json`（manifest），但**注意**：plugin.json **不**声明 settings 字段的 schema。settings 表单是 plugin 自己写的 React 组件 `ConfigEditor`（plugin 自带 admin UI 实现）。整个 admin UI 不是 schema-driven，是 plugin 自己出 React 代码。

**Schema 格式**：N/A（无 schema-driven 表单）。

**Schema 版本演进**：plugin.json 有 `info.version` (SemVer) + `dependencies.grafanaDependency`。**没有** schema 字段级 deprecation。Plugin 自己负责在 ConfigEditor 里读旧字段、迁移到新字段。

**Secret 字段处理**（最有价值的一段）：

- **前端永远拿不到明文**：`secureJsonData` 在 plugin meta 里只暴露 `secureJsonFields`（key 列表，告诉 UI "这个字段已配置"），值永远是空字符串。
- **写入 = 完全覆盖**：POST `/api/plugins/<pluginId>/settings`，"如果你设置 secureJsonData 中的某个 key，应该只发送用户修改过的那些 key — 任何值（包括空字符串）都会覆盖现有值"。
- **解密只在 backend**：Go plugin 通过 `req.PluginContext.DataSourceInstanceSettings.DecryptedSecureJSONData["apiKey"]` 拿明文。
- **Encryption mechanism**（Grafana v9.0+）：envelope encryption。DEKs（data encryption keys）存 `data_keys` 表，每个 DEK 用 KEK（key encryption key，root secret 或 KMS 派生）加密。secret 本身存 `secrets` 表。算法：默认 AES-CFB，企业版可选 AES-GCM。

**启动 vs 运行期**：plugin 设置变更走 admin POST 接口，立即生效（plugin 进程重新读取 InstanceSettings）。**没有"重启才能生效"的字段标记**——Grafana 假设所有 settings 都热生效。

**默认值机制**：plugin.json 不放 default。default 由 plugin 自己的 ConfigEditor React 组件 hardcode（`useState(defaultConfig)`）。

**Admin UI 渲染**：**不自动生成**。plugin 必须实现 `ConfigEditor` (React) + `QueryEditor` (React) 组件，Grafana 框架只负责挂载/卸载，表单内容完全是 plugin 自定义。

**多租户**：`org_id` 列内置。Grafana 是 multi-tenant 平台，每个 org 自己一份 plugin_setting 行。`(org_id, plugin_id)` 唯一索引。

---

### 1.2 VS Code extensions

**一手源**：
- 文档：https://code.visualstudio.com/api/references/contribution-points
- 源码：https://github.com/microsoft/vscode/blob/main/src/vs/workbench/api/common/configurationExtensionPoint.ts
- 源码：https://github.com/microsoft/vscode/blob/main/src/vs/platform/configuration/common/configurationRegistry.ts
- SecretStorage API：https://vscode-api.js.org/interfaces/vscode.SecretStorage.html
- 安全分析：https://cycode.com/blog/exposing-vscode-secrets/

**Storage layout**：分层文件系统，**没有**单张数据库表。
- 普通设置：`settings.json`（user / workspace / folder 三个层级，按 scope 决定写哪一层）
- 秘密：单独走 `SecretStorage` API → SQLite `state.vscdb` 的 `ItemTable` 表（key 形如 `secret://<extensionId>::<key>`），value 用 OS keyring（macOS Keychain / Windows Credential Manager / Linux Keyring）封的 Electron `safeStorage` 加密。

**Schema 声明位置**：`package.json` → `contributes.configuration`（manifest 内联）。

**Schema 格式**：JSON Schema 子集。支持 `default / minimum / maximum / maxLength / minLength / pattern / patternErrorMessage / format / maxItems / minItems / enum / enumDescriptions / markdownEnumDescriptions / enumItemLabels`，但**明确不支持** `$ref` 和 `definitions`（"configuration schemas must be self-contained"）。

**Schema 版本演进**：**没有显式 version 字段**。靠：
- `deprecationMessage` / `markdownDeprecationMessage`：UI 出删除线 + 警告 + 隐藏未配置过的字段
- 旧字段永远兼容（只是不推荐）
- 推荐字段用 `` `#editor.colorDecorators#` `` 链接语法引导用户跳转

```json
"json.colorDecorators.enable": {
  "type": "boolean",
  "markdownDeprecationMessage": "**Deprecated**: Please use `#editor.colorDecorators#` instead."
}
```

**Secret 字段处理**：**完全分离的 SecretStorage API**，不走 settings.json。
- API：`context.secrets.store(key, value)` / `get(key)` / `delete(key)` + onDidChange 事件
- 加密：Electron `safeStorage.encryptString` → AES-128-CBC（IV 硬编码 16 个空格 — 已知缺陷，见 cycode 文章）+ OS keyring 派生密钥
- 重要：**schema 里不出现 secret 字段**。secret 是 runtime API，跟 contributes.configuration 完全解耦。

**启动 vs 运行期**：靠 `scope` 字段隐式表达：
- `application` / `machine` / `machine-overridable`：通常含义是"安装路径/远程 host"等需要重启 window 的
- `window` (default)：需要重新 reload window 才生效（VS Code 有专用 prompt "需要重新加载窗口"）
- `resource` / `language-overridable`：实时生效

VS Code 没有显式 `requiresRestart: true` 标记，**靠扩展自己监听 `vscode.workspace.onDidChangeConfiguration` 决定怎么响应**（弹 reload 提示 / 直接生效）。

**默认值机制**：schema 内 inline `default`。另外有 `contributes.configurationDefaults` 让 extension 覆盖**其他** extension 的 default（小心使用）。**重要约束**：`application` 和 `machine` scope 的 default 不允许被 `configurationDefaults` 覆盖。

**Admin UI 渲染**：**完全自动生成**。VS Code Settings UI 直接读取 manifest schema 渲染表单（boolean → checkbox，enum → dropdown，array of primitives → editable list，复杂 object → "Edit in settings.json" 链接）。**这是 schema-driven UI 的标杆实现。**

**多租户**：N/A（VS Code 是单用户工具）。但 scope 设计有等价物：`user / workspace / folder` 三层级 + `language overrides`。

---

### 1.3 Backstage plugins

**一手源**：
- 文档：https://backstage.io/docs/conf/defining/
- 文档：https://backstage.io/docs/conf/writing/
- meta-schema：https://backstage.io/schema/config-v1（实际拉过，确认是 JSON Schema Draft-07 superset）

**Storage layout**：**没有数据库**。Backstage 配置全部走文件：`app-config.yaml` + `app-config.local.yaml` + `app-config.production.yaml`，启动时合并。
- 合并规则：`primitive 完全替换`、`array 完全替换`、`object 深合并`，命令行 `--config` 顺序决定优先级（后者覆盖前者）。
- 环境变量覆盖：`APP_CONFIG_app_baseUrl=...`（最高优先级）。

**Schema 声明位置**：每个 npm package 的 `package.json` 顶层 `configSchema` 字段，**值要么是内联 JSON Schema，要么是相对路径指向 `.json` 或 `.d.ts` 文件**。Backstage CLI 启动时遍历所有 `@backstage/*` 依赖 + 任何带 `configSchema` 字段的 package，把它们 stitch 成一份完整 schema。

**Schema 格式**：JSON Schema Draft-07 的 **superset**，加自定义关键字 `visibility` 和 `deepVisibility`。Meta-schema URL `https://backstage.io/schema/config-v1` —— 注意 `-v1` 后缀，明示版本号。

```typescript
// 实际示例 (TS .d.ts 形式)
export interface Config {
  app: {
    /**
     * @visibility frontend  // 仅这个字段对前端可见
     */
    baseUrl: string;
    /**
     * @deepVisibility frontend  // 自身 + 所有后代字段对前端可见
     */
    customSchedule: HumanDuration;
  };
  backend: {
    /**
     * @deepVisibility secret  // 自身 + 所有后代标 secret
     */
    customCredentials: {
      password: string;
    };
  };
}
```

**Schema 版本演进**：靠 meta-schema URL 钉版本（`config-v1`）。**当前没有 v2** —— Backstage 实际上用 "永远向后兼容 v1，重大变更走 plugin 自己的 changelog" 的策略。**没有自动 conversion webhook 概念**。

**Secret 字段处理**：
- schema 标记：`@visibility secret` 或 `@deepVisibility secret`
- 含义："运行期 scope 同 backend，但在某些场景受到更严格保护"（文档原话）
- **冲突保护**：不同 plugin 对同一字段同时声明 `frontend` 和 `secret` → schema 合并时**报错 fail startup**
- 实际加密落地：Backstage **不内置加密 store**。文档明确写"敏感信息（如私钥）不应写死在配置里，建议用 Vault 等外部 secret store"。Backstage 提供 `$env / $file / $include` 引用机制把外部 secret 注入：

```yaml
backend:
  mySecretKey:
    $env: MY_SECRET    # 从环境变量读
  certificate:
    $file: ./cert.pem  # 从文件读

# 启动时一次性 resolve，运行时不会重新读
```

**启动 vs 运行期**：**全部启动时 resolve**。文档明确："All includes are loaded at startup, so changing the contents of files or environment variables will not be reflected at runtime."

**默认值机制**：schema 内 inline `default`（JSON Schema Draft-07 标准）。Meta-schema 自身也用 default（如 `properties` 默认 `{}`，`stringArray` 默认 `[]`）。

**Admin UI 渲染**：**N/A**。Backstage 配置走文件 + git，**没有 admin "改配置" UI**。运维改 yaml + 重启。

**多租户**：N/A，Backstage 是单租户。

---

### 1.4 Strapi v4 plugins

**一手源**：
- 文档：https://docs-v4.strapi.io/dev-docs/configurations/plugins
- 文档：https://docs-v4.strapi.io/dev-docs/api/plugins/server-api
- 文档：https://docs-v4.strapi.io/dev-docs/api/plugins/admin-panel-api
- 文档：https://docs.strapi.io/cms/plugins-development/server-configuration

**Storage layout**：**两套并存**：
- 文件：`config/plugins.js` — 启动时静态加载，存 plugin 启用/禁用 + 运维参数
- 数据库：admin UI 通过 Settings 区块允许的设置写到数据库（content-type 路径）—— **每个 plugin 自己定义自己的 entity，没有统一 settings 表**

**Schema 声明位置**：plugin server 端代码 `./server/index.js` 的 `config` 字段：

```javascript
// strapi-plugin/server/index.js
module.exports = () => ({
  config: {
    default: ({ env }) => ({ optionA: true }),
    validator: (config) => {
      if (typeof config.optionA !== 'boolean') {
        throw new Error('optionA has to be a boolean');
      }
    },
  },
});
```

**Schema 格式**：**没有 schema**。`default` 是 JS 函数返回 object，`validator` 是 JS 函数 throw error。**纯命令式**。

**Schema 版本演进**：N/A，plugin 自己 changelog 处理。

**Secret 字段处理**：**官方建议"不要在 plugin config 里存 secret"**（原话："Don't store secrets in plugin config"）。建议走 `env('MY_VAR')` 拿环境变量。**没有内置加密 store**。

**启动 vs 运行期**：`config/plugins.js` 启动时加载 + validator 运行 + plugin 加载，全程在 boot 阶段。运行时不可改。admin Settings UI 写入数据库的部分热生效。

**默认值机制**：`default: ({ env }) => ({...})` 函数返回。**与 user config 深合并** —— Strapi 文档原话：deep-merge defaults with user's config from `config/plugins.js`。

**Admin UI 渲染**：**完全 plugin 自定义**。plugin 用 `app.addMenuLink({...})` + `app.registerPlugin({...})` 注册自己的 React 页面。**必须用 Strapi Design System** (`@strapi/design-system/Button`)。**没有 schema-driven 自动渲染**。

**多租户**：N/A。

---

### 1.5 Kubernetes Operator CRDs

**一手源**：
- https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/
- https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/
- https://kubernetes.io/blog/2023/04/24/openapi-v3-field-validation-ga/

**Storage layout**：每个 CRD 一个 etcd "table"，由 K8s API server 透明管理。CRD 自身是一个 cluster-scope 资源，里面声明 schema。

**Schema 声明位置**：CRD YAML 的 `spec.versions[*].schema.openAPIV3Schema` 字段。每个 version 一份独立 schema。

**Schema 格式**：**OpenAPI v3.0 schema 的 "structural" 子集**。约束：
- 必须是 structural（每个字段类型明确，不允许 `oneOf` 跨字段）
- 默认情况下未声明字段 **会被 prune**（删除）
- 想保留未知字段：在子树加 `x-kubernetes-preserve-unknown-fields: true`
- 不允许：`default`（旧版有限制，新版放宽）/ `nullable` / `discriminator` / `readOnly` / `writeOnly` / `xml` / `deprecated` / `$ref`、`uniqueItems: true`
- 自定义 keyword：`x-kubernetes-validations`（CEL 校验，1.27 GA）/ `x-kubernetes-preserve-unknown-fields` / `x-kubernetes-int-or-string`

**Schema 版本演进**（**最成熟的部分**）：

```yaml
spec:
  versions:
    - name: v1beta1
      served: true       # 是否对外暴露
      storage: false     # 是否是 etcd 持久化版本（全局只能一个 true）
      deprecated: true   # 标记此版本废弃
      deprecationWarning: "use v1 instead"  # API response 加 Warning header
      schema: ...
    - name: v1
      served: true
      storage: true      # 当前唯一持久化版本
      schema: ...
  conversion:
    strategy: Webhook    # 多 schema 不兼容时，调外部 webhook 转换
    webhook:
      clientConfig:
        service: { namespace: default, name: example-conv-webhook, path: /convert }
        caBundle: <base64>
      conversionReviewVersions: ["v1", "v1beta1"]
```

迁移工作流：(1) 加新版本 served+不 storage → (2) 切换 storage flag → (3) 重写所有现有对象到新 storage 版本 → (4) 标 deprecated → (5) 停止 served → (6) 移除版本。

Conversion webhook 必须满足：**幂等 + 往返一致 (`v1 → v2 → v1` 必须等于原对象) + 顺序保持 + 只能改 apiVersion/labels/annotations**（不能动 name/namespace/UID）。

**Secret 字段处理**：CRD schema 不区分 secret，**敏感字段统一走独立的 `Secret` 资源** + `valueFrom.secretKeyRef` 引用。`Secret` 资源在 etcd 里是 base64 编码（**非加密**），需要开启 [encryption-at-rest](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/) 才有加密。

**启动 vs 运行期**：CRD 改了之后立即生效（`served: false` 立即下线）。controller 自己监听 schema 变更。

**默认值机制**：`schema.openAPIV3Schema.properties.foo.default: bar` —— **API server 在 admission 时填默认值并持久化**（不是读时填）。OpenAPI v3 GA 之后，default 才在 OpenAPI 文档里完整表达（v2 会丢失 default）。

**Admin UI 渲染**：N/A（K8s 没有 admin UI；客户端 kubectl 拿 schema 做补全，第三方 dashboard 拿 OpenAPI 做表单 e.g. Lens、Rancher）。

**多租户**：靠 namespace 隔离 + RBAC + ResourceQuota。

---

### 1.6 WordPress Settings API（**反面教材**）

**一手源**：
- https://developer.wordpress.org/plugins/settings/settings-api/
- https://developer.wordpress.org/reference/functions/register_setting/
- https://codex.wordpress.org/Settings_API

**Storage layout**：单张全局 `wp_options` 表（key/value），所有 plugin 共享。每个 plugin 自己挑一个 unique key 当命名空间（约定 `pluginname_options` 然后塞 array），但 **WordPress 不强制**任何隔离。

**Schema 声明位置**：**没有 schema 声明位置**。`register_setting($option_group, $option_name, $args)` 在运行期注册，`$args` 里有 `type / description / sanitize_callback / default / show_in_rest`。

**Schema 格式**：`show_in_rest` 接受一个 JSON schema 对象，**但**：
- 文档原话："The schema is only used by the REST API to define the schema associated with the setting and to implement sanitization over the REST API. **It has no effect for the workings of the Admin pages or the way the Setting is handled by the Options API.**"
- 也就是 admin UI 与 schema 完全脱钩

**Admin UI 渲染**：靠 `add_settings_section()` + `add_settings_field()` 命令式注册 + 每个字段一个 PHP `callback` 渲染 HTML：

```php
add_settings_field(
  'my_field_id',
  'My Field Title',
  'my_field_callback',  // <-- PHP 函数自己 echo HTML
  'my_settings_page',
  'my_section_id'
);

function my_field_callback() {
  $options = get_option('my_options');
  echo "<input type='text' name='my_options[field]' value='{$options['field']}' />";
}
```

**这就是为什么 WordPress 做不了 schema-driven UI**：UI 长什么样完全取决于 PHP callback，外部工具拿不到结构化 schema。社区有第三方 wrapper（如 iconicwp/WordPress-Settings-Framework）用大数组 + 反射模拟 schema-driven，但**不是官方机制**。

**Secret 字段处理**：**没有任何机制**。secret 与普通 option 同存 `wp_options.option_value` 列（plain text）。安全靠数据库本身或第三方 plugin（e.g., WP-CLI 加密插件）。

**对我们的反面意义**：这就是 V5-CURATE Q2 选 JSON Schema 而不是 "用 SettingService 硬编码 struct + handler 写每个字段渲染" 的根本理由——后者就是 WordPress 模式。

---

### 1.7 Hashicorp Terraform provider schema（**Go 原生但不可移植**）

**一手源**：
- https://pkg.go.dev/github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema
- https://developer.hashicorp.com/terraform/plugin/sdkv2/schemas/schema-behaviors
- https://github.com/hashicorp/terraform-plugin-sdk/blob/main/helper/schema/schema.go

**Schema 声明位置**：Go 代码内 `*schema.Resource` 结构体。

```go
return &schema.Resource{
    Schema: map[string]*schema.Schema{
        "api_key": {
            Type:         schema.TypeString,
            Required:     true,
            Sensitive:    true,                         // 标 secret
            DefaultFunc:  schema.EnvDefaultFunc("YOURSERVICE_API_KEY", nil),
            ValidateFunc: validation.IsURLWithHTTPorHTTPS, // Go 函数
        },
        "amount": {
            Type:     schema.TypeInt,
            Required: true,
            ValidateFunc: func(val any, key string) (warns []string, errs []error) {
                v := val.(int)
                if v < 0 || v > 10 {
                    errs = append(errs, fmt.Errorf("%q must be between 0 and 10", key))
                }
                return
            },
        },
    },
}
```

**Schema 格式**：**Go 结构体**，**完全不是 JSON Schema**。框架用反射生成 HCL 文档 + Terraform Cloud 的 Web UI（也是 Go 框架渲染）。

**Secret 字段处理**：`Sensitive: true` 标记。Terraform 不打印此字段值到日志/`terraform plan` 输出。**但**：(1) 不支持条件 sensitivity，(2) 不能从配置传给 provider，(3) 引用此字段的下游字段如果不也标 sensitive 则会泄露。

**对我们的反面意义**：Terraform 这种"Go 结构体即 schema"模式只适合 **provider 与 client 都用同一个 Go 二进制**的场景（Terraform plugin 是 RPC 起的 Go 进程）。我们 admin UI 是 Vue 前端，**不可能拿 Go 反射的 schema 渲染表单**。所以必须走 JSON Schema（或 OpenAPI）这种平台无关的描述。

---

## 2. 横向对比表

| 维度 | Grafana | VS Code | Backstage | Strapi v4 | K8s CRD | WordPress | Terraform |
|------|---------|---------|-----------|-----------|---------|-----------|-----------|
| **Storage layout** | 单表 `plugin_setting` + (org_id, plugin_id) 唯一键 + json/secure_json 双列 | 文件分层 (settings.json) + SQLite SecretStorage | 文件 (yaml) + 启动 merge | 文件 `plugins.js` + 数据库（plugin 自有 entity） | etcd（API server 管理）| 单表 `wp_options` (key/value 全局共享) | N/A（运行期内存）|
| **Schema 位置** | plugin.json（**不含** settings schema） | package.json `contributes.configuration` 内联 | package.json `configSchema` 字段（内联或 .d.ts/.json 文件） | 代码 `./server/index.js` 的 `config` 字段 | CRD YAML `spec.versions[*].schema.openAPIV3Schema` | 运行期 `register_setting()` 调用 | Go 代码 `*schema.Resource` |
| **Schema 格式** | **无**（plugin 自己写 React `ConfigEditor`）| JSON Schema 子集（无 `$ref`/`definitions`）| JSON Schema Draft-07 superset（加 `visibility`/`deepVisibility`）| 命令式 JS `default` + `validator` 函数 | OpenAPI v3 structural schema + `x-kubernetes-*` 自定义 keyword | JSON Schema（仅 REST API 用，UI 不用）| Go struct（反射）|
| **Schema 版本演进** | plugin SemVer + 自管 | `deprecationMessage` + `markdownDeprecationMessage`（旧字段永久兼容）| meta-schema URL 钉 v1（无 v2 路径）| plugin 自管 | **完整方案**：`served`/`storage`/`deprecated`/`deprecationWarning` + Webhook conversion | 无 | 无 |
| **Secret 处理** | `secure_json_data` 列（envelope 加密 AES-CFB / KMS）+ 前端永远 mask | 独立 `SecretStorage` API → SQLite + OS keyring | `@visibility secret` schema 标记，存储建议外接 Vault；`$env`/`$file`/`$include` 引用 | 官方建议不存 secret，走 `env()` | 独立 `Secret` 资源 + `secretKeyRef` 引用（etcd encryption-at-rest 可选）| 无机制（与普通 option 同存明文）| `Sensitive: true` 标记（仅日志 mask，无加密）|
| **启动 vs 运行期** | 全部热生效（plugin context 重读）| 隐式：`scope` + 扩展自己监听 onDidChangeConfiguration（部分 reload window）| **全部启动时 resolve**，运行期不重新读 | `plugins.js` 启动时；UI 写入热生效 | 立即生效 | 立即生效 | provider 启动时绑定 |
| **默认值机制** | plugin React 组件 hardcode `useState(defaults)` | schema 内 `default`；`configurationDefaults` 跨 plugin 覆盖（限制 application/machine 不可被覆盖）| schema 内 `default`（Draft-07 标准）| `default: ({env}) => ({...})` 函数返回 + 深合并用户 config | schema 内 `default`，admission 时持久化注入 | `register_setting($args['default'])` | `Default: "x"` 或 `DefaultFunc: env(...)` |
| **Admin UI 渲染** | 完全 plugin 自定义 React | **完全自动生成**（schema-driven 标杆）| **N/A**（无 admin UI，配置走 git）| 完全 plugin 自定义 React + 必须用 Strapi Design System | N/A | 半自动：plugin 用 `add_settings_field` + PHP callback 渲染 HTML | 完全自动（基于 Go schema 反射）|
| **多租户/隔离** | `org_id` 列内置 | 三层 scope (`user/workspace/folder`) + language overrides | 多 config 文件 merge（`--config a.yaml --config b.yaml`）| N/A | namespace + RBAC | 无 | N/A |

---

## 3. 给我们的启示（按 V5-DESIGN W3 落地需求逐条决策）

> Curator 用法：每行直接看"决策"列，"理由"列只在你想推翻决策时再看。

| # | 决策点 | 决策 | 业界依据 | 落地动作（具体到 V5-DESIGN 的位置）|
|---|--------|------|----------|----------------------------------|
| 1 | `plugin_settings` 表是否拆 plain/secret 双列？ | **拆**：`value_jsonb` + `secure_value_jsonb` 两列，后者存 W5 加密后的 blob。后者读 API 永远 mask。 | Grafana `plugin_setting` 实测拆 `json_data` + `secure_json_data`；VS Code 干脆把 secret 完全拆出去单独 SQLite + keyring；Backstage 把 secret 推给外部 store。**三家头部都拆了**。 | V5-DESIGN §2.3 D 段加一行："secure_value_jsonb 列由 W5 SecretEncryption 写入，admin GET 时只返回 keys 列表，永不返回明文" |
| 2 | 是否引入 envelope encryption（DEK/KEK 两层）？ | **暂不**，V5 单层（HKDF 派生 + AES-GCM）就够；V6 再考虑。 | Grafana v9 才加 envelope，主要为支持 KMS。我们当前没 KMS 集成需求。 | V5-DESIGN §3.2（W5 部分）保持不动 |
| 3 | settings 表是否加 `org_id` / `tenant_id`？ | **不加**，留 schema migration hook（`(plugin_name, key)` 唯一索引而非 `(org_id, plugin_name, key)`）。 | Grafana 是平台级 multi-tenant 才需要。我们当前是单租户系统。 | V5-DESIGN §2.3 D 段表结构注释加一行："org_id 列预留位由 V6 multi-tenant 改造时再加" |
| 4 | 是否抄 Backstage 的 `x-visibility: frontend\|backend\|secret`？ | **抄**，作为 `x-visibility` 自定义 JSON Schema keyword。`secret` 字段 host 不返回明文给前端。 | Backstage 实证可工程化（合并冲突直接 fail startup）；VS Code 没有此区分（单用户工具不需要）。我们 plugin/host/admin 三方边界清晰，需要这个标记。 | V5-DESIGN §2.3 B 段 `SettingsSchema` 结构里加一句："JSONSchema 内字段可标 `x-visibility: frontend\|backend\|secret`，host 在 `GET /api/v1/admin/settings/plugins/:name` 响应时按此过滤" |
| 5 | schema 演进策略选哪种？ | **抄 VS Code 模式**：`x-deprecated: "<message>"` + 旧字段永久兼容。**不做** K8s 那套多版本 conversion webhook（过度设计）。 | K8s 完整方案是因为它是公共 API，第三方 controller 大量依赖；我们的 plugin schema 是内部消费。VS Code 模式刚好够用：deprecation 软迁移 + plugin 自己在 `Get(ctx)` 时做字段 fallback。 | V5-DESIGN §2.3 增加 §2.3.E.4："schema 字段级演进：旧字段保留 + 加 `x-deprecated` 标记；admin UI 渲染时给字段加 deprecated 样式 + 隐藏未配置过的字段（VS Code 风格）" |
| 6 | scope 字段（machine/window/resource）要不要抄？ | **简化**：单字段 `x-scope: "instance"` 默认，预留 `"plugin_global"`。当前不需要 user/workspace 概念。 | VS Code 6 个 scope 是 IDE 多 window/sync 场景；我们是单实例 server，不需要这么复杂。但保留扩展位避免后续改 schema breaking. | V5-DESIGN §2.3 B 段 `SettingsSchema` 注释里加："`x-scope` 当前仅 `instance`（默认值），未来 V6 multi-tenant 时支持 `plugin_global`" |
| 7 | 是否需要 "改完要重启" 标记？ | **加 `x-requires-reload: true`**，admin UI 渲染时给字段加红色提示 "保存后会重启 plugin"。 | VS Code 没有显式标记但靠 reload window 弹窗；我们 plugin 进程是独立的，可以精确知道哪些字段必须重启 plugin（如 `defaultIntervalSec` 改了 scheduler 要重启）。 | V5-DESIGN §2.3.E 增加："`x-requires-reload: true` 字段被改 → host 写入后通过 W3 的 reconcile 重启 plugin 进程；admin UI 显示警告 banner" |
| 8 | default 怎么落地？schema 内 inline 还是独立 defaults map？ | **schema 内 inline**（Backstage / K8s 都这么做），`SettingsSchema.Defaults` 字段在 SDK 层只是 schema 内 default 的 Go 形式 cache。**`plugin_settings` 表只存用户改过的值**，未配置的 key 启动时即时填默认（不写库）。 | K8s 是 admission 时持久化默认值——但代价是 schema 改 default 时**老对象不会自动更新**。我们读时填默认 = schema 改 default → 立即生效（用户体验更好）。 | V5-DESIGN §2.3.E 增加："host SettingService.GetForPlugin(pluginName) 实现：先查 plugin_settings 表，对未存在的 key 用 schema 内 default 填充，结果作为完整 config 给 plugin。schema 改 default 不需要 backfill 数据库。" |
| 9 | UI 富文本支持哪些字段？ | V5 必须支持 `description / enum / enumDescriptions / default / type / minimum / maximum / pattern`。markdown 系列（`markdownDescription`）留 V6。 | VS Code 是 schema-driven 标杆，但我们当前没 markdown 渲染基建（前端无 markdown component），先别上。 | V5-DESIGN §2.3 增加 §2.3.H "JSON Schema 支持子集"：列出 V5 必须支持的关键字白名单，与 D2 决策提到的 vue-json-schema-form Draft-07 子集对齐 |
| 10 | UI 库选什么？schema-driven 还是手写？ | （**已由 V5-DESIGN D2 决策**：`crickford/vue-json-schema-form` PoC + 手写 fallback）保持不变。**新增建议**：fallback 时 form layout 复用 Element Plus 的 `<el-form>` + 按 schema `properties` 顺序渲染（不要按 ID 字典序——VS Code 按字典序但有人吐槽过）。 | VS Code 的字典序 ordering 是缺陷（用户希望按声明顺序）；新版本加了 `order` 字段补救。我们直接按 properties 在 JSON 里的顺序渲染（JS 对象 key 顺序保证）。 | V5-DESIGN D2 决策段补一句："fallback 渲染按 JSON Schema `properties` map 中字段声明顺序，不字典序" |
| 11 | secret 字段写入时是 "覆盖" 还是 "可空跳过"？ | **抄 Grafana**：admin 提交 secret 字段时，**只发送用户改过的 key**；空字符串 = 清除；未发送 = 保持原值。 | Grafana 实测："如果你设置 secureJsonData 中的某个 key，只发送用户修改过的那些 key — 任何值（包括空字符串）都会覆盖现有值"。这避免了 "前端拿 mask 的占位符回写导致用 mask 覆盖明文" 的经典 bug。 | V5-DESIGN §2.3.E 增加 "secret 字段写入语义：admin PUT body 只包含本次修改的 key，未提及的 key 保持原值；显式传空字符串 = 清除" |
| 12 | Watch 断流降级怎么实现？ | 已写在 V5-DESIGN ("SDK 自动重连 + 全量同步当前值")。**新增建议**：plugin 启动时第一次 Get 全量 + Watch 增量；Watch 断了走 exponential backoff 重连，重连后再次全量 Get 兜底。 | K8s informer 模式（list-watch）就是这个套路，业界成熟。 | V5-DESIGN §2.3.E "Watch 断流" 段补一句："SDK 内置 list-then-watch 模式：每次重连后先全量 Get 一次再继续 Watch，避免错过断流期间的变更" |
| 13 | schema 校验失败时的行为？ | **启动失败 + admin UI 回退到 default**（V5-DESIGN 已写），保持。**补一条**：plugin manifest schema 不合法（写错 JSON Schema 自身）→ host 启动 plugin 失败，错误写到 plugin status，admin UI 显示红色错误。 | Backstage 是 fail-fast（schema merge 冲突 = 启动失败）；K8s 也是（CRD schema 不合法直接拒收）。 | V5-DESIGN §2.3.E "失败/降级" 段加："plugin 上报 SettingsSchema.JSONSchema 自身不是合法 JSON Schema Draft-07 → plugin status=ErrorBadSettingsSchema，gateway 摘流" |

---

## 4. 备查：所有引用的 URL + 关键源文件

### 一手文档

- Grafana plugin.json reference — https://grafana.com/developers/plugin-tools/reference/plugin-json
- Grafana data source auth — https://grafana.com/developers/plugin-tools/how-to-guides/data-source-plugins/add-authentication-for-data-source-plugins
- Grafana app plugin auth — https://grafana.com/developers/plugin-tools/how-to-guides/app-plugins/add-authentication-for-app-plugins
- Grafana DB encryption — https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-database-encryption/
- Grafana provisioning — https://grafana.com/docs/grafana/latest/administration/provisioning/
- VS Code Contribution Points — https://code.visualstudio.com/api/references/contribution-points
- VS Code SecretStorage API ref — https://vscode-api.js.org/interfaces/vscode.SecretStorage.html
- VS Code SecretStorage discussion — https://github.com/microsoft/vscode-discussions/discussions/748
- Backstage Defining Configuration — https://backstage.io/docs/conf/defining/
- Backstage Writing Configuration — https://backstage.io/docs/conf/writing/
- Backstage meta-schema — https://backstage.io/schema/config-v1
- Strapi v4 plugins config — https://docs-v4.strapi.io/dev-docs/configurations/plugins
- Strapi v4 server API — https://docs-v4.strapi.io/dev-docs/api/plugins/server-api
- Strapi v4 admin panel API — https://docs-v4.strapi.io/dev-docs/api/plugins/admin-panel-api
- Strapi 5 server-configuration — https://docs.strapi.io/cms/plugins-development/server-configuration
- K8s CRD docs — https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/
- K8s CRD versioning — https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/
- K8s OpenAPI v3 GA blog — https://kubernetes.io/blog/2023/04/24/openapi-v3-field-validation-ga/
- WordPress Settings API handbook — https://developer.wordpress.org/plugins/settings/settings-api/
- WordPress register_setting reference — https://developer.wordpress.org/reference/functions/register_setting/
- WordPress Codex Settings API — https://codex.wordpress.org/Settings_API
- Terraform plugin SDK schema (pkg.go.dev) — https://pkg.go.dev/github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema
- Terraform schema behaviors — https://developer.hashicorp.com/terraform/plugin/sdkv2/schemas/schema-behaviors

### 一手源码（实际拉到本文里引用）

- Grafana `plugin_setting` migration（直接拉到完整源码，§1.1 引用）— https://github.com/grafana/grafana/blob/main/pkg/services/sqlstore/migrations/plugin_setting.go
- VS Code `configurationExtensionPoint.ts` — https://github.com/microsoft/vscode/blob/main/src/vs/workbench/api/common/configurationExtensionPoint.ts
- VS Code `configurationRegistry.ts` — https://github.com/microsoft/vscode/blob/main/src/vs/platform/configuration/common/configurationRegistry.ts
- Terraform plugin SDK schema.go — https://github.com/hashicorp/terraform-plugin-sdk/blob/main/helper/schema/schema.go

### 二手参考（仅作辅证，决策不依赖）

- Cycode "VS Code's Token Security" — https://cycode.com/blog/exposing-vscode-secrets/
- Cycode "One Plugin Away: Breaking Into Grafana from the Inside" — https://cycode.com/blog/one-plugin-away-breaking-into-grafana-from-the-inside/
- iconicwp/WordPress-Settings-Framework（社区 schema-driven wrapper，证明官方机制不够）— https://github.com/iconicwp/WordPress-Settings-Framework

---

## 5. 一句话给 Curator 的总结

**核心抄袭对象 = Backstage（schema 哲学：Draft-07 + `x-visibility` + meta-schema URL 钉版本）+ VS Code（UI 哲学：schema-driven 自动渲染 + `deprecationMessage` 软演进）+ Grafana（存储哲学：单表双列 plain/secret + 写时只覆盖改过的 key）。**

**核心反面教材 = WordPress（命令式 + filter hook 拼 UI，做不出 schema-driven）+ Strapi v4（validator 函数式，前端不可消费）+ Terraform（Go struct schema，跨进程不可移植）。**

**最重要的两个抄过来的小细节**：
1. Grafana 的 secret 写入语义（"未提及 key 保持原值，空串清除"）—— 避免 mask 占位符被回写覆盖明文的经典 bug
2. Backstage 的 schema 合并冲突 fail-fast（同字段双标 frontend+secret 直接 startup 报错）—— 避免运行时才暴露权限漏洞
