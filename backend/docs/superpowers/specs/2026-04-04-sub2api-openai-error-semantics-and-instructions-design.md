# sub2api OpenAI 错误语义与 Instructions 兼容设计

## 背景

当前 `sub2api` 在 OpenAI `/v1/responses` 链路上有两类已经被验证的问题：

1. **非 passthrough 正式路径会把上游很多结构化 4xx 错误错误地包装成 502。**
   以 `invalid image` 为例，上游原始错误是类似：

   ```json
   HTTP 400
   {
     "error": {
       "code": "invalid_value",
       "message": "The image data you provided does not represent a valid image.",
       "param": "input",
       "type": "invalid_request_error"
     }
   }
   ```

   但当前 `sub2api` 非 passthrough 路径会把它包装成：

   ```json
   HTTP 502
   {
     "error": {
       "type": "upstream_error",
       "message": "Upstream request failed"
     }
   }
   ```

   这会把一个本来不该重试的上游 400 伪装成可重试的 5xx，影响 OpenCode 的错误识别与 backoff 行为。

2. **`instructions` 兼容策略当前会稀释 OpenCode 的真实系统提示词语义。**
   OpenCode 对 GPT-5 家族模型会使用 `codex_header.txt` 作为 rich system prompt：
   - 内置 `openai + oauth` provider 走顶层 `instructions`
   - 自定义 provider / Copilot provider 走普通 system message

   但 `sub2api` 当前在非 passthrough 路径里，如果请求体顶层 `instructions` 为空，会直接补：

   ```go
   reqBody["instructions"] = "You are a helpful coding assistant."
   ```

   这会让 OpenCode 已经通过 system message 发送的 rich prompt，再叠加一个过弱的 fallback `instructions`，有语义降级/污染风险。

3. **passthrough 路径有另一条独立兼容性问题。**
   对 OpenAI OAuth 上游，passthrough 直连时 `gpt-5.4` 也会因为缺少顶层 `instructions` 被上游直接拒绝，返回：

   ```json
   HTTP 400
   {"detail":"Instructions are required"}
   ```

   这说明 `passthrough + OpenCode 自定义 provider` 当前在请求兼容性上不成立。

## 目标

本次设计同时解决两件事：

1. 修正 `sub2api` 对 OpenAI 上游结构化错误的错误包装，让客户端（尤其是 OpenCode）能看到正确的状态码和完整的 upstream 错误语义。
2. 修正 `instructions` 兼容逻辑，避免把 OpenCode 的 rich system prompt 稀释成一个过弱 fallback，同时让 passthrough 路径也具备基础兼容性。

## 非目标

本次不做以下事情：

1. 不在这一轮里实现“精准定位哪张坏图并自动清理”。
2. 不试图让 passthrough 与非 passthrough 在所有行为上完全统一。
3. 不在这轮里改 OpenCode 本身的系统提示词策略。
4. 不对所有 OpenAI 错误做彻底重构，只聚焦当前已验证会影响错误识别和请求兼容性的关键路径。

## 设计概览

本次方案分两条线推进，但共用同一原则：

1. **错误语义要更接近上游原文，同时明确区分这是 upstream 错误。**
2. **`instructions` 要尽量从 OpenCode 已经提供的 rich prompt 中提炼/提升，而不是再硬塞一个弱 fallback。**

---

## 设计第 1 节：错误返回契约

### 目标行为

对于 OpenAI 非 passthrough 正式路径下、上游返回的结构化错误：

1. HTTP 状态码直接沿用上游。
2. 返回体仍然由 `sub2api` 组织，而不是裸抛上游 body。
3. 但返回体必须同时满足：
   - 明确标记这是 `upstream` 错误；
   - 完整保留 upstream 原始错误语义（至少包括 `code`、`type`、`message`、`param` 和可选的原始 body 文本）。

### 建议返回形态

推荐统一成类似结构：

```json
HTTP 400
{
  "error": {
    "type": "upstream_error",
    "message": "The image data you provided does not represent a valid image.",
    "upstream": {
      "status": 400,
      "code": "invalid_value",
      "type": "invalid_request_error",
      "param": "input",
      "message": "The image data you provided does not represent a valid image.",
      "raw": {
        "error": {
          "code": "invalid_value",
          "message": "The image data you provided does not represent a valid image.",
          "param": "input",
          "type": "invalid_request_error"
        }
      }
    }
  }
}
```

### 原则

