# sub2api 未分组 Key OpenAI 全局池设计

日期：2026-04-03

## 背景

当前 `sub2api` 的 OpenAI 兼容入口是否走 `OpenAIGateway`，主要由 `API key -> group.platform` 决定。

这带来两个直接限制：

1. 未分组 API key 即使允许调度，也不会天然等价于“访问全部 OpenAI 账号”。
2. `groupID == nil` 时，OpenAI 账号选择走的是“未分组账号池”，不是“全部 OpenAI 账号池”。

在当前实例里，管理员只打算接入 OpenAI，不打算接入 Anthropic/Gemini/Sora 等其他平台；因此希望提供一种纯运行时可切换的模式，让**未分组 API key** 可以直接访问**全部 OpenAI 账号**，同时不修改数据库结构，不引入隐藏自动绑组逻辑。

## 已验证事实

### 现状实现

- `backend/internal/server/routes/gateway.go`
  - `/v1/messages`、`/v1/responses`、`/v1/chat/completions` 等入口会依据 `group.platform` 决定是否走 `OpenAIGateway`
- `backend/internal/service/openai_gateway_service.go`
  - `groupID == nil` 时，OpenAI 账号选择走未分组账号查询分支，而不是全平台查询分支
- `backend/internal/service/setting_service.go`
  - 现有 `allow_ungrouped_key_scheduling` 只表示“允许未分组 key 调度未分组账号”，不表示“访问全部 OpenAI 账号”

### 现场验证结论

对当前实例做过一次可逆实验：

1. 打开 `allow_ungrouped_key_scheduling`
2. 将测试 API key 解绑为未分组
3. 将 OpenAI 账号 `64/65/66` 全部解绑为未分组
4. 使用该 key 发一条标准 `POST /v1/responses` OpenAI 请求

结果：

- 请求直接返回 `503`
- `usage_logs` 无记录
- access log 中无 `account_id`
- 没有出现 `handler.openai_gateway.responses` 的选中账号痕迹

结论：**“把 key 和账号都扔进未分组”在当前实现里并不会变成“访问全部 OpenAI 账号”**。

## 目标

新增一个**后台 settings 开关**，开启后满足以下行为：

1. 当 API key 未分组时，请求级别可以被解析为 `openai` 平台。
2. 所有现有的 OpenAI 兼容入口，都能自然继承这一解析结果，而不是每个 endpoint 手写特判。
3. 当未分组 key 被解析为 OpenAI 时，账号选择走**全部 OpenAI 账号池**，而不是未分组账号池。
4. 关闭开关时，系统完全保持当前默认行为。

## 非目标

本次不做以下事情：

1. 不修改 `api_keys` 表结构，不新增 `default_platform` 之类字段。
2. 不伪造数据库中的 group 关系。
3. 不删除或阉割现有 group 体系。
4. 不改变 Anthropic/Gemini/Antigravity/Sora 相关专用路由行为。
5. 不改变非未分组 key 的现有行为。

## 设计结论

### 1. 新增后台设置

新增一个布尔型 setting，暂定名：

- `openai_global_pool_for_ungrouped_keys`

语义：

- `false`：保持现状
- `true`：对未分组 API key 启用“OpenAI 全局池”语义

该开关放在现有后台 settings 体系中，而不是 `config.yaml` / env，也不新增任何数据库字段到 `api_keys`。

### 2. 引入 request-scoped effective platform

在 API key 鉴权之后，为当前请求解析一个**请求级平台值**，记为 `effective platform`。

解析优先级：

1. 若已有 `ForcePlatform`，优先使用 `ForcePlatform`
2. 否则若 API key 绑定了真实 group，使用 `apiKey.Group.Platform`
3. 否则若 API key 未分组且 `openai_global_pool_for_ungrouped_keys == true`，返回 `openai`
4. 否则为空字符串，保持当前行为

关键点：

- 这是**请求上下文中的派生值**
- 不改数据库
- 不伪造 `apiKey.Group`
- 不改变 `GroupID == nil` 这一真实身份语义

### 3. 平台分流统一改读 effective platform

所有当前依赖 `group.platform` 做 OpenAI/非 OpenAI 分流的兼容入口，都应改为读取 `effective platform`。

这意味着不需要按协议名称手工枚举逻辑分支，而是统一修改“平台解析源”。

需要覆盖的现有入口包括：

