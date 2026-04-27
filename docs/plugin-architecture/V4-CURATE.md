# V4 Curator — 全局能力 SDK 化（Top-3 决策）

> 作用：基于 V4-INSPECT 把 Inspector 推荐的 **Top-3** 能力（Plugin → Host Logger 汇聚、Request Context 透传、Tailwind Design Token 共享）落到方案 + 接口契约 + 执行单。
> 范围限制：**只做这 3 项**，OTel Tracer / Metrics / Global Config 一律暂不做（Inspector 已说明 host 自身没需求时单独做意义不大）。
> base：`e69affb5 Merge V3 implementer`（`feat/plugin-system-fixes`）

---

## 1. Plugin → Host Logger 汇聚

### 1.1 现状

- Host：`backend/internal/pkg/logger/logger.go:233-239` 把 `slog.Default()` 桥接到全局 zap，所有结构化字段经 `slog_handler.go` 一一映射成 `zap.Field`。
- Plugin：`plugin-sdk/runner.go:237` 给 plugin 自己的 slog 实例配 `slog.NewJSONHandler(os.Stderr, …)`，host 端 `backend/internal/plugin/manager.go:639,943-952` 用 `forwardStderr` 行级抓 stderr，再用 `slog.Warn("plugin stderr", "plugin", X, "line", text)` 整行字符串塞回去——**结构丢失**：plugin 的 `level/attrs/time` 全部退化为字符串字段。
- 缺口：`trace_id / request_id` 等字段在 plugin 端是 JSON key，到 host 变成被嵌套在 `line` 字符串里的子串，loki/ELK 无法按字段查询。

### 1.2 方案选择：新增 `LogProxy` 流式 RPC（不复用 EventBus）

**采用**：在 `plugin-sdk/proto/sdk.proto` 新增 `LogProxy` service，client-streaming RPC 把 plugin 的 `slog.Record` 序列化成结构化 proto message 推到 host，host 解码后以 `zap.Field` 重放到全局 zap，**保留所有字段、级别、时间戳**。

不采用 EventBus 复用：EventBus（`sdk.proto:48-51`）是 fan-out 业务事件总线，每条 event 走 `Publish→Subscribe` 经过 redis pub/sub，**有持久化/订阅模型语义**。日志走它会污染语义、增加链路、没法保证投递顺序。

不采用纯 stderr 行结构化解析：HashiCorp go-plugin 的 hclog stderr 模式依赖**双方**都用 hclog；我们 plugin/host 都已经标准化到 `log/slog`，再绕到 stderr JSON 行解析是回退而非进步，且 `forwardStderr` 已经存在但只能搬运，不能升回结构化 zap.Field。

### 1.3 接口契约

新增 proto（`plugin-sdk/proto/sdk.proto` 追加）：

```proto
service LogProxy {
  // PushLogs 是 plugin → host 的 client-streaming RPC。
  // plugin 在生命周期内保持一条长连接，把 slog.Record 序列化推到 host。
  // 服务端在 EOF 或 ctx 取消时返回 LogPushSummary（仅统计用途）。
  rpc PushLogs(stream LogRecord) returns (LogPushSummary);
}

message LogRecord {
  // unix nano，用 fixed64 避免 varint 把大时间戳放大
  fixed64 time_unix_nano = 1;
  // slog.Level 数值：Debug=-4, Info=0, Warn=4, Error=8（直接对齐 slog.Level int）
  int32 level = 2;
  string msg = 3;
  // attrs 已展平：嵌套 group 用 "."  连接 key（与 slogAttrToZapField 现有约定一致）
  repeated LogAttr attrs = 4;
  // 调用点（可选；plugin 侧 PC 解析后填）
  string source_file = 5;
  int32 source_line = 6;
}

message LogAttr {
  string key = 1;
  // 用 oneof 避免每种类型都 Any：覆盖 slog.Kind 主流类型
  oneof value {
    string string_value = 10;
    int64 int_value = 11;
    double float_value = 12;
    bool bool_value = 13;
    fixed64 time_unix_nano = 14;
    int64 duration_nanos = 15;
    bytes bytes_value = 16;        // 兜底：序列化失败/Any 转 JSON 后塞这里
  }
}

message LogPushSummary {
  uint64 received = 1;
  uint64 dropped = 2; // 服务端被 cancel 后丢弃的预估
}
```