1. `sub2api` 外壳保持稳定，方便客户端知道错误来源不是本地 schema 校验，而是 upstream。
2. upstream 细节必须完整保留，避免把 400 客户端输入错误伪装成 502 通用网关错误。
3. 仅当上游 body 不可解析或根本没有结构化错误时，才继续走当前的通用包装逻辑。

---

## 设计第 2 节：Instructions 兼容策略

### 当前问题

OpenCode rich prompt 并没有丢失，而是：

1. 自定义 provider / Copilot provider 会把 `PROMPT_CODEX` 作为普通 system message 发送；
2. `sub2api` 又会在顶层 `instructions` 为空时补一个过弱 fallback；
3. passthrough 直连 OpenAI OAuth 时，上游还会直接要求非空 `instructions`。

### 目标行为

透传和非透传都要改，但重点优先修非 passthrough：

1. **非 passthrough 优先**
   - 不再直接补 `"You are a helpful coding assistant."` 这种弱 fallback；
   - 如果顶层 `instructions` 为空，应优先从现有 system message 中提炼/提升出一份与 OpenCode rich prompt 同源的内容；
   - 这样顶层 `instructions` 与 system message 至少语义同源，而不是两套相互竞争的系统提示词。

2. **passthrough 同步兼容**
   - 复用同一套“从 system message 提炼/提升出 `instructions`”逻辑；
   - 避免非 passthrough 和 passthrough 在 `instructions` 语义上再次分叉。

### 设计原则

1. 不把 OpenCode rich prompt 降级成弱 fallback。
2. 不要求 OpenCode 为了兼容 `sub2api` 去改变 provider id 或 prompt 生成逻辑。
3. `sub2api` 应尽量吸收这层兼容性差异。

---

## 设计第 3 节：作用范围与例外

### 本轮明确要覆盖

1. OpenAI `/v1/responses` 非 passthrough 正式路径的结构化上游错误返回。
2. OpenAI passthrough OAuth 路径的 `instructions` 兼容。
3. OpenCode 当前最敏感的客户端行为：
   - 不把 400 invalid image 误判成可重试 502；
   - 不让 rich prompt 被弱 fallback 稀释。

### 本轮明确不覆盖

1. 自动定位并删除坏图。
2. OpenCode 本身的会话状态修复逻辑。
3. 对所有 OpenAI 错误做全量统一重构。

---

## 验证方案

验证分三层：

### A. 后端错误语义验证

验证非 passthrough 路径下：

1. 上游 400 结构化错误不再被包装成 502。
2. 返回体同时包含 `sub2api` upstream 外壳和完整 upstream 明细。
3. 原先的 5xx/网络类网关错误仍保持现有包装。

### B. Instructions 兼容验证

验证：

1. 非 passthrough 路径下，空 `instructions` 时不再补弱 fallback，而是补同源 rich prompt。
2. passthrough + OAuth + `gpt-5.4` 请求，不再因为缺 `instructions` 直接报 `Instructions are required`。

### C. OpenCode 兼容性验证

至少验证两件事：

1. 对坏图错误，OpenCode 不再因为 `sub2api` 返回的 502 通用包装而进入错误 backoff。
2. `gpt-5.4` / `gpt-5.4-Sys` 请求的系统提示词语义没有被弱 fallback 稀释。

---

## 风险与控制

### 风险 1：客户端依赖当前的旧错误体字段

控制：

1. 保留 `sub2api` 外壳，不直接裸抛 upstream body。
2. 新增 `upstream` 明细字段，而不是删除现有错误对象。

### 风险 2：instructions 提炼逻辑改变模型行为

控制：

1. 只在顶层 `instructions` 为空时补齐。
2. 优先从现有 system message 中提升，而不是重新生成另一套提示词。

### 风险 3：passthrough 与非 passthrough 再次分叉

控制：

1. 两条路径共用同一套 `instructions` 补齐逻辑。
2. 仅保留错误返回行为上的必要差异，不再复制 prompt 兼容逻辑。

## 最终结论

本次设计的核心不是“把错误都裸透传”，而是：

1. **状态码跟上游走；**
2. **错误体保留 `sub2api` 外壳；**
3. **外壳里完整保留 upstream 原始语义；**
4. **同时修正 `instructions` 兼容，让 rich prompt 不被弱 fallback 污染。**

按这个方向落地后，可以同时解决：

1. OpenCode 对 invalid image 被误导成可重试 502 的问题；
2. passthrough + OAuth 上游的 `Instructions are required` 问题；
3. 非 passthrough 路径里 rich prompt 被弱 fallback 稀释的问题。
