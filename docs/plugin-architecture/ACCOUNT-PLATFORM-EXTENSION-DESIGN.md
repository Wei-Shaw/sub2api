# Account Platform Extension — 插件化账号管理系统设计

> 状态: DRAFT  
> 日期: 2026-05-07  
> 前置: GATEWAY-EXTRACTION Phase 1 (pipeline + provider registry)

---

## 1. 问题陈述

当前账号管理**硬耦合** 4 个平台（Anthropic / OpenAI / Gemini / Antigravity）：

| 文件 | 行数 | 耦合方式 |
|------|------|----------|
| `CreateAccountModal.vue` | 5190 | 硬编码平台 Tab + 账号类型卡片 + 表单字段 |
| `EditAccountModal.vue` | 3714 | 同上 |
| `AccountsView.vue` | 1640 | 平台过滤器 + 图标/颜色映射 |
| `AccountUsageCell.vue` | 1243 | 平台特定用量展示 |
| `account_handler.go` | 2221 | 平台特定 OAuth service × 4 |
| `account_test_service.go` | 1567 | 平台特定测试逻辑 |
| `account.go` (model) | 2186 | `IsPrivacySet()` / `IsGemini()` 等平台 switch |
| `domain/constants.go` | — | 硬编码 Platform* / AccountType* 常量 |

**新增一个平台**需要修改 **15+ 文件**，跨前后端两侧。这违背了工程原则中的**抽象**和**解耦**。

### 目标

- 插件声明平台 + 账号类型 + UI + 验证 + 测试连接
- Account CRUD 留在 host（调度 / 计费 / 并发 强依赖）
- 新增平台 = 新增一个 gateway 插件，**零改动** host 代码
- 账号列表界面保持现有外观和功能不变

---

## 2. 架构总览

```
┌─────────────────────────────────────────────────────┐
│                      Host (Core)                     │
│                                                      │
│  ┌──────────────┐   ┌──────────────┐                │
│  │ Account CRUD │   │ PlatformReg  │◀── collects    │
│  │ (AdminSvc)   │──▶│ (registry)   │    from all    │
│  └──────────────┘   └──────┬───────┘    plugins     │
│         │                  │                         │
│  ┌──────┴──────┐    ┌──────┴───────┐                │
│  │ AccountHndl │    │ AcctTestSvc  │                │
│  │ (refactored)│    │ (refactored) │                │
│  └─────────────┘    └──────────────┘                │
│                                                      │
│  Frontend:                                           │
│  ┌─────────────────────────────────────────────┐    │
│  │ AccountsView (list, filters, columns — host)│    │
│  │ CreateAccountModal (step wizard — host)      │    │
│  │   Step 1: platformRegistry → platform tabs   │    │
│  │   Step 2: PluginAccountForm (plugin or JSON) │    │
│  │ EditAccountModal (same pattern)              │    │
│  │ AccountTestModal (delegates to plugin)       │    │
│  └─────────────────────────────────────────────┘    │
└───────────────────────┬─────────────────────────────┘
                        │ gRPC
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
┌──────────────┐ ┌─────────────┐ ┌─────────────┐
│ gateway-     │ │ gateway-    │ │ gateway-    │
│ anthropic    │ │ openai      │ │ gemini      │
│              │ │             │ │             │
│ Manifest:    │ │ Manifest:   │ │ Manifest:   │
│  platforms:  │ │  platforms: │ │  platforms: │
│  - anthropic │ │  - openai   │ │  - gemini   │
│  account_    │ │  ...        │ │  ...        │
│  types: [..] │ │             │ │             │
│              │ │             │ │             │
│ Implements:  │ │ Implements: │ │ Implements: │
│ AccountPlat- │ │ AccountPlat-│ │ AccountPlat-│
│ formExt gRPC │ │ formExt     │ │ formExt     │
│              │ │             │ │             │
│ Frontend:    │ │ Frontend:   │ │ Frontend:   │
│  custom Vue  │ │  custom Vue │ │  custom Vue │
│  components  │ │  components │ │  components │
└──────────────┘ └─────────────┘ └─────────────┘
```

### 分层原则

| 层 | 归属 | 职责 |
|----|------|------|
| Account 实体 / DB | Host | 存储、CRUD、调度、计费 |
| 平台声明 / 验证 | Plugin (via gRPC) | 定义平台、类型、字段约束 |
| 创建/编辑 UI | Plugin (Vue 组件) | 平台特定表单、OAuth 流程 |
| 列表 UI | Host | 通用表格、过滤器、列渲染 |
| 列表单元格渲染 | Host + Plugin 数据 | host 渲染通用列，plugin 提供图标/颜色/自定义展示配置 |
| 测试连接 | Plugin (via gRPC) | 平台特定 API 调用 |
| Token 刷新 | Plugin (via gRPC) | 平台特定 OAuth/Token 管理 |

