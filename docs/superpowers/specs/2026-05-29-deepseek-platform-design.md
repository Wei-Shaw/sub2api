---
title: "DeepSeek Platform Integration"
change: "deepseek-platform"
design-doc: "docs/superpowers/specs/2026-05-29-deepseek-platform-design.md"
date: "2026-05-29"
status: "draft"
---

# DeepSeek 平台集成设计

## 1. 概述

在 Sub2API 中新增 DeepSeek 作为独立平台，支持 API Key 类型账号，复用 OpenAI 兼容转发逻辑，自带独立的模型白名单管理。用户通过 Sub2API 统一入口发送请求（支持 Anthropic 和 OpenAI 两种格式），网关自动路由并格式转换后转发给 DeepSeek。

本次集成同时预留 OAuth 扩展接口，为后续可能的 OAuth 登录流程做准备。

## 2. 架构决策

### 2.1 平台标识策略

采用 **独立 Platform 字符串 + 复用 OpenAI 转发逻辑** 的策略：

- DeepSeek 拥有独立的 `platform = "deepseek"` 常量
- 在模型白名单、调度策略、账号管理上与 OpenAI 完全解耦
- 网关转发层复用 OpenAI `/v1/chat/completions` 格式（DeepSeek API 完全兼容此格式）

选择理由：DeepSeek 作为独立平台需要自己的生态位（模型白名单、配额策略、调度规则），但其 API 与 OpenAI 完全兼容，转发逻辑无需重复实现。

### 2.2 请求路由

| 用户请求格式 | 网关处理 | 转发目标 |
|-------------|---------|---------|
| Anthropic `/v1/messages` | 转换为 OpenAI `/v1/chat/completions` | `api.deepseek.com/v1/chat/completions` |
| OpenAI `/v1/chat/completions` | 直接透传 | `api.deepseek.com/v1/chat/completions` |

### 2.3 数据流

```
客户端 → Sub2API Gateway
  → resolvePlatform 解析 platform = "deepseek"
  → listSchedulableAccounts 筛选 deepseek 账号
  → 按 RPM/配额/模型白名单过滤
  → 选中最佳账号，取出 API Key
  → 如有需要，将 Anthropic 格式转为 OpenAI 格式
  → 转发到 api.deepseek.com/v1/chat/completions
  → 将响应按原始请求格式返回
```

## 3. 后端改动

### 3.1 常量定义

**`backend/internal/domain/constants.go`**
- 新增 `PlatformDeepSeek = "deepseek"`

**`backend/internal/service/domain_constants.go`**
- 新增 `PlatformDeepSeek = "deepseek"` 重新导出
- 加入 `AllowedQuotaPlatforms` 列表

### 3.2 账号逻辑

**`backend/internal/service/account.go`**
- 新增 `IsDeepSeek()` 平台判断方法
- 新增 `GetDeepSeekAPIKey()` / `GetDeepSeekBaseURL()` 凭证访问器
- 新增 `IsDeepSeekAPIKey()` 类型判断方法
- 在 `IsModelSupported()` 等模型相关方法中加入 DeepSeek 分支

### 3.3 Token Provider

**`backend/internal/service/deepseek_token_provider.go`（新文件）**
- 新建 Token Provider，复用 OpenAI 的结构定义
- 本次只实现 API Key 模式（`GetToken()` 直接返回 API Key）
- 预留 OAuth 接口方法，返回 `ErrNotImplemented`

### 3.4 网关转发

**`backend/internal/service/gateway_service.go`**
- 在 `resolvePlatform` 中支持 deepseek
- 在账号选择逻辑的 platform 分支中加入 DeepSeek
- DeepSeek 转发复用 OpenAI 格式的转发路径（`/v1/chat/completions`）
- 在 Anthropic → OpenAI 格式转换层中，DeepSeek 作为目标平台时走 OpenAI 转换逻辑

### 3.5 Handler

**`backend/internal/handler/admin/account_handler.go`**
- 创建 DeepSeek 账号时走 API Key 路径
- 不支持 OAuth 创建（预留入口，返回明确错误提示）

## 4. 前端改动

### 4.1 类型定义

**`frontend/src/types/index.ts`**
- `AccountPlatform` 联合类型增加 `'deepseek'`

### 4.2 账号创建

**`frontend/src/components/account/CreateAccountModal.vue`**
- DeepSeek 平台选择按钮
- API Key 表单字段（`api_key`、`base_url`，默认 `https://api.deepseek.com`）

### 4.3 账号编辑

**`frontend/src/components/account/EditAccountModal.vue`**
- DeepSeek 编辑表单，复用 OpenAI 风格的凭证输入

### 4.4 模型白名单

**`frontend/src/composables/useModelWhitelist.ts`**
- DeepSeek 预设模型列表：`deepseek-chat`、`deepseek-reasoner`、`deepseek-v4-flash`、`deepseek-v4-pro`
- DeepSeek 模型预设映射

## 5. 不做的范围

- DeepSeek OAuth 登录流程（仅预留接口定义）
- DeepSeek WebSocket 实时流
- Anthropic 专有功能（tool use、extended thinking）到 DeepSeek 的完整映射
- 健康检测 / 自动重试
- 数据库 schema 变更（platform 字段是通用字符串，无需 migration）

## 6. 错误处理

| 错误场景 | 处理方式 |
|---------|---------|
| DeepSeek 401/403 | 标记账号异常 |
| DeepSeek 429 | 标记限流，按 RPM 调度逻辑处理 |
| DeepSeek 5xx | 尝试重试或降级 |
| 模型不在白名单 | 在模型白名单层拦截 |

## 7. 测试策略

- DeepSeek 平台常量单元测试
- 账号创建 API 集成测试
- 模型白名单匹配测试
- 网关 Anthropic → OpenAI 格式转换单元测试
