# V4 Inspector 全局能力调研

> 作用：盘点 host 已实现 / plugin 还缺失的"全局能力"，为 V4 Curator 决策做基础。
> 范围：只调查、不决策、不改代码。

base: `8080ff43 Merge V3 curator` (`feat/plugin-system-fixes`)

---

## 1. 前端 — Tailwind tokens 与全局样式

### 1.1 Host 现状

`frontend/tailwind.config.js`（135 行）声明了大量自定义 design tokens：

- **colors**：`primary.{50..950}` (Teal/Cyan 系)、`accent.{50..950}` (深蓝灰)、`dark.{50..950}` (slate 镜像) — 全部是 host 特有的，标准 Tailwind 没有
- **fontFamily**：`sans` (含 PingFang SC / 微软雅黑)、`mono`
- **boxShadow**：`glass`、`glass-sm`、`glow`、`glow-lg`、`card`、`card-hover`、`inner-glow`
- **backgroundImage**：`gradient-primary`、`gradient-glass`、`mesh-gradient`、…
- **animation / keyframes**：`fade-in`、`slide-up`、`shimmer`、`glow` 等 8 个
- **borderRadius**：`4xl`

`frontend/src/style.css`（19441 字节）：

- L1-L3：`@tailwind base/components/utilities`
- L5-L64：`@layer base` — 全局 `border-gray-200`、滚动条样式、`::selection` 用 `primary-500/20`
- L66+：`@layer components` — `.btn` / `.btn-primary` / `.btn-secondary` / `.btn-ghost` / `.btn-danger` 等基础组件 class
- 没有 CSS variable 声明（grep `--[a-z]+-[a-z0-9]+:` in `frontend/src` = 0 hit），全靠 Tailwind class 主导

### 1.2 Plugin 现状

`plugins/channel-management/frontend/`：

- **没有自己的 tailwind.config**（`Glob plugins/channel-management/frontend/tailwind.config.*` = 0 hit）
- `vite.config.ts` L24-L65：bundle 时不引入任何 host CSS / Tailwind 工具链；`cssCodeSplit:false`，最后产出 `dist/entry.css`，但当前 `plugins/channel-management/frontend/dist/` 仅 `.keep` —— **plugin 没有可用的 css 产物**
- plugin SFC 中 host-extend tokens 使用统计（grep `primary-|accent-|dark-[0-9]|shadow-glass|shadow-glow|shadow-card|btn-primary|btn-secondary|btn-ghost`）：
  - `views/ChannelsView.vue`：21 处
  - `components/PricingEntryCard.vue`：6 处
  - `components/ModelTagInput.vue`：2 处
  - `components/IntervalRow.vue`：1 处
  - **合计 30 处**
- `ChannelsView.vue` L1130-1138 使用 `@apply border-primary-600 text-primary-600 dark:border-primary-400` —— **依赖 host 的 Tailwind 配置才能解析**

### 1.3 缺口表

| 能力 | 现状 | 缺口 | 工作量 |
|---|---|---|---|
| Tailwind 颜色 token 共享 | host 在 tailwind.config 内声明，plugin 无法读取 | plugin build 时找不到 `primary-600`、`dark-700` 等 class（编译期或 PurgeCSS 阶段会被丢弃）；运行期靠 host 已 bundle 的同名 class 才生效 | **大** — 需 plugin 工具链扩展（共享 preset / runtime CSS variable） |
| 全局 base layer (`.btn-primary` 等) | host `style.css` `@layer components` 提供 | plugin 直接用 `class="btn btn-primary"` 也能命中（host 已注入 DOM 全局），但是没有契约保证 plugin SDK 暴露 | **小** — 在 SDK README 文档化"plugin 可依赖的 host 类清单"即可 |
| CSS variable 设计 token | host 没有 (0 hit) | 想做主题切换 / dark mode 跨进程同步只能改全栈 | **中** — 引入 `--sub2api-color-primary-600` 系列变量，先改 host |
| Plugin 独立产出 entry.css | 当前 `dist/.keep`，未 build | build 流程没接通；后端 `frontend_assets.go` 已有 entry_css_url 加载逻辑 | **小** — 跑 `pnpm build` 即可，但 token 缺口未解前 build 出来的 css 也不全 |

---

## 2. 后端 — Logger / Tracer / Metrics / Config / Request Context

### 2.1 Logger

- **Host**：`backend/internal/pkg/logger/logger.go:238` — `slog.SetDefault(slog.New(newSlogZapHandler(...)))`，全局 slog 桥接到 zap，输出由 zap 配置（文件 / stdout / 切割），`backend/internal/plugin/manager.go:66` 也走 `slog.Default()`
- **Plugin**：`plugin-sdk/runner.go:237` — `slog.NewJSONHandler(os.Stderr, …)`，每个 plugin 进程独立写自己的 stderr，host 端是否捕获取决于 `cmd.Stderr` 转发（未在本次调研验证）
- **缺口**：plugin 日志无法以结构化方式回流到 host zap sink，无法应用 host 的 sampling / rotation / 字段补全（如 `trace_id`、`request_id`）；ELK / loki 聚合时分两个 stream
- **工作量**：**中** — 走 gRPC `LogProxy` service 把 plugin slog 流 push 到 host zap，需要新 proto + 流式 RPC + plugin runner 替换默认 handler

### 2.2 Tracer