---

## 3. Manifest 扩展：PlatformDeclaration

在现有 `ManifestResponse` 中新增 `platforms` 字段：

```protobuf
message ManifestResponse {
  // ... existing fields ...

  // platforms declares the account platforms this plugin provides.
  repeated PlatformDeclaration platforms = 60;
}

// PlatformDeclaration describes an account platform a gateway plugin provides.
message PlatformDeclaration {
  // platform is the unique identifier stored in account.platform DB column.
  // Must be lowercase alphanumeric + hyphens (e.g. "anthropic", "openai").
  string platform = 1;

  // display_name is the human-readable platform name shown in UI.
  string display_name = 2;

  // icon_svg is the complete SVG markup for the platform icon.
  string icon_svg = 3;

  // theme_color is the CSS color used for platform badges/highlights.
  string theme_color = 4;

  // account_types declares the account types this platform supports.
  repeated AccountTypeDeclaration account_types = 5;

  // capacity_display customizes the "容量" column rendering.
  // nil/empty = default (show concurrency only).
  CapacityDisplayConfig capacity_display = 6;

  // usage_display customizes the "用量窗口" column rendering.
  UsageDisplayConfig usage_display = 7;

  // custom_actions declares additional items in the "更多操作" menu.
  repeated CustomActionDeclaration custom_actions = 8;

  // test_config customizes the test connection dialog.
  TestConnectionConfig test_config = 9;

  // sort_order controls ordering in platform selector tabs.
  int32 sort_order = 10;

  // privacy_states declares the privacy states this platform supports.
  // These appear in the "Privacy状态" filter dropdown.
  // Empty = this platform has no privacy concept.
  repeated PrivacyState privacy_states = 11;
}

// AccountTypeDeclaration describes an account type within a platform.
message AccountTypeDeclaration {
  string type = 1;           // e.g. "oauth", "apikey", "bedrock"
  string display_name = 2;   // e.g. "Claude Code", "Claude Console"
  string description = 3;    // e.g. "OAuth / Setup Token"
  string icon_svg = 4;       // optional; falls back to platform icon
  string theme_color = 5;    // optional; falls back to platform color

  // credential_schema is JSON Schema (Draft-07) for credential fields.
  // Host renders a form from this when no custom component is provided.
  bytes credential_schema = 6;

  // extra_schema is JSON Schema for extra fields.
  bytes extra_schema = 7;

  // form_component_path names a Vue component the plugin ships for
  // rendering the creation/edit form. Empty = use schema renderer.
  string form_component_path = 8;

  // sub_types allows account type sub-options.
  // Example: "oauth-based" has sub-types "OAuth" and "Setup Token".
  repeated SubTypeOption sub_types = 9;

  int32 sort_order = 10;

  // badge_label is the short text on the list view type badge.
  string badge_label = 11;
}

message SubTypeOption {
  string value = 1;  // stored value (e.g. "oauth", "setup-token")
  string label = 2;  // display text
}

message CapacityDisplayConfig {
  bool show_concurrency = 1;  // default: true
  repeated CapacityRow extra_rows = 2;
}

message CapacityRow {
  string label = 1;   // e.g. "D", "W", "总"
  string source = 2;  // e.g. "extra.daily_cost" or "credential.quota"
  string format = 3;  // "currency" | "percentage" | "count"
}

message UsageDisplayConfig {
  string component_path = 1;   // custom Vue component (optional)
  string window_label = 2;     // e.g. "5h窗口"
  bool show_req_count = 3;
  bool show_cost = 4;
  repeated UsageRow extra_rows = 5;
}

message UsageRow {
  string label = 1;
  string source = 2;
  string format = 3;
}

message CustomActionDeclaration {
  string action_id = 1;
  string icon_svg = 2;
  map<string, string> labels = 3;  // locale → label
  string action_type = 4;          // "api_call" | "open_modal"
  string api_endpoint = 5;
  string component_path = 6;
  int32 sort_order = 7;
}

message TestConnectionConfig {
  bool model_selector = 1;
  string test_component_path = 2;
  string default_test_model = 3;
}

message PrivacyState {
  string value = 1;        // stored in extra.privacy_mode
  string display_name = 2;
  string badge_color = 3;
  bool is_set = 4;         // true = privacy is configured
}
```

### Go SDK 对应类型

