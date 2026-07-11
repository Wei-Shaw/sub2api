# 网关 JavaScript 钩子（JS Handler）

Sub2API 可在网关上按**账号**执行 JavaScript 脚本，改写请求与响应载荷。脚本运行在嵌入式 [Goja](https://github.com/dop251/goja) 虚拟机中，带固定超时，失败时保持原数据（fail-open）。

本文描述 **sub2api 原生**模型（脚本库 + 账号绑定）。钩子形态受上游 [cpa-plugin-jshandler](https://github.com/router-for-me/cpa-plugin-jshandler) 启发，但配置与生效方式不同。

英文版见 [JSHANDLER.md](./JSHANDLER.md)。

---

## 目录

- [快速开始](#快速开始)
- [生效条件](#生效条件)
- [存储布局](#存储布局)
- [钩子 API](#钩子-api)
- [协议/路径覆盖](#协议路径覆盖)
- [多脚本链式执行](#多脚本链式执行)
- [管理后台 UI](#管理后台-ui)
- [管理端 HTTP API](#管理端-http-api)
- [错误处理与限制](#错误处理与限制)
- [示例脚本](#示例脚本)
- [与 CPA 插件的差异](#与-cpa-插件的差异)

---

## 快速开始

1. **管理后台 → 设置 → Gateway** → 打开 **Gateway JavaScript hooks**。
2. 打开 **启用**，设置 **超时**（默认 `1s`，如 `500ms`、`2s`）。
3. 向脚本库 **上传** `.js` 文件（最大 **512 KiB**）。
4. **编辑账号** → 从脚本库绑定一个或多个脚本（**顺序有意义**）。
5. 用该账号所属分组/密钥发请求；**仅绑定了脚本的账号**会执行钩子。

以下情况钩子**不会**执行：

- 全局配置 `enabled` 为 `false`，或
- 选中账号的 `extra` 中没有 `jshandler_script_id` / `jshandler_script_ids`。

---

## 生效条件

| 层级 | 机制 |
|------|------|
| 全局 | 设置项 `jshandler_config`：`{ "enabled": bool, "timeout": string }` |
| 账号 | `extra.jshandler_script_ids`（有序列表，推荐）与兼容字段 `extra.jshandler_script_id`（单个字符串） |
| 运行时 | 按 ID 从脚本库加载 → 按文件 mtime 编译/缓存 → 调用钩子 |

**没有**目录自动扫描，也**没有**全局 `script_paths` 列表。可执行脚本必须先上传到脚本库，再绑定到账号。

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

脚本定义**全局函数**（不是 `module.exports`）。生产环境仅接入下列钩子：

| 函数 | 调用时机 |
|------|----------|
| `on_after_auth_request(ctx)` | 账号选定之后、请求体转发上游之前 |
| `on_after_nonstream_response(ctx)` | 非流式上游完整响应组装之后 |
| `on_after_stream_response(ctx)` | 每个流式数据载荷（以及一次合成的 header-init 调用） |

### 生产环境不可用

- **`on_before_request`**：仅存在于上游 CPA 插件与单元测试中。Sub2API 请求路径一律调用 `on_after_auth_request`。

### 返回值

- 返回修改后的 **对象**（与 `ctx` 同形），设置 `body` / `chunk` / `headers`。
- 或返回 **字符串**，仅替换 `body`（请求/非流式）或 `chunk`（流式）。
- `headers[name] = null` 或 `[]` 表示删除该头。
- 流式：`chunk` 返回**空字符串**可**丢弃**该分片，不发给客户端。

**请求头：** 修改会写回入站 Gin 请求。是否真正发到上游取决于各协议路径如何组装上游 Header（Anthropic/OpenAI 等多条路径仅**白名单**转发，因此像 `X-JSHandler-Req` 这类自定义头即使钩子已执行，也可能到不了上游）。需要稳定、与供应商无关的效果时，优先改 **`body`**。

**响应头：** 非流式/流式钩子设置的自定义响应头会返回给客户端（hop-by-hop 名称会被忽略）。

### 请求：`on_after_auth_request(ctx)`

```javascript
{
  id: "request-id",
  body: "...",              // 请求体字符串
  headers: {},              // 请求头（string 或 string[]）
  url: "",                  // sub2api 中恒为空
  model: "gpt-4",           // 客户端 / 原始模型名
  protocol: "openai_chat",  // 与 source_format 相同
  source_format: "openai_chat",
  sourceFormat: "openai_chat",
  to_format: "codex",       // 推断的上游协议族
  toFormat: "codex",
  account_platform: "...",  // 可知时提供
  mapped_model: "..."       // 可知时提供
}
```

常见 `source_format`：`anthropic_messages`、`openai_chat`、`openai_responses`、`gemini_native`。

### 非流式响应：`on_after_nonstream_response(ctx)`

```javascript
{
  id: "request-id",
  body: "...",              // 完整响应体
  req: { body: "...", headers: {}, url: "" },
  protocol: "openai_chat",
  headers: {},              // 响应头
  chunk: null,
  history_chunks: null
}
```

改写 body 后，脚本设置的 hop-by-hop 头会被忽略，且会在 writer 上**始终清除 `Content-Length`**，由 Go 按改写后的 body 重新设置（客户端仍可能看到重新计算后的 `Content-Length`）。

### 流式响应：`on_after_stream_response(ctx)`

```javascript
{
  id: "request-id",
  body: null,
  req: { body: "...", headers: {}, url: "" },
  protocol: "openai_chat",
  headers: {},              // 响应头（可改）
  chunk: "...",             // 当前数据载荷（不是完整 SSE 帧）
  header_init: false,       // 合成的首次调用为 true
  history_chunks: []        // 冻结的字符串数组，最近最多 64 个载荷
}
```

说明：

- **Anthropic SSE**：`chunk` 仅为 `data:` 中的 JSON；重建块时保留原 `event:` 名。
- **OpenAI SSE**：`chunk` 为 data 行 JSON；空行 / `[DONE]` 不进入钩子。
- **Header-init**：在首个真实载荷前调用一次，`chunk` 为空且 `header_init: true`，可只改响应头而不输出分片。
- 同一流会话内 VM **状态跨 chunk 保留**（可用计数器、缓冲等）。

### 控制台

`console.log(...)` 会以 info 级别写入服务端日志（`jshandler console`）。

---

## 协议/路径覆盖

账号选定后，主要网关路径都会跑请求钩子，包括：

- Anthropic Messages（含 Antigravity 转发）
- OpenAI Chat Completions
- OpenAI Responses（HTTP + WebSocket 入口）
- Gemini 原生（`v1beta`）
- OpenAI ↔ Anthropic 兼容层

非流式与流式响应钩子在网关持有上游响应的对应路径上同样接入。

### 协议转换时的第二次请求钩子

当网关把 body **重新编码**为另一种上游形态时，会对转换后的 body **再次**调用 `on_after_auth_request`，使脚本看到实际上游载荷：

| 转换 | 第二次 `source_format` |
|------|------------------------|
| Claude Messages → Gemini | `gemini_native` |
| OpenAI Responses → Chat 回退 | `openai_chat` |

### 内容审核

若请求钩子修改了 body，转发前会对改写后的 body **重新执行**内容审核。

---

## 多脚本链式执行

1. 从 `extra.jshandler_script_ids` 解析有序 ID（去重，保留首次出现）。
2. 按顺序加载脚本库中的脚本并执行钩子。
3. 将 `body` / `headers` / `chunk` 传给下一脚本。
4. 失败或缺失的脚本**跳过**，链继续。
5. 流式：若某脚本丢弃分片（`DropChunk`），链上后续脚本看不到该分片。

---

## 管理后台 UI

### 设置 → Gateway

- 启用 / 禁用
- 超时时间
- 脚本库：列表、上传 `.js`、预览（高亮）、编辑名称/源码、删除

### 编辑账号

- 从脚本库有序多选（添加、删除、调序）
- 保存时：
  - `extra.jshandler_script_ids = [...]`
  - `extra.jshandler_script_id = 第一个 id`（兼容旧字段）
  - 未选任何脚本则删除上述两个键

---

## 管理端 HTTP API

基础路径（需管理员鉴权）：**`/api/v1/admin/gateway/jshandler`**  
（具体前缀以部署时的 API 挂载为准；路由注册在 admin 组下的 `/gateway/jshandler`。）

响应信封：`{ "code": 0, "message": "...", "data": ... }`（`code: 0` 表示成功）。

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/config` | 读取配置（`data.config` 为 `{enabled,timeout}` 的 JSON **字符串**） |
| `PUT` | `/config` | 请求体：`{ "enabled": bool, "timeout": "1s" }` |
| `GET` | `/scripts` | 列出脚本库 |
| `POST` | `/scripts` | multipart：`file`（必填，`.js`），可选 `name` |
| `GET` | `/scripts/:id` | 元数据 + `content` |
| `PUT` | `/scripts/:id` | JSON：`{ "name"?: string, "content"?: string }`（至少一个） |
| `DELETE` | `/scripts/:id` | 从索引与磁盘删除 |

### 常见错误 reason

| Reason | 含义 |
|--------|------|
| `INVALID_SCRIPT_ID` | ID 格式非法 |
| `SCRIPT_NOT_FOUND` | 未知 ID |
| `EMPTY_SCRIPT_CONTENT` | 内容为空或仅空白 |
| `SCRIPT_TOO_LARGE` | 超出大小限制 |
| `NO_SCRIPT_CHANGES` | 更新无有效变更（如仅空名称） |
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
function on_after_auth_request(ctx) {
  // 调试：给 body 打上 source_format 标记
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
    // 可选：仅调整响应头
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
| 谁执行脚本 | 全局路径列表，所有流量 | **按账号**绑定脚本库 ID |
| `on_before_request` | 支持 | **未接入** |
| 存储 | 任意路径 | `{data_dir}/jshandler/` 注册表 |
| 内置示例 | 插件 `scripts/` | 无，需自行上传 |
| 管理 | 配置文件 | REST + 设置页 + 账号 UI |
| 流式 history | 完整历史 | 最多 **64** 个 chunk |
| 额外 ctx | 格式 / 模型 | 另有 `account_platform`、`mapped_model`、流式 `header_init` |

仓库中的 `cpa-plugin-jshandler/`（若存在）仅作**参考**，网关不会加载它。

---

## 相关代码

- 引擎 / 钩子：`backend/internal/service/jshandler/`
- 网关接线：`backend/internal/service/gateway_jshandler.go`、`backend/internal/handler/jshandler_*.go`
- 管理 API：`backend/internal/handler/admin/jshandler_handler.go`
- 账号 extra 字段：`backend/internal/service/jshandler_account.go`
- 前端：设置页 Gateway 卡片 + `EditAccountModal` 脚本绑定