1. `POST /v1/messages`
2. `POST /v1/messages/count_tokens`
3. `POST /v1/responses`
4. `POST /responses`
5. `POST /v1/chat/completions`
6. `POST /chat/completions`
7. `GET /v1/models`

预期表现：

- 若 `effective platform == openai`，这些入口应像当前“OpenAI 分组 key”一样走 OpenAI 相关路径
- `/v1/messages/count_tokens` 在 OpenAI 下应继续返回现有 404 行为
- `/v1/models` 应返回 OpenAI 模型视图与 `-Sys` 别名

### 4. 账号选择层改为“未分组 key + 开关开 => 全部 OpenAI 账号池”

当前问题的根源之一是：`groupID == nil` 在 OpenAI 账号选择层里等价于“未分组账号池”。

本次需要改成：

- `groupID != nil`：保持原有按组查询
- `groupID == nil` 且开关关闭：保持原有未分组账号查询
- `groupID == nil` 且开关开启，并且 `effective platform == openai`：改为查询**全部 OpenAI 账号**

这条语义必须同时覆盖：

1. 普通 active 请求
2. exhausted 目标组请求
3. `-Sys` 请求
4. Claude 协议映射到 OpenAI 的入口

## 为什么不用 fake group

讨论过程中明确放弃了“在请求里直接伪造 `apiKey.Group.Platform=openai`”的做法。

原因：

1. `Group` 在现有代码中不只是 `platform`，还承载 `group_id`、模型路由、倍率、fallback 等语义。
2. 许多代码先判断 `apiKey.Group != nil`，再顺手读取 `Group.ID`；如果伪造一个内存 group，会把 `groupID=0` 这类假身份污染进查询、缓存键和逻辑分支。
3. 这比单独引入 `effective platform` 更隐蔽，也更难维护。

因此本设计明确采用：

- 平台语义派生到请求上下文
- group 身份语义仍由真实 `GroupID` 决定

## 代码改动边界

目标是最小改动，预估只碰以下几类位置：

### A. setting 面

- setting 常量
- settings 读取/写入
- 管理后台设置 DTO / handler

### B. 平台解析面

- 提供统一的 `effective platform` 解析 helper
- 让依赖 `group.platform` 的分流逻辑改读该 helper

### C. handler 面

- 兼容入口的路由分流
- `GET /v1/models` 的平台视图

### D. OpenAI service 选择面

- 将“未分组 + 开关开”的分支改到全部 OpenAI 账号池
- 保持 active/exhausted / `-Sys` 现有语义不变

## 风险点

1. 不能破坏已有“已分组 key 按真实 group.platform 分流”的行为。
2. 不能让 Anthropic/Gemini/Antigravity/Sora 路由被意外卷入。
3. `GET /v1/models` 不能在该模式下错误地列出非 OpenAI 模型白名单。
4. OpenAI 账号选择切换到“全平台账号池”后，不能破坏 active/exhausted 与 `-Sys` 既有路由语义。

## 验证方案

### 单元/服务验证

1. 新增 `effective platform` 解析测试
2. 覆盖开关开/关、未分组/已分组、ForcePlatform 覆盖优先级
3. 覆盖 OpenAI 账号选择在 `groupID == nil` 下的两种行为：
   - 开关关：未分组账号池
   - 开关开：全部 OpenAI 账号池
4. 保持 active/exhausted / `-Sys` 语义回归

### handler / 路由验证

覆盖以下入口在“未分组 key + 开关开”下的行为：

1. `/v1/messages`
2. `/v1/messages/count_tokens`
3. `/v1/responses`
4. `/responses`
5. `/v1/chat/completions`
6. `/chat/completions`
7. `/v1/models`

### 现场验证

在真实运行实例上做一轮可逆验证：

1. 开启后台 setting
2. 使用未分组 API key
3. 将 OpenAI 账号保持未分组
4. 用 Claude 兼容协议打一条请求
5. 用 OpenAI 兼容协议打一条请求
6. 确认两者都能命中 OpenAI 账号
7. 再关闭开关，确认行为恢复

## 取舍结论

这份设计选择的是：

- 不做数据库 schema 变更
- 不做 fake group
- 不删除 group 体系
- 只补一个后台 setting
- 只新增一个 request-scoped `effective platform`
- 在 `groupID == nil` 的 OpenAI 账号选择分支上做最小修正

这是在当前讨论范围内，概念负担最小、行为最明确、回滚成本也最低的方案。