```go
// plugin-sdk/manifest.go 新增

type PlatformDecl struct {
    Platform        string
    DisplayName     string
    IconSVG         string
    ThemeColor      string
    AccountTypes    []AccountTypeDecl
    CapacityDisplay *CapacityDisplayConfig
    UsageDisplay    *UsageDisplayConfig
    CustomActions   []CustomActionDecl
    TestConfig      *TestConnectionConfig
    SortOrder       int
    PrivacyStates   []PrivacyStateDecl
}

type AccountTypeDecl struct {
    Type              string
    DisplayName       string
    Description       string
    IconSVG           string
    ThemeColor        string
    CredentialSchema  json.RawMessage
    ExtraSchema       json.RawMessage
    FormComponentPath string
    SubTypes          []SubTypeOption
    SortOrder         int
    BadgeLabel        string
}

type SubTypeOption struct {
    Value string
    Label string
}

type CapacityDisplayConfig struct {
    ShowConcurrency bool
    ExtraRows       []DisplayRow
}

type UsageDisplayConfig struct {
    ComponentPath string
    WindowLabel   string
    ShowReqCount  bool
    ShowCost      bool
    ExtraRows     []DisplayRow
}

type DisplayRow struct {
    Label  string
    Source string
    Format string
}

type CustomActionDecl struct {
    ActionID      string
    IconSVG       string
    Labels        map[string]string
    ActionType    string
    APIEndpoint   string
    ComponentPath string
    SortOrder     int
}

type TestConnectionConfig struct {
    ModelSelector     bool
    TestComponentPath string
    DefaultTestModel  string
}

type PrivacyStateDecl struct {
    Value       string
    DisplayName string
    BadgeColor  string
    IsSet       bool
}
```

---

## 4. AccountPlatformExtension gRPC 服务

插件实现此 gRPC 服务来处理账号生命周期中的平台特定操作：

```protobuf
service AccountPlatformExtension {
  // ValidateAccountData validates credentials/extra before host persists.
  rpc ValidateAccountData(ValidateAccountDataRequest)
      returns (ValidateAccountDataResponse);

  // TestConnection performs platform-specific connectivity test.
  rpc TestConnection(TestConnectionRequest)
      returns (stream TestConnectionEvent);

  // RefreshToken performs platform-specific token refresh.
  rpc RefreshToken(RefreshTokenRequest)
      returns (RefreshTokenResponse);

  // RefreshTier refreshes account tier/plan info from upstream.
  rpc RefreshTier(RefreshTierRequest)
      returns (RefreshTierResponse);

  // GetAvailableModels returns models available for an account.
  rpc GetAvailableModels(GetModelsRequest)
      returns (GetModelsResponse);

  // ExecuteCustomAction handles plugin-defined custom actions.
  rpc ExecuteCustomAction(CustomActionRequest)
      returns (CustomActionResponse);
}

message ValidateAccountDataRequest {
  string platform = 1;
  string account_type = 2;
  bytes credentials_json = 3;
  bytes extra_json = 4;
  bool is_update = 5;
  int64 account_id = 6;
}

message ValidateAccountDataResponse {
  bool valid = 1;
  map<string, string> field_errors = 2;
  bytes processed_credentials_json = 3;
  bytes processed_extra_json = 4;
}

message TestConnectionRequest {
  int64 account_id = 1;
  string platform = 2;
  string account_type = 3;
  bytes credentials_json = 4;
  bytes extra_json = 5;
  string model_id = 6;
}

message TestConnectionEvent {
  string type = 1;     // "test_start" | "content" | "test_end" | "error"
  string text = 2;
  string model = 3;
  bool success = 4;
  string error = 5;
  bytes data_json = 6;
}

message RefreshTokenRequest {
  int64 account_id = 1;
  string platform = 2;
  string account_type = 3;
  bytes credentials_json = 4;
  bytes extra_json = 5;
}

message RefreshTokenResponse {
  bool success = 1;
  string error = 2;
  bytes updated_credentials_json = 3;
  bytes updated_extra_json = 4;
}

message RefreshTierRequest {
  int64 account_id = 1;
  string platform = 2;
  string account_type = 3;
  bytes credentials_json = 4;
  bytes extra_json = 5;
}

message RefreshTierResponse {
  bool success = 1;
  string error = 2;
  bytes updated_extra_json = 3;
}

message GetModelsRequest {
  int64 account_id = 1;
  string platform = 2;
  string account_type = 3;
  bytes credentials_json = 4;
}

message GetModelsResponse {
  repeated ModelInfo models = 1;
}

message ModelInfo {
  string model_id = 1;
  string display_name = 2;
  bool available = 3;
}

message CustomActionRequest {
  string action_id = 1;
  int64 account_id = 2;
  string platform = 3;
  bytes credentials_json = 4;
  bytes extra_json = 5;
  bytes payload_json = 6;
}

message CustomActionResponse {
  bool success = 1;
  string error = 2;
  bytes result_json = 3;
  bytes updated_credentials_json = 4;
  bytes updated_extra_json = 5;
}
```

