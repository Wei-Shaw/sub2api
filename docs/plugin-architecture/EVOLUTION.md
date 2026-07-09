# 插件体系演进路线（Evolution）

> 本文档记录插件体系的**战略决策**与**演进路线**：为什么是当前架构、未来往哪走、
> 哪些事有意不做。与 [BUILTIN-PLUGIN-GUIDE.md](./BUILTIN-PLUGIN-GUIDE.md)（内建层
> 作者指南）配套。日常插件开发读指南即可；做架构决策、评估"能不能拆成插件"时读本文。

---

## 一、双层机制的决策记录

### 1.1 为什么是"两档"而不是"一档"

内建层（`internal/pluginkit`）与外部层（`internal/pluginhost`）**不是两套冗余机制，
而是同一套插件模型的两个信任/能力档位**：

| 维度 | 内建层 (`pluginkit`) | 外部层 (`pluginhost`) |
|---|---|---|
| 形态 | 源码进 `builtin.go` 清单，编译进宿主 | zip 上传 → 子进程二进制 |
| 通信 | 进程内函数调用 | HTTP/1 over unix socket（数据面）+ 127.0.0.1 能力面（控制面） |
| 能力 | 全库 DB / 全 Redis / 受限路由面 | 仅 KV / Log / Config，硬隔离 |
| 信任 | 完全可信（自己的代码） | 半可信（管理员自装自担） |
| 命名空间 | 点分 ID（`demo`、`job.backup`），两层**共享**，冲突拒装 | 同左 |
| 业界对应 | Caddy / CoreDNS 模块（编译期装配） | HashiCorp go-plugin（Terraform / Vault 子进程模型） |

- 内建插件是自己的代码，要的是"能力全开 + 运行时启停开关"（把存量功能域整体
  竖切出核心，如内容审计）。
- 外部插件是别人上传的二进制，要的是"最小能力面 + 进程隔离 + 崩溃不拖垮宿主"。
- 两档用**同一套契约**（`Plugin` + 可选 `Initializer`/`Runner`/`APIProvider`）、
  **同一个启停状态机**（DB `plugin_states` 表 + Redis 广播）、**同一套前端四扩展点**
  （路由/菜单/i18n/设置面板）统一驱动。
- Go 语言层面没有可靠的动态加载（`plugin` .so 包公认是坑：编译器版本/依赖必须
  逐字节一致、无法卸载），"编译期源码装配 + 子进程 RPC"双轨就是 Go 生态的传统
  正确方案（Grafana / Mattermost 同构）。

### 1.2 传输层：为什么是 HTTP/1 over unix socket 而不是 gRPC

- PROPOSAL 原案曾选 HashiCorp go-plugin 的 gRPC；Phase-4 实现时改为
  **HTTP/1 over unix socket**，理由成立且**维持不改**：
  - 插件对外提供的本来就是 HTTP API（宿主经 `:id` 分发器反向代理转发），
    HTTP/1 天然支持 SSE 流式（反代 `FlushInterval: -1` 即可）；
  - 换 gRPC 要在插件 HTTP 与宿主之间加一层协议翻译，且强制非 Go 插件引入
    protoc 工具链，得不偿失。
- `manifest.protocol` 字段已版本化（当前唯一合法值 `"http/1"`）。未来若真需要
  gRPC（强类型 RPC、双向流），可新增 `"grpc"` 档而不破坏现有插件。

---

## 二、平台适配插件化路线（Provider Registry）

### 2.1 现状与硬边界

- 平台枚举定义在 `backend/internal/domain/constants.go`：`anthropic` / `openai` /
  `gemini` / `antigravity` / `grok`。
- **97 个非测试 Go 文件**引用平台常量；仅 service 层就有数十个文件按平台
  `switch`/`if` 分发。平台专属逻辑与共享管线深度交织。

**硬性结论：**

1. **热路径是禁区**：网关转发（SSE 长流）+ 计费同进程事务结算，任何插件机制
   不得进入；外部层能力面刻意不提供网关/计费扩展点。
2. **外部（子进程）层永远做不了平台适配**：能力面只有 KV/Log/Config，够不到
   账号/调度/计费/上游 client；跨进程做转发引入一致性风险，违背零回归。
3. **内建层理论上能承载，但当前没有接缝**——需要先在核心建缝（见下）。

### 2.2 平台分发点清单（建缝目标）

按关注点归类的主要 `switch platform` 分发点（Step 1 的 ports 抽象对象）：