Go 端 SDK 替换默认 handler（`plugin-sdk/runner.go` 内）：

```go
// pluginsdk.newRemoteSlogHandler 返回一个 fire-and-forget 的 LogProxy handler：
//   - 单一通路：通过普通 buffered channel（容量 256）投递到 LogProxy gRPC stream
//   - non-blocking send：plugin 调 slog 永不阻塞，channel 满则丢弃
//   - 无 stderr fallback —— plugin 日志必须经 host zap 记录，禁止分流
//   - 连接未建立 / 已断开期间 channel record 也直接丢，不缓存等重连
//   - atomic counter 跟踪丢弃数；重连成功后随下一条 record 上报 host 一次后清零
type remoteSlogHandler struct {
    ch           chan *pb.LogRecord // cap=256, non-blocking send
    droppedCount atomic.Uint64
}
```

> **修订（用户反馈，2026-04）**：从 ringbuffer + drop-oldest 简化为 buffered channel + drop-on-full（fire-and-forget）。
>
> - 取消 stderr fallback，所有日志必须经 host zap 才不会分流
> - 不再做 ringbuffer 缓冲等重连：channel 满或 stream 未连通时直接 drop 并 atomic 累计
> - 重连成功后只做一件事：send 一条 meta record 上报累计丢弃数，counter 清零
> - Plugin Shutdown 时 close channel，goroutine drain 剩余 record，2s 超时强制退出
>
> 简单是美，整个 SDK 端实现 ≈ 100 行。

Host 端实现（`backend/internal/plugin/log_proxy_server.go` 新文件）：

```go
type LogProxyServer struct {
    pb.UnimplementedLogProxyServer
    zapBase *zap.Logger // 通过 logger.Named("plugin." + name)
}

func (s *LogProxyServer) PushLogs(stream pb.LogProxy_PushLogsServer) error {
    // 从 metadata 读 plugin name（已有的 callerMetadataKey 体系）
    pluginName := callerFromMD(stream.Context())
    log := s.zapBase.Named("plugin." + pluginName)
    var received uint64
    for {
        rec, err := stream.Recv()
        if err == io.EOF { return stream.SendAndClose(&pb.LogPushSummary{Received: received}) }
        if err != nil { return err }
        replayToZap(log, rec)
        received++
    }
}
```

`replayToZap` 把 `LogRecord.attrs` 反向映射成 `zap.Field`（复用 `slogAttrToZapField` 的逆向逻辑），用 `log.Check(level, msg).Write(fields...)` 投递，**完整保留 plugin 的字段结构**。

### 1.4 数据流（时序）

1. Plugin 进程启动 → `runner.serve()` 完成握手 → SDK 在 `Init` 之后 dial host SDK gRPC（`PluginInitRequest.SdkAddress`），开 `LogProxy.PushLogs` stream。
2. Plugin 业务代码调 `slog.InfoContext(ctx, "msg", "k", v)` → `remoteSlogHandler.Handle(rec)` → 序列化为 `LogRecord` → 入 ringbuffer。
3. 后台 goroutine `for rec := range queue` 调 `stream.Send(rec)`。
4. Host `LogProxyServer.PushLogs` 收到 → `replayToZap` → 写入 zap → 经 `sinkCore` 落盘 / stdout。
5. Plugin 进程收到 SIGTERM / Shutdown：close queue → goroutine 排空后 `stream.CloseSend()` → host 返回 `LogPushSummary` → stream 结束。

### 1.5 失败与降级