---

## 5. Host 端：PlatformRegistry

```go
// backend/internal/plugin/platform_registry.go

type PlatformRegistry struct {
    mu        sync.RWMutex
    platforms map[string]*RegisteredPlatform
}

type RegisteredPlatform struct {
    Decl       PlatformDecl
    PluginName string
    Conn       *grpc.ClientConn
}

func (r *PlatformRegistry) Register(pluginName string, decl PlatformDecl, conn *grpc.ClientConn)
func (r *PlatformRegistry) Unregister(pluginName string)
func (r *PlatformRegistry) Get(platform string) (*RegisteredPlatform, bool)
func (r *PlatformRegistry) All() []RegisteredPlatform
func (r *PlatformRegistry) AllPlatformIDs() []string
func (r *PlatformRegistry) AccountTypesFor(platform string) []AccountTypeDecl
```

### 新增 API 端点

```
GET  /api/v1/admin/platforms              → 所有注册的平台声明
GET  /api/v1/admin/platforms/:platform    → 单个平台详情
```

---

## 6. Account Handler 重构

### 当前（耦合）

```go
type AccountHandler struct {
    oauthService            *service.OAuthService           // ← 移除
    openaiOAuthService      *service.OpenAIOAuthService     // ← 移除
    geminiOAuthService      *service.GeminiOAuthService     // ← 移除
    antigravityOAuthService *service.AntigravityOAuthService // ← 移除
    // ...
}
```

### 重构后（解耦）

```go
type AccountHandler struct {
    adminService       service.AdminService
    platformRegistry   *plugin.PlatformRegistry  // ← 新增
    rateLimitService   *service.RateLimitService
    concurrencyService *service.ConcurrencyService
    // ... (保留通用 services)
}
```

**CreateAccount 流程**：

```
请求 → 验证 platform 在 PlatformRegistry 中存在
     → 验证 account_type 在 platform 的 types 中存在
     → gRPC: plugin.ValidateAccountData(credentials, extra)
     → 失败: 返回 field_errors
     → 成功: adminService.CreateAccount(processedData)
```

**TestConnection 流程**：

```
请求 → platformRegistry.Get(account.Platform)
     → gRPC: plugin.TestConnection(account, model)
     → pipe gRPC stream → SSE response
```

---

## 7. 前端重构策略

### 7.1 AccountsView.vue（列表页 — 保持外观不变）

**Filter 下拉框**：从 `/api/v1/admin/platforms` 动态获取。

```typescript
// 当前（硬编码）
const platformOptions = [
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  // ...
]

// 重构后（动态）
const { data: platforms } = useQuery(['platforms'], api.getPlatforms)
const platformOptions = computed(() =>
  platforms.value?.map(p => ({ value: p.platform, label: p.display_name }))
)
```

**平台/类型列**：用通用 `PlatformBadge` + `TypeBadge` 组件，颜色/图标来自 registry。

**容量列**：根据 `CapacityDisplayConfig` 动态渲染。

**用量窗口列**：根据 `UsageDisplayConfig` 动态渲染。

### 7.2 CreateAccountModal.vue（大幅重构）

**Step 1: 平台 + 类型选择**（host 渲染，数据来自 registry）

```
┌─────────────────────────────────┐
│ 账号名称: [__________________]  │
│ 备注:     [__________________]  │
│                                 │
│ 平台:  (动态渲染自 registry)    │
│ ┌──────┐┌──────┐┌──────┐       │
│ │Anthr.││OpenAI││Gemini│ ...   │
│ └──────┘└──────┘└──────┘       │
│                                 │
│ 账号类型: (动态渲染)            │
│ ┌──────────┐┌──────────┐       │
│ │Claude Cod││Console   │ ...   │
│ └──────────┘└──────────┘       │
│                                 │
│ [取消]              [下一步 →]  │
└─────────────────────────────────┘
```

**Step 2: 账号设置表单**（混合策略）

```vue
<template>
  <!-- 优先使用插件自定义组件 -->
  <component
    v-if="selectedType.formComponentPath"
    :is="pluginComponent"
    mode="create"
    v-model="formData"
    @validate="onValidate"
  />
  <!-- 降级: JSON Schema 表单渲染 -->
  <JsonSchemaForm
    v-else
    :credential-schema="selectedType.credentialSchema"
    :extra-schema="selectedType.extraSchema"
    v-model="formData"
    @validate="onValidate"
  />

  <!-- 通用字段区域（所有平台共用） -->
  <CommonAccountFields v-model="commonFields" />
</template>
```