| 关注点 | 代表文件（`internal/service/`） | 未来接口形态 |
|---|---|---|
| 转发管线选择 | `gateway_forward.go`、`openai_gateway_forward.go`、`gateway_forward_as_responses.go`、`gateway_forward_as_chat_completions.go` | `Forwarder` |
| 上游请求构造 | `gateway_upstream_request.go`、`gateway_request.go` | `UpstreamRequestBuilder` |
| 用量解析 | `gateway_service.go`、`openai_gateway_service.go` 内的 usage 提取 | `UsageParser` |
| OAuth/token 刷新 | `claude_token_provider.go`、`gemini_token_provider.go`、`gemini_token_refresher.go`、`openai_*` 对应物 | `TokenRefresher` |
| 账号 CRUD 校验 | `account_service.go`、`admin_account.go` | `AccountValidator` |
| 调度/限流 | `gateway_scheduling.go`、`openai_account_scheduler.go`、`model_rate_limit.go`、`gemini_quota.go` | `Scheduler` 挂钩点 |
| 模型列表/可用性 | `openai_gateway_model_availability.go` 等 | `ModelLister` |
| 账号同步 | `crs_sync_service.go` | `AccountSyncer` |

### 2.3 四步路线

- **Step 0（PR #3832，已完成）**：不碰网关代码，只打地基——修内核缺陷
  （panic 隔离、路径归一化、stdin 握手、permissions enforce 等），本路线入库。
- **Step 1（核心重构，独立后续 PR 系列）**：把"平台适配"抽成一组窄 ports 接口
  （按上表关注点拆），核心管线改为**按 platform 字符串查注册表分发**；现有 5 个
  平台作为 in-tree 适配器原地注册，**零行为变更**（用特征化测试 + benchstat CI
  gate 守护）。
- **Step 2**：注册表对内建插件开放挂载点（Host 增加一个受评审登记的注册能力），
  新平台 = 一个内建插件（`platform.<name>` 命名空间），编译进但可运行时启停。
  `content-moderation` 的"原子句柄"模式已证明热路径挂钩点可以被内建层竖切。
- **Step 3（远期评估）**：存量平台逐个迁出为内建插件。**共享管线
  （调度/计费/粘性会话）永久留核心。**

一句话：账号平台适配是"改核心的建缝工程"，不是"加插件"。前置的 ports 抽象和
switch 改造要单独立项、按核心重构的节奏排期。

---

## 三、内建插件独立开源仓库路线（Caddy 模式）

**可行，但有两个硬前提；现在不做，契约冻结后再启动。**

### 3.1 硬前提

- **A. 契约迁出 `internal/`**：`internal/pluginkit` 无法被外部仓库 import。要让
  第三方仓库实现 `Plugin` 契约，契约包必须迁到可导出位置（如 `backend/pluginkit/`）。
- **B. 模块可 `go get`**：`go.mod` 声明 `module github.com/Wei-Shaw/sub2api` 但物理
  位于 `backend/` 子目录，模块本身无法被 `go get`。解决方案二选一：
  - 推荐：把插件契约 + SDK 抽成**独立可发布模块**（如 `sub2api-plugin-sdk` 仓库），
    宿主与外部插件都依赖它；
  - 或调整主模块路径使其可 `go get`（全仓 import 重写，侵入大，不推荐）。

### 3.2 连带约束

- 当前 `Host` 暴露 `*ent.Client`（宿主生成代码），独立仓库的插件拿不到该类型。
  契约模块的能力面必须收敛成**窄接口**（插件需要什么业务能力，先在核心抽 ports
  再挂载），与 PROPOSAL"永不暴露具体 Service"的纪律一致。

### 3.3 装配模型与节奏

- 每个内建插件一个独立 Go module；宿主 `builtin.go` 里 `import` 并在 `go.mod`
  锁版本。远期可加 xcaddy 式构建器（按清单拼装 + 编译定制二进制）。
- **先让契约在 in-tree 插件上跑稳**（各功能域竖切期间持续打磨接口），契约冻结后
  再抽 SDK 模块。现在就抽 = 把不稳定接口发布出去，此后每次改契约都是外部生态的
  破坏性变更。

---

## 四、已知项清单（有意不修 / 域级跟进）

以下问题经审计评估后**有意不在内核层修复**，记录在案防止重复审计：

| 项 | 原因 |
|---|---|
| procgroup `Kill(-pid)` PID 复用竞态 | 亚毫秒窗口，概率极低；修复需引入 pidfd 等平台专属机制，收益不成比例 |
| KV 键数配额 / log 限流 | 待外部生态起量再做；当前外部插件均为管理员自装自担 |
| assets 端点枚举 oracle | 设计如此（静态资源需匿名可达）；Content-Type 白名单 + nosniff 已挡 XSS |
| moderation Start-fail fail-open | 该插件 Start 仅内存分配，几乎不失败；域级跟进 |
| moderation disable 丢队列尾巴 | 域级跟进（内容审计域的优雅排空） |