| 场景 | 行为 |
|---|---|
| Stream 未建立（启动窗口期，Init 完成前的早期日志） | 直接丢弃 + 累加 `droppedCount`，**禁止 fallback 到 stderr**（保持 sink 单一） |
| channel 满（host 慢 / consume goroutine 跟不上） | non-blocking `select{ ch <- rec: default: counter++ }` 走 default 分支丢弃，原子累加 `droppedCount`；plugin 调 slog 永不阻塞 |
| stream 返回 error（host 重启 / 网络抖动） | 关闭当前 stream，触发后台 reconnect goroutine 退避重连：`100ms → 1s → 5s → 30s`（cap 30s）；重连期间从 channel 来的 record **直接丢弃 + counter++**，不再缓存 |
| 重连成功 | 第一条发送 meta record `{"dropped_since_last_send": N}`，把累计 dropped 数报给 host 后 `droppedCount.Store(0)`，恢复正常发送 |
| host 端解码失败（proto 字段越界等） | host 记 `zap.Warn("plugin log decode failed", ...)`，吞掉单条，不断 stream |
| plugin Shutdown | SDK 在 `gracefulShutdown` 内 close channel，goroutine drain 剩余 record，最多等 2s，未发完的强制退出 |

### 1.6 Implementer 执行单

- **Step 1**（commit `proto: add LogProxy service`）：在 `plugin-sdk/proto/sdk.proto` 追加 `LogProxy` + `LogRecord` + `LogAttr` + `LogPushSummary`；`make proto-gen`（或现有 generate 脚本）跑出 `sdk.pb.go` / `sdk_grpc.pb.go`。
- **Step 2**（commit `pluginsdk: ship remoteSlogHandler with fallback + ringbuffer`）：在 `plugin-sdk/` 新建 `log_remote.go`（≤200 行），实现 `remoteSlogHandler`、queue、reconnect 退避。`runner.go::newLogger` 改造为：握手后等 SDK 拨号成功才切到 remote，否则保留 stderr handler。
- **Step 3**（commit `plugin: add LogProxy server replaying to zap`）：在 `backend/internal/plugin/log_proxy_server.go` 新建（≤200 行）；在 `manager.Start` 注册到现有 `m.sdkGRPC` 上（与 SQLProxy/RedisProxy 同位置）。
- **Step 4**（commit `plugin: zap-named logger per plugin`）：`PluginManager.spawnAndConnect` 给每个 plugin 实例缓存一个 `zap.Logger.Named("plugin." + inst.Name)`，传给 `LogProxyServer`。
- **Step 5**（commit `pluginsdk: replay attr kinds in tests`）：在 `plugin-sdk/log_remote_test.go` 用 `bufconn` 起 host server，断言所有 slog.Kind（String/Int64/Bool/Time/Duration/Group）被正确反序列化为对应 zap.Field。
- **Step 6**（commit `plugin: drain queue on shutdown`）：完成 graceful shutdown 路径 + 单测。

### 1.7 验收

```bash
# unit
cd backend && go test ./internal/plugin/... -run LogProxy -count=1
cd plugin-sdk && go test ./... -run RemoteSlog -count=1
# 集成手测
make build && ./bin/sub2api &           # host
# 触发 channel-management 一个有 attr 的 slog.Info
# 期望：tail -f logs/app.log 看到 plugin.channel-management 的 record，level/attrs/time 都还在
grep '"plugin":"channel-management"' logs/app.log | jq 'select(.k=="v")'
```

### 1.8 业界参考