### 7.3 新增 Host 通用组件

| 组件 | 职责 |
|------|------|
| `PlatformBadge.vue` | 渲染平台 icon + name + color |
| `TypeBadge.vue` | 渲染账号类型 badge |
| `PlatformSelector.vue` | 平台选择 Tab 组件 |
| `AccountTypeSelector.vue` | 账号类型卡片选择器 |
| `JsonSchemaForm.vue` | JSON Schema → 表单渲染器 |
| `PluginFormLoader.vue` | 加载并挂载插件 Vue 组件 |
| `CommonAccountFields.vue` | 通用字段区域（proxy, concurrency, groups...） |
| `DynamicCapacityCell.vue` | 动态容量列渲染 |
| `DynamicUsageCell.vue` | 动态用量列渲染 |

---

## 8. 通用字段 vs 插件字段

### 通用字段（host 管理）

| 字段 | 位置 | 可扩展 |
|------|------|--------|
| `name` | Step 1 | 否 |
| `notes` | Step 1 | 否 |
| `platform` | Step 1 平台选择器 | 插件声明 |
| `type` | Step 1 类型选择器 | 插件声明 |
| `proxy_id` | 通用设置区 | 否 |
| `concurrency` | 通用设置区 | 否 |
| `priority` | 通用设置区 | 否 |
| `rate_multiplier` | 通用设置区 | 否 |
| `load_factor` | 通用设置区 | 否 |
| `group_ids` | 通用设置区 | 否 |
| `expires_at` | 通用设置区 | 否 |
| `auto_pause_on_expired` | 通用设置区 | 否 |
| `status` | 编辑专用 | **不可扩展** |

### 插件字段

| 字段 | 控制方 |
|------|--------|
| `credentials.*` | 插件（credential_schema 或自定义组件） |
| `extra.*` | 插件（extra_schema 或自定义组件） |

---

## 9. 兼容性 & 降级

### 插件未加载时

- 列表正常展示已有账号（platform 显示原始字符串，灰色 badge）
- 创建新账号时，该平台不出现在选择器中
- 测试连接/Token刷新返回"平台插件未加载"错误

### 数据库

- Account 表不变。`platform` 和 `type` 是自由字符串
- 可选新增 `platform_declarations` 缓存表用于离线查询

---

## 10. 分阶段实施路线图

### Phase A: 协议定义 + SDK 骨架（~400 行）

1. proto 定义 PlatformDeclaration messages + AccountPlatformExtension service
2. protoc 生成 Go 代码
3. plugin-sdk Go 类型 + manifest 转换
4. plugin-sdk AccountPlatformExtension Go 接口

### Phase B: Host PlatformRegistry + API（~500 行）

1. `platform_registry.go` — 注册/查询/生命周期
2. Plugin Manager 集成：加载时注册，卸载时移除
3. `/api/v1/admin/platforms` API 端点
4. Account Handler 验证委托

### Phase C: Account Handler 解耦（~500 行新增，~2000 行移除）

1. CreateAccount: 验证 → PlatformRegistry → plugin gRPC
2. TestConnection: 委托 → plugin gRPC stream
3. RefreshToken: 委托 → plugin gRPC
4. RefreshTier: 委托 → plugin gRPC
5. 移除平台特定 OAuth service 依赖

### Phase D: 前端重构（~3000 行新增，~8000 行移除）

1. 通用组件: PlatformBadge, TypeBadge, Selectors, JsonSchemaForm
2. CreateAccountModal: 动态平台/类型 + 插件表单挂载
3. EditAccountModal: 同上模式
4. AccountsView: 动态过滤器 + 动态列渲染
5. AccountTestModal: 委托给插件

### Phase E: 首个 Gateway 插件验证

Anthropic 平台作为第一个 gateway 插件完整实现，验证流程。

---

## 11. 开放问题

| # | 问题 | 建议 | 状态 |
|---|------|------|------|
| Q1 | 多插件声明同一 platform | 拒绝后来者，first-win | 待定 |
| Q2 | 插件升级时声明变更 | 版本比较 + 渐进更新 | 待定 |
| Q3 | JSON Schema 表单库选型 | `@formkit/vue` 或自研轻量渲染器 | 待定 |
| Q4 | OAuth 回调路由到正确插件 | host 统一入口 `/oauth/:platform/callback` | 待定 |
| Q5 | Bulk Update credentials 验证 | 不经过插件（原行为） | 待定 |
