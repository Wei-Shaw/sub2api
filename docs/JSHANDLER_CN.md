# 网关 JavaScript 钩子（JS Handler）

Sub2API 可在网关上执行 JavaScript 脚本，改写请求与响应载荷。脚本运行在嵌入式 [Goja](https://github.com/dop251/goja) 虚拟机中，带固定超时，失败时保持原数据（fail-open）。

绑定模型：

| 钩子 | 绑定 |
|------|------|
| `on_before_request` | **分组** `jshandler_script_ids`（该分组下全部 API Key） |
| `on_after_auth_request` / 响应钩子 | **账号** `extra.jshandler_script_ids` |

本文描述 **sub2api 原生**模型（脚本库 + 分组/账号绑定）。钩子形态受上游 [cpa-plugin-jshandler](https://github.com/router-for-me/cpa-plugin-jshandler) 启发，但配置与生效方式不同。

英文版见 [JSHANDLER.md](./JSHANDLER.md)。

---

## 快速开始

1. **管理后台 → 设置 → Gateway** → 打开 **Gateway JavaScript hooks**。
2. 打开 **启用**，设置 **超时**（默认 `1s`，如 `500ms`、`2s`）。
3. 向脚本库 **上传** `.js` 文件（最大 **512 KiB**）。
4. **编辑分组** → 绑定 `on_before_request` 脚本（账号选择前，对该分组所有 Key 生效）。
5. **编辑账号** → 绑定 after-auth / 响应脚本。
6. 使用绑定该分组的 Key 发请求。

以下情况钩子**不会**执行：

- 全局配置 `enabled` 为 `false`，或
- 对应绑定为空（before 看分组，after/响应看账号）。

---

## 生效条件

| 层级 | 机制 |
|------|------|
| 全局 | 设置项 `jshandler_config`：`{ "enabled": bool, "timeout": string }` |
| 分组 | `groups.jshandler_script_ids` → `on_before_request`（选账号前执行一次） |
| 账号 | `extra.jshandler_script_ids`（有序列表，推荐）与兼容字段 `extra.jshandler_script_id` |
| 运行时 | 按 ID 从脚本库加载 → 按文件 mtime 编译/缓存 → 调用钩子 |

**没有**目录自动扫描，也**没有**全局 `script_paths` 列表。认证缓存快照会带上分组脚本 ID；更新分组会按 group 失效 auth cache。

---

## 存储布局

根目录：`{pricing.data_dir}/jshandler/`（配置项 `pricing.data_dir`，默认 `./data`）。

```
{data_dir}/jshandler/
  registry.json          # 脚本库索引
  scripts/
    {id}.js              # 脚本源码
```

`registry.json` 条目字段：`id`、`name`、`filename`、`created_at`、`updated_at`。

脚本 ID 为无横线的十六进制 UUID。允许格式：`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`。

---

## 钩子 API

脚本定义**全局函数**（不是 `module.exports`）。

| 函数 | 调用时机 |
|------|----------|
| `on_before_request(ctx)` | 内容审核后、**选账号前**（分组脚本） |
| `on_after_auth_request(ctx)` | 账号选定之后、请求体转发上游之前（账号脚本） |
| `on_after_nonstream_response(ctx)` | 非流式上游完整响应组装之后 |
| `on_after_stream_response(ctx)` | 每个流式数据载荷（以及一次合成的 header-init 调用） |

### 返回值

- 返回修改后的 **对象**（与 `ctx` 同形），设置 `body` / `chunk` / `headers`。
- 或返回 **字符串**，仅替换 `body`（请求/非流式）或 `chunk`（流式）。
- `headers[name] = null` 或 `[]` 表示删除该头。
- 流式：`chunk` 返回**空字符串**可**丢弃**该分片，不发给客户端。

**请求头：** 修改会写回入站 Gin 请求。是否真正发到上游取决于各协议路径如何组装上游 Header（多条路径仅**白名单**转发）。需要稳定效果时优先改 **`body`**。

**响应头：** 非流式/流式钩子设置的自定义响应头会返回给客户端（hop-by-hop 名称会被忽略）。

### 请求：`on_before_request` / `on_after_auth_request`

```javascript
{
  id: "request-id",
  body: "...",
  headers: {},
  url: "",
  model: "gpt-4",           // Gemini 原生为 path 模型名
  protocol: "openai_chat",
  source_format: "openai_chat",
  sourceFormat: "openai_chat",
  to_format: "codex",       // before 时为空
  toFormat: "codex",
  account_platform: "...",  // before 时为空
  mapped_model: "..."       // before 时为空
}
```

常见 `source_format`：`anthropic_messages`、`openai_chat`、`openai_responses`、`gemini_native`。

### `on_before_request` 说明

- 首次内容审核之后、账号选择 / 渠道映射之前执行**一次**。
- 故障转移**不会**重跑 before。
- 改写 body 后会**重新**内容审核。
- OpenAI 兼容与 Anthropic Messages 路径会从改写后的 body 刷新 **`model`** 与 **`stream`**（`stream` 缺失或类型非法时保留原值）。粘性会话哈希尽量使用改写后 body。
- Anthropic Messages 还会在改写后刷新 Claude Code / haiku 探测相关上下文。
- **OpenAI `/v1/messages`**：分组 before + 账号 after（Anthropic body）；协议转换后会在 Responses 或 Chat Completions body 上再跑一次账号 after。
- **OpenAI Responses WebSocket**：首包在 handler 跑分组 before，ingress parse 再跑账号 after；后续 turn 先分组 before、再账号 after（与 HTTP 顺序一致）。
- **Gemini 原生**（`/v1beta/...`）：model/action 来自 **URL path**；body 改写只影响载荷与粘性哈希，不影响 path 选路。

### 非流式 / 流式响应

与账号 after 钩子一致：支持 `req`、`headers`、`chunk`、`header_init`、`history_chunks`（最多 64）。改写 body 后会清除 **`Content-Length`** 由 Go 重算。

### 控制台

`console.log(...)` 会以 info 级别写入服务端日志（`jshandler console`）。

---

## 协议/路径覆盖

- Anthropic Messages（含 Antigravity 转发）
- OpenAI Chat Completions
- OpenAI Responses（HTTP + WebSocket 入口）
- Gemini 原生（`v1beta`）
- OpenAI ↔ Anthropic 兼容层

协议转换时会在转换后的 body 上**再次**调用 `on_after_auth_request`（如 Claude Messages → Gemini 为 `gemini_native`）。

---

## 多脚本链式执行

1. 解析有序 ID（去重，保留首次出现）。
2. 按顺序加载并执行钩子。
3. 将 `body` / `headers` / `chunk` 传给下一脚本。
4. 失败或缺失的脚本**跳过**，链继续。
5. 流式：若某脚本丢弃分片，链上后续脚本看不到该分片。

---

## 管理后台 UI

### 设置 → Gateway

启用 / 禁用、超时、脚本库（列表、上传、预览、编辑、删除）

### 分组

有序绑定 `on_before_request` 脚本

### 编辑账号

有序绑定 after-auth / 响应脚本；保存时写入 `extra.jshandler_script_ids` 与兼容字段 `jshandler_script_id`

---

## 管理端 HTTP API

基础路径（需管理员鉴权）：**`/api/v1/admin/gateway/jshandler`**  
响应信封：`{ "code": 0, "message": "...", "data": ... }`。

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/config` | 读取配置（`data.config` 为 `{enabled,timeout}` 的 JSON **字符串**） |
| `PUT` | `/config` | 请求体：`{ "enabled": bool, "timeout": "1s" }` |
| `GET` | `/scripts` | 列出脚本库 |
| `POST` | `/scripts` | multipart：`file`（必填，`.js`），可选 `name` |
| `GET` | `/scripts/:id` | 元数据 + `content` |
| `PUT` | `/scripts/:id` | JSON：`{ "name"?: string, "content"?: string }`（至少一个） |
| `DELETE` | `/scripts/:id` | 从索引与磁盘删除 |

分组创建/更新字段：`jshandler_script_ids`（更新时省略=不改动，空数组=清空）。

### 常见错误 reason

| Reason | 含义 |
|--------|------|
| `INVALID_SCRIPT_ID` | ID 格式非法 |
| `SCRIPT_NOT_FOUND` | 未知 ID |
| `EMPTY_SCRIPT_CONTENT` | 内容为空或仅空白 |
| `SCRIPT_TOO_LARGE` | 超出大小限制 |
| `NO_SCRIPT_CHANGES` | 更新无有效变更 |
| `JSHANDLER_UNAVAILABLE` | 服务未配置 |

上传限制：**512 KiB**。运行时加载拒绝超过 **8 MiB** 的文件。

---

## 错误处理与限制

| 项 | 行为 |
|----|------|
| 未定义钩子函数 | 空操作，保留输入 |
| 超时 / 编译 / 运行时错误 | 打警告日志，保留链上上一状态 |
| 热更新 | 脚本文件 mtime 变化时重新编译 |
| 配置缓存 | 约 60s TTL；管理端更新配置后失效 |
| 默认超时 | 每次钩子预算 `1s`（流式：会话建立后按 chunk 计） |

---

## 示例脚本

```javascript
function on_before_request(ctx) {
  try {
    var o = JSON.parse(ctx.body || "{}");
    if (o.model === "gpt-4") {
      o.model = "gpt-4o";
    }
    ctx.body = JSON.stringify(o);
  } catch (e) {}
  return ctx;
}

function on_after_auth_request(ctx) {
  try {
    var o = JSON.parse(ctx.body || "{}");
    o._js_source = ctx.source_format || "";
    ctx.body = JSON.stringify(o);
  } catch (e) {}
  return ctx;
}

function on_after_nonstream_response(ctx) {
  if (typeof ctx.body === "string" && ctx.body.indexOf("hello") >= 0) {
    ctx.body = ctx.body.replace(/hello/g, "hello-js");
  }
  return ctx;
}

function on_after_stream_response(ctx) {
  if (ctx.header_init) {
    return ctx;
  }
  if (typeof ctx.chunk === "string") {
    ctx.chunk = ctx.chunk.replace(/hello/g, "hello-js");
  }
  return ctx;
}
```

---

## 与 CPA 插件的差异

| 项 | CPA `cpa-plugin-jshandler` | Sub2API |
|----|----------------------------|---------|
| 配置 | YAML `script_paths[]` + timeout | 仅 DB 中 `enabled` + `timeout` |
| 谁执行脚本 | 全局路径列表，所有流量 | **分组** before + **账号** after/响应 |
| `on_before_request` | 支持 | **分组绑定并已接入** |
| 存储 | 任意路径 | `{data_dir}/jshandler/` 注册表 |
| 内置示例 | 插件 `scripts/` | 无，需自行上传 |
| 管理 | 配置文件 | REST + 设置页 + 分组/账号 UI |
| 流式 history | 完整历史 | 最多 **64** 个 chunk |

仓库中的 `cpa-plugin-jshandler/`（若存在）仅作**参考**，网关不会加载它。

---

## 相关代码

- 引擎 / 钩子：`backend/internal/service/jshandler/`
- 分组绑定：`backend/internal/service/jshandler_group.go`
- 账号 extra：`backend/internal/service/jshandler_account.go`
- 网关接线：`backend/internal/handler/jshandler_openai.go`
- 管理 API：`backend/internal/handler/admin/jshandler_handler.go`
- 迁移：`backend/migrations/173_add_group_jshandler_script_ids.sql`
- 前端：设置页 Gateway 卡片 + 分组表单 + `EditAccountModal`