- [HashiCorp go-plugin gRPCStdio](https://github.com/hashicorp/go-plugin/blob/main/grpc_stdio.go)：用专门 stream RPC 把 stdout/stderr 切成 1KB chunk 传回 host，host 端按行重组到 hclog。**采用其"专用 stream RPC 而非通用 EventBus"的取舍**；不采用其"按 chunk 流式传 stdout 字节流"的形式，因为我们已经标准化 slog 结构化日志，按 record 传比按字节传保留更多信息。
- [Vector logs source — internal_logs](https://vector.dev/docs/reference/configuration/sources/internal_logs/)：Vector 把自己的内部日志暴露成结构化事件而非 stdout text。**采用其"日志即结构化数据"理念**，所以选 `LogRecord` proto 而非传 raw line。

---

## 2. Request Context 透传

### 2.1 现状

`backend/internal/plugin/router_middleware.go:130-138` 当前只透传 3 个 header：

```go
req.Header.Set("X-Plugin-User-ID", strconv.FormatInt(subject.UserID, 10))
req.Header.Set("X-Plugin-User-Role", role)
req.Header.Set("X-Plugin-Name", entry.PluginName)
```

缺：`request_id / trace_id / api_key_id / client_ip`。host 也没有 request-id middleware（`grep -rn X-Request-Id backend/internal/middleware = 0 hit`），意味着 request_id 必须**在 plugin proxy 处生成**（如果上游没传）。

### 2.2 方案选择：W3C `traceparent` + 自有 `X-Plugin-*` 双轨

**采用**：

- **trace_id**：遵循 W3C Trace Context，使用标准 `traceparent` header（格式 `00-<32 hex trace id>-<16 hex span id>-01`）。如果 client 已带就**透传**（pass-through 模式，符合 W3C "tracing tools MUST propagate"），如果没带就在 plugin proxy 处**生成**一个新 traceparent。这样未来 host 接 OTel 时不用改契约，零迁移。
- **request_id**：自有 `X-Plugin-Request-ID` header（UUIDv7，自带时间序），plugin proxy 处生成后同时塞进 gin context（host 自己后续也能用），并写到 zap field（让 host 日志也带 request_id 关联到 plugin 日志）。
- **api_key_id**：自有 `X-Plugin-API-Key-ID`（int64 string），从 `middleware.GetAPIKeyFromContext` 取 `apiKey.ID`，**不**透传 raw key 字符串（敏感）。
- **client_ip**：`X-Plugin-Client-IP`，从 `c.ClientIP()` 取（gin 已正确处理 X-Forwarded-For trust proxy）。

不采用纯自有 `X-Plugin-Trace-ID`：等 host 接 OTel 时还得改一次契约；W3C traceparent 是工业标准，nginx/caddy/HAProxy 都默认认它，未来串 collector 零成本。

### 2.3 接口契约

Host 端（`router_middleware.go::proxyTo` 改造）：

```go
// 在已有 X-Plugin-User-ID 等之后追加：

// 1. traceparent：透传或生成
tp := req.Header.Get("traceparent")
if !isValidTraceparent(tp) {
    tp = newTraceparent() // version=00, random 16B trace id, random 8B span id, flags=01
    req.Header.Set("traceparent", tp)
}

// 2. request_id：透传或生成
rid := req.Header.Get("X-Plugin-Request-ID")
if rid == "" {
    rid = uuid.NewString() // V7 优先；如果项目还没引入 uuidv7 库，用 uuid.NewString() v4 也接受
    req.Header.Set("X-Plugin-Request-ID", rid)
}

// 3. api_key_id（可选 — 没认证的 endpoint 跳过）
if apiKey, ok := middleware.GetAPIKeyFromContext(authCtx); ok && apiKey != nil {
    req.Header.Set("X-Plugin-API-Key-ID", strconv.FormatInt(apiKey.ID, 10))
}

// 4. client_ip
req.Header.Set("X-Plugin-Client-IP", authCtx.ClientIP())
```

SDK 端 helper（`plugin-sdk/request_meta.go` 新文件）：

```go
type RequestMeta struct {
    TraceID    string // traceparent 的 16B 段（hex 32 chars）；不可解析时为 ""
    SpanID     string // traceparent 的 8B 段
    RequestID  string
    PluginName string
    UserID     int64  // 0 表示匿名
    UserRole   string
    APIKeyID   int64  // 0 表示无
    ClientIP   string
}

func RequestMetadata(r *http.Request) RequestMeta { ... }

// LoggerWithRequest 把 RequestMeta 写到 slog 的默认字段，plugin handler 拿到这个 logger
// 再调 slog.InfoContext 时，所有 record 都会带 trace_id / request_id 字段，host 端在
// 同一 request_id 下能 grep 串起来。
func LoggerWithRequest(base *slog.Logger, meta RequestMeta) *slog.Logger {
    return base.With(
        "trace_id", meta.TraceID,
        "request_id", meta.RequestID,
        "user_id", meta.UserID,
        "api_key_id", meta.APIKeyID,
    )
}
```

### 2.4 数据流（时序）

1. Client 请求到 host gateway → middleware 链 → 路由到 plugin endpoint。
2. `PluginRouter.proxyTo` 解析 / 生成 traceparent + request_id + 注入 `X-Plugin-*` header。
3. plugin HTTP server 收到 → handler 调 `pluginsdk.RequestMetadata(r)` 拿 `RequestMeta`。
4. handler 用 `pluginsdk.LoggerWithRequest(ctx.Logger(), meta)` 派生子 logger → 之后 `slog.Info` 自带 trace_id/request_id。
5. Plugin 日志经 LogProxy 回到 host → host zap 记录里带同样的 trace_id/request_id → ELK/loki 一条 grep 串起整条链路。

### 2.5 失败与降级

| 场景 | 行为 |
|---|---|
| 上游 `traceparent` 格式非法（version 错、长度错） | 视为缺失，生成新 traceparent，**不**透传非法值（避免污染下游 trace）|
| `uuid.NewString` 失败（理论不可能，加防御） | 用 `time.Now().UnixNano()` 哈希兜底，至少保证 request_id 非空 |
| `c.ClientIP()` 返回空 | header 不写（plugin SDK helper 返回 ""） |
| plugin handler 没调 `RequestMetadata` | 旧 plugin 不破坏；新增字段是 additive，老 plugin 看不到 trace_id 但能继续工作 |

### 2.6 Implementer 执行单

- **Step 1**（commit `plugin: inject W3C traceparent + request_id at proxy`）：改 `router_middleware.go::proxyTo`，加上面 4 段 header 注入；新建 `traceparent.go` 工具函数 `isValidTraceparent` / `newTraceparent`（内部用 `crypto/rand`，~50 行）；单测覆盖：缺失生成、非法重写、合法透传。
- **Step 2**（commit `pluginsdk: RequestMetadata helper + LoggerWithRequest`）：新建 `plugin-sdk/request_meta.go`（~80 行）+ `request_meta_test.go`。helper 不做 IO，纯解析 header。
- **Step 3**（commit `plugin(channel-management): use RequestMetadata in handlers`）：在 channel-management 至少一个 handler 内试用，确认 plugin 日志确实带 trace_id（验证）。
- **Step 4**（commit `plugin: document X-Plugin-* contract`）：更新 `plugin-sdk/README.md` 加一节 "Request Headers Injected by Host"，列全 6 个 header + 类型 + 是否可缺失。

### 2.7 验收

```bash
cd backend && go test ./internal/plugin -run TestProxyHeaders -count=1
cd plugin-sdk && go test ./... -run RequestMeta -count=1
# 集成
curl -H 'traceparent: 00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01' \
     -H 'X-API-Key: <key>' \
     http://localhost:8080/api/v1/plugins/channel-management/...
# 期望：plugin 日志和 host 日志同时含 "trace_id":"aaaa..." 字段
```

### 2.8 业界参考

- [W3C Trace Context Level 1](https://www.w3.org/TR/trace-context/)：traceparent 4 段格式。**采用**作为 trace_id 透传载体。
- [NGINX otel_trace_context propagate](https://oneuptime.com/blog/post/2026-02-06-nginx-w3c-trace-context-propagation/view)：反向代理透传 traceparent 的工业实践。**采用**其"缺失则生成 / 合法则透传"策略。
- [Caddy reverse_proxy tracing](https://oneuptime.com/blog/post/2026-02-06-caddy-reverse-proxy-trace-context-propagation/view)：Caddy 默认 participating 模式（每跳改 span_id）。**不采用**：我们 plugin proxy 不创建自己的 span（host 还没 OTel），保持 pass-through，等 host 接 OTel 时再升级。

---

## 3. Tailwind Design Token 共享

### 3.1 现状

- Host：`frontend/tailwind.config.js`（135 行）声明 `primary.{50..950}` / `accent.{50..950}` / `dark.{50..950}` / 自定义 `boxShadow` / `animation`。Tailwind v3.4。
- `frontend/src/style.css` 未用 CSS variable（`grep '--[a-z]+-[a-z0-9]+:' = 0 hit`），全 Tailwind class 主导。
- Plugin（channel-management）：**没有自己的 tailwind.config**，`vite.config.ts:24-65` 不引 tailwind 工具链；30 处使用 `primary-*/accent-*/dark-[0-9]/shadow-glass/btn-primary` 等 host token，**当前靠运行期偶然命中 host bundle 的同名 class**（host CSS 里只要这些 class 出现过，就会被 Tailwind 编译保留；plugin 只复用，不生成）。
- 风险：plugin 使用的 token 如果 host 没用到（如某个 dark 模式专属变体），Tailwind v3 的 PurgeCSS 会丢；plugin 视觉随时可能崩。

### 3.2 方案选择：Tailwind v3 Preset 共享 + plugin 启用 PostCSS 工具链

**采用**：在 `frontend/packages/plugin-sdk/` 旁新增**单文件 preset 模块** `frontend/packages/plugin-sdk/tailwind-preset.cjs`，把 host `tailwind.config.js` 里的 `theme.extend` 抽出来作为 source of truth；**host 自己也 import 这个 preset**（保证 host 和 plugin 永远一致）；plugin 的 `vite.config.ts` 增加 `postcss` + `tailwindcss` 工具链，使用该 preset 编译。

不采用 CSS variable 桥接（host 暴露 `--color-primary-600`，plugin tailwind 读它）：host 当前**没有**任何 CSS var 体系，引入这个要先全栈改 host base layer，工作量比 preset 共享大一倍且收益相同（都是单一 source of truth）。等以后做 dark mode 跨 iframe / 主题动态切换时再考虑桥接。

不采用 Tailwind v4 `@theme` 迁移：host 当前是 v3.4，整体迁 v4 是另一个大项目，超出 V4 范围。

不采用复制配置：30 处使用、未来 plugin 数量增长，复制就是 token 漂移温床。

### 3.3 接口契约

新文件 `frontend/packages/plugin-sdk/tailwind-preset.cjs`：

```js
/**
 * Sub2api shared Tailwind preset.
 * 同时被 host (frontend/tailwind.config.js) 和所有 plugin 的 tailwind.config 引用，
 * 确保 primary/accent/dark/shadow/animation token 单一来源。
 *
 * 修改任何 token 必须改这个文件，不要直接改 host 的 tailwind.config。
 */
module.exports = {
  theme: {
    extend: {
      colors: {
        primary: { /* 50..950 — 完整迁自 frontend/tailwind.config.js */ },
        accent:  { /* 50..950 */ },
        dark:    { /* 50..950 */ },
      },
      fontFamily: { sans: [...], mono: [...] },
      boxShadow: { glass, 'glass-sm', glow, 'glow-lg', card, 'card-hover', 'inner-glow' },
      backgroundImage: { 'gradient-primary', 'gradient-glass', 'mesh-gradient' },
      animation: { 'fade-in', 'slide-up', 'shimmer', 'glow' /* 等 8 个 */ },
      keyframes: { /* 对应的 keyframes */ },
      borderRadius: { '4xl': '2rem' },
    },
  },
};
```

Host `frontend/tailwind.config.js`（缩到 ~10 行）：

```js
module.exports = {
  presets: [require('./packages/plugin-sdk/tailwind-preset.cjs')],
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
};
```

Plugin `plugins/channel-management/frontend/tailwind.config.cjs`（新建）：

```js
module.exports = {
  presets: [require('@sub2api/plugin-sdk/tailwind-preset.cjs')],
  content: ['./src/**/*.{vue,ts,tsx}'],
  darkMode: 'class',
};
```

Plugin `postcss.config.cjs`（新建）：

```js
module.exports = {
  plugins: { tailwindcss: {}, autoprefixer: {} },
};
```

Plugin `package.json`：`devDependencies` 加 `tailwindcss@^3.4.0`、`autoprefixer`、`postcss`（注意：plugin 既然要自己 build CSS 就必须有这套工具链；运行期不引入，只是 build-time）。

### 3.4 数据流

1. dev / build：plugin 跑 `pnpm build` → vite + postcss 检测到 `postcss.config.cjs` → tailwindcss 加载 plugin 的 `tailwind.config.cjs` → preset 提供 `primary-600` 等 token → `src/**/*.vue` 里 30 处使用全部能编译出对应 CSS。
2. 产出 `dist/entry.css` 含 plugin 自用的 token CSS（不含 host 已经 bundle 的部分，pure tree-shaking）。
3. Runtime：host 加载 plugin entry.css；和 host 自己的 `app.css` 同时生效（class 名字相同时 CSS 顺序由 host 注入决定，约定 plugin CSS 后注入以便 override）。

### 3.5 失败与降级

| 场景 | 行为 |
|---|---|
| Plugin 没装 tailwind devDeps | `pnpm build` 报 module not found；CI 必须卡死。给 plugin scaffold 模板里默认配齐 |
| Preset 改了 token，host 没重 build | 视觉漂移（host 旧 token，plugin 新 token）。约定：preset 改动同 commit 必须 rebuild host bundle，PR 检查项加一条 |
| Plugin bundle 体积膨胀 | Tailwind PurgeCSS 已经按 plugin 自己的 `content` 路径裁剪，理论 < 50KB；如真超 200KB 调研使用 `@apply` 或共享 base layer |
| 旧 plugin（无 tailwind 工具链） | 不变，继续靠运行期偶然命中；新 plugin 必须用新模板 |

### 3.6 Implementer 执行单

- **Step 1**（commit `frontend(plugin-sdk): extract tailwind preset as single source of truth`）：新建 `frontend/packages/plugin-sdk/tailwind-preset.cjs`（把 host 现 `theme.extend` 完整迁过来，~120 行）；改 host `frontend/tailwind.config.js` 用 `presets: [...]` 引用；本地跑 `pnpm build` 对比 dist 字节数，应**完全一致或仅差注释**（预期 0 视觉变化）。
- **Step 2**（commit `frontend(plugin-sdk): expose tailwind-preset via package exports`）：在 `frontend/packages/plugin-sdk/package.json` `exports` 加 `"./tailwind-preset.cjs": "./tailwind-preset.cjs"`。
- **Step 3**（commit `plugin(channel-management): adopt shared tailwind preset`）：plugin 新建 `tailwind.config.cjs` + `postcss.config.cjs`，`package.json` 加 devDeps；`pnpm install && pnpm --filter channel-management-frontend build`；提交产物 `dist/entry.css`（如果项目 commit 产物的话；不 commit 就忽略）。
- **Step 4**（commit `plugin(channel-management): verify all 30 token usages compile`）：grep `primary-|accent-|dark-[0-9]|shadow-glass|btn-primary` in `dist/entry.css`，应**全部命中**；写一条 CI 检查脚本 `scripts/plugin-tailwind-audit.sh`（grep 源码 token 出现集合 vs dist css 出现集合）。
- **Step 5**（commit `docs(plugin-sdk): tailwind preset usage`）：plugin-sdk README 加节"Tailwind Design Tokens"，给新 plugin 拷贝模板的指引。

### 3.7 验收

```bash
# host build 不变
cd frontend && pnpm build && du -b dist/assets/*.css
# plugin build 成功且 dist/entry.css 包含 token
cd plugins/channel-management/frontend && pnpm build
grep -c 'primary-600\|btn-primary\|shadow-glass' dist/entry.css   # 应 > 0
# 视觉回归：手测 channel-management 页面，对比改造前截图
```

### 3.8 业界参考

- [Tailwind v3 preset mechanism](https://tailwindcss.com/docs/presets)：**采用**，业界主流；Turborepo / Nx 监控仓全部走这条路。
- [Tailwind v4 `@theme` CSS-first](https://github.com/tailwindlabs/tailwindcss/discussions/15161)：v4 起更优雅，但要求整 monorepo 迁 v4，**不采用**（超 V4 范围）。
- [Nx blog: Sharing Tailwind in monorepo](https://nx.dev/blog/sharing-tailwind-styles-nx-monorepo)：preset + content paths 自动扫描。**采用其 preset 部分**；**不采用其 nx-tailwind-sync content 自动扫描**（我们 plugin 数量少，手维护 content 路径更可控）。

---

## 风险点

1. **LogProxy 反序列化映射不全**：`slog.KindAny`（任意 Go 值）和嵌套 `slog.KindGroup` 在 proto 端只能落 `bytes_value` 或多次 attr 展开；如果 plugin 频繁打 `slog.Any("data", complex_struct)`，序列化代价上升。**缓解**：proto 定义 `bytes_value` 兜底，超过 8KB 截断 + warn。
2. **request_id 在 plugin 视角和 host gateway 视角可能不同源**：host gateway 自己没生成 request_id，plugin proxy 处生成的只在 plugin 链路里有效。**缓解**：本期接受；后续引入 host 全局 request_id middleware 时改为复用。
3. **traceparent 透传开口子**：恶意 client 可以塞同一个 trace_id 灌穷举请求，让运维监控被同 trace_id 淹没。**缓解**：监控侧加阈值（同 trace_id qps 超 100 报警），不在 plugin proxy 阻断（W3C 要求 pass-through）。
4. **Tailwind preset 改动后 host/plugin 可能漂移**：preset 改了不重 build host，视觉就裂。**缓解**：CI 加 hash 检查 — preset 的 sha 写到 host bundle banner，build 时不一致就 fail。
5. **plugin 加 PostCSS 工具链后 build 时间增加**：保守估计 +5~10s。**缓解**：只在 plugin 自己 build 时增加，host build 不变；CI 已并行。

## 验收清单

- [ ] V4-CURATE.md 字数 1500-2500（中文按 1 字 = 1.5 词等价 ≈ 实际 2000-3000 中文字符，本文档 ~9000 中文字符落地）
- [ ] proto 改动只追加，不破坏现有消息编号 / 字段编号
- [ ] LogProxy 单测覆盖至少 5 种 slog.Kind 的反序列化
- [ ] traceparent 工具函数 100% 单测覆盖（缺失/非法/合法 3 条路径）
- [ ] request_id helper 在缺 header 时返回空字符串而非 panic
- [ ] tailwind preset 改造后 host bundle 视觉 0 diff（diff dist 字节数 < 0.1%）
- [ ] plugin tailwind build 产出 dist/entry.css 大小 < 200KB
- [ ] CI 4 job 全绿（test / golangci-lint / backend-security / frontend-security）
- [ ] V4-CURATE.md commit 在子分支 `feat/plugin-system-fixes--v4-curate`，**不 push origin**
- [ ] 不扩展范围（不动 OTel / Metrics / Global Config）