- **Host**：grep `otel\.Tracer|otel\.GetTracerProvider|tracer\.Start|trace\.SpanFromContext` 在 `backend` = **0 hit**；`go.mod:163-167` OTel 仅以 `// indirect` 形式存在（被其他依赖间接引入），host 完全没用
- **Plugin**：同样无 OTel 使用
- **缺口**：跨进程 tracing 完全没接 — 调试 plugin 调用链只能依赖日志关联
- **工作量**：**大** — 需要 host 先选型 + 接入（Jaeger/OTLP collector），再在 plugin SDK gRPC client / server / HTTP proxy 三层都注入 propagator

### 2.3 Metrics

- **Host**：grep `prometheus\.|metrics\.Counter|expvar\.Publish` 在 `backend` = **0 hit**；grep `/metrics` 在 `backend/internal` = **0 hit**
- **缺口**：host 自己都没有 metrics 端点；plugin 无法暴露 plugin 维度的 RPS/error rate 给监控
- **工作量**：**大** — host 需先建 metrics 基础设施（promhttp handler / 命名规范），然后 SDK 提供 `Metrics()` 接口让 plugin 注册 counter/histogram，metric label 自带 `plugin=<name>`

### 2.4 Global Config

- **Host**：`backend/internal/config/config.go:55-87` — `Config` 顶层 32 个子结构（Server/Database/Redis/Pricing/Gateway/…），无 `SiteName` / `FeatureFlag` 字段（grep = 0 hit）
- **Plugin**：`plugin-sdk/context.go:35` — `Config() map[string]string`；`backend/internal/plugin/manager.go:170,676-680` 把 `PluginRecord.Config`（plugin 自己 db 行的 config map）传给 plugin —— **完全是 plugin 自己的 namespace，不是 host config**
- **缺口**：plugin 想读 host 的 timezone / run_mode / pricing 默认值 / OAuth client_id 等"全局只读配置"，**没有任何机制**；只能要求 admin 在 plugin 自己的 config map 里冗余配置一份
- **工作量**：**中** — gRPC 加 `HostConfig.Get(key)` RPC + capability `host_config_read:<allowlist>`，host 端按 capability 决定哪些 key 能暴露（避免泄露密钥）

### 2.5 Request Context

- **Host gateway / admin 中间件设置的 ctx 内容**（grep `c\.Set\("api_key"`）：
  - `c.Set("api_key", *APIKey{...})` — 完整对象，包含 GroupID / UserID
  - `c.Set("user", ...)`、`c.Set("user_role", ...)` 等（在 middleware 包中）
- **Plugin proxy 实际透传**（`backend/internal/plugin/router_middleware.go:130-138`）：
  - `X-Plugin-User-ID`（int64）
  - `X-Plugin-User-Role`（string）
  - `X-Plugin-Name`
  - **没有** `request_id` / `trace_id` / `api_key_id` / `group_id` / 客户端 IP
- **缺口**：plugin 无法关联自己的请求到 host gateway 的同一条业务流；问题排查需手工对时
- **工作量**：**小** — 加几个 X-Plugin-* header 即可，零侵入；规范化为 SDK helper `RequestMetadata(r *http.Request)`

---

## 3. 跨进程 propagation 现状

`plugin-sdk/runner.go:122` plugin gRPC server：`grpc.NewServer()` — 无 interceptor、无 OTel handler

`backend/internal/plugin/manager.go:134` host gRPC server：`grpc.NewServer()` — 同上

`plugin-sdk/runner.go:174-210` 的 client interceptor 只做一件事：把 `x-sub2api-plugin: <plugin_name>` 加到 outgoing metadata 用于 caller identity

host 端 `backend/internal/plugin/grpc_server_redis_do.go:147` `metadata.FromIncomingContext` 也只读这一个 key

**结论**：gRPC metadata 当前**只有 caller identity 一个维度**，没有 trace context / baggage / request id 透传。任何"跨进程链路追踪"都缺基础设施。

---

## 4. V4 优先级排序建议（Inspector 视角）

> 仅给优先级建议，不替 V4 Curator 做决策

1. **Plugin → Host Logger 汇聚（slog → host zap）** — why：plugin 日志现在飘在独立 stderr，运维已经在抱怨"找不着 channel-management 的错误日志在哪"；改造范围相对受限（gRPC 加一个 LogProxy service + plugin runner 替默认 handler），ROI 高
2. **Request Context 透传（trace_id / request_id / api_key_id）** — why：在没有 OTel 的情况下，先用 X-Plugin-* header 解决 90% 的场景；几行代码 + SDK helper 就能让 plugin 日志和 host 日志能 grep 关联；为后续 OTel 改造留口子
3. **Tailwind Design Token 共享（preset / runtime CSS variable）** — why：channel-management 已经有 30 处依赖 host 的 `primary` / `dark` 颜色 token，新增 plugin 都会踩同样坑；不解决就形成"plugin 视觉割裂"或"靠运行期偶然命中"的脆弱实现

不优先：

- **OTel Tracer**：host 自己都还没接，plugin 先做意义不大；等 host 接入 OTel 时一并设计 propagator
- **Metrics**：同上，host 没有 /metrics 端点，单独给 plugin 加是空中楼阁
- **Global Config 透出**：影响面有限，多数 plugin 在 PluginRecord.Config 内自管够用；可延后到首个真实需求出现
