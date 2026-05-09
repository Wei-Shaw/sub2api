# Sub2API Repo Notes

本文件用于沉淀这个仓库里已经反复验证过、适合长期复用的实现经验。目标不是重复全局规则，而是给后续子代理一个**项目级事实基线**，减少反复踩坑。

## 优先级

- 先遵守用户当前指令。
- 再遵守本仓库 `AGENTS.md`。
- 再遵守全局 `AGENTS.md` / 平台默认规则。

## 长期保留的本地语义

以下几条不是“还没同步 upstream”，而是当前仓库已经确认要保留的本地主线差异。后续同步 upstream、解冲突、做重构时，默认要保住它们；如果 upstream 代码看起来“更干净”，也不能直接覆盖这些本地产品语义。

1. OpenAI `active / exhausted / any / -Sys` guardrail 语义
- `-Sys` 请求与 exhausted-class 路由、tool continuation、相关错误码/调度语义已经是本地基线。
- 不要为了跟 upstream 靠齐而把 `reserve`、`-Sys`、`active/exhausted` 重新抹平成一套普通 target group。

2. routing observability 超集链
- `routing_*` 相关 usage 写库、request details、ops dashboard、重试钻取是本地主线能力。
- 同步 upstream 时不要把这些字段或页面能力裁回窄版。

3. 用户侧 pricing factor 展示链
- `BillingTier`、`PricingSource`、`EffectiveMultiplier`、`Effective*UnitPrice` 等解释层字段是本地保留语义。
- DTO / repo / UI 都要一起看，不要只盯单层。

4. `sub2api-openai` OpenCode 推荐配置链
- 这是本地产品能力，不是 upstream 原生内建配置。
- 当前推荐配置不只是 provider 名称，还包括 OpenCode 展示入口、`GPT-5.5 Fast (Sys)` / `GPT-5.5 Image (Sys)` 这类本地模型命名、`variant: image` 语义，以及配套的 OpenAI / Codex 路由提示。
- 不要把它误改成 `provider.openai`，也不要假设 upstream 会直接消费我们的私有字段或 runtime materialization 结构。

5. OpenAI model-subset projection 语义
- OpenAI exhausted / reserve 现在不是纯账号级静态分桶；本地主线已经引入“`scheduler bucket + canonical routing model`”维度的 projection。
- 请求阶段必须消费预计算 projection 结果，不要在热路径重新 live derive reserve 身份。
- 对 projection 参与账号，当前 bucket bundle 是自包含视图：
  - `Accounts` = 主快照账号集
  - `ProjectionAccounts` = projection 构建参与账号全集
  - `Projection` / `ProjectionVersion` / `BuiltAt` = 同版发布
- 后续同步 upstream 时，不要把这套 bundle/projection 结构回退成只存主快照账号，或重新引入 `GetAccount()/DB` 对 projection 账号的回退混读。

6. OpenAI reserve active-overflow 语义
- 当前本地主线语义已经修正为：`reserve` 首先是 active 身份，其次才是 exhausted overflow 身份。
- `routing_target_group` 继续表示请求语义（`active / exhausted / any`），`routing_selected_group` 表示实际账号身份；命中 reserve 时统一写 `reserve`。
- `active/any` 现在可以命中 reserve；`exhausted` 仍按原 overflow / 60% / `exhausted=0 => 100%` 规则消费 reserve。
- reserve 身份的唯一来源必须是**当前 canonical projection view 的 `ReserveOverflowIDs`**，不是 live `IsOpenAIReserveCandidate()`，也不是 legacy overlay 临时推导。
- 旧 binding 兼容规则也已是本地主线语义：`selected_group=reserve + affinity_domain=exhausted` 的历史 binding 在 projection 元数据仍匹配时必须可读；不要在同步 upstream 时把 active/any reserve binding 兼容逻辑删掉。

7. OpenAI / OpenCode `image_generation` 本地 carrier 链
- `builtin_tools` / `metadata.builtin_tools` 是本地 carrier，不是 upstream 原生字段；它负责把 OpenCode 推荐配置里的生图意图转换成上游可接受的 `tools` / `tool_choice`。
- `/v1/responses` 和 Chat Completions 两条入口都已经接入生图 carrier；同步 upstream 时不要只保留其中一条，也不要把 Chat Completions 的 carrier / 约束注入删掉。
- `image_generation` 是否可用受账号能力、model-subset projection、`-Sys` 入口和 OpenCode image variant 共同约束；不能因为 upstream 新增了类似字段，就绕过本地 gating 直接全局开启。

8. OpenCode 生图结果改写、回填和下载链
- OpenCode 不能直接消费上游 `image_generation_call.result` 的 base64；本地主线会把结果写入 `OpenAIGeneratedImageStore`，再通过 `/sub2api/generated-images/...` 这类短期下载 URL 交给 OpenCode 工具链。
- `openai_opencode_image_rewrite.go`、`openai_opencode_image_sse.go`、`openai_opencode_image_rehydrate.go`、`openai_generated_image_store.go`、`generated_image_handler.go` 及相关 route / middleware / redaction 测试属于同一条能力链，不能在同步 upstream 时按“清理未使用代码”拆掉。
- 这条链还负责避免把 base64、服务端本地路径或敏感图片输出写进客户端最终文本、请求日志和 ops upstream context。

9. OpenCode 生图服务端续接与下载工具调用链
- 第一轮生图完成后，本地主线会服务端发起第二轮 `/v1/responses` 续接，并在 `input` 中追加 synthetic `function_call` / `function_call_output`（工具名为 `sub2api_image_generation_result`），让 OpenCode 继续执行可下载工具。
- 第二轮续接必须移除 `image_generation` tool，避免再次生图；如果仍有普通 `function` tool，必须把 `tool_choice` 设为 `"required"`，强制模型先输出工具调用再总结。
- 如果第二轮没有任何可用 function tool，必须删除无效的 `required`，退回“明确说明无法下载并提供临时 URL + marker”的 fallback；不要改回纯 prompt-only、`previous_response_id + function_call_output` 增量续接或硬编码 `bash`。

10. OpenAI `-Sys` / 非 `-Sys` 模型访问保护
- 普通用户侧的公开入口应优先暴露 `-Sys` 模型；非 `-Sys` 模型访问限制、错误文案和 handler 层测试是本地 guardrail。
- 非 `-Sys` 入口的路由语义不是 exhausted-class 入口；当前本地主线已经让它走 active 路由。同步 upstream 时不要把非 `-Sys` 请求重新接回 exhausted / reserve 溢出语义。

11. 错误与图片输出脱敏链
- gateway handler、Gemini handler、error passthrough runtime、ops upstream context 中的错误与图片输出脱敏是本地安全基线。
- 不要为了贴近 upstream，把上游原始错误、图片 base64、临时下载 URL、服务端路径或账号细节重新写回客户端、日志、ops dashboard 或 usage 记录。

12. 支付与邀请返利本地产品链
- 邀请返利系统、feature toggle、按用户自定义邀请设置、Zpay refund endpoint 兼容，以及 Stripe 支付页绕过 router auth guard 都是本地产品能力或支付兼容修复。
- 同步 upstream 时不要只看后端 handler，也要一起保住 schema / repository / DTO / 前端入口 / 路由守卫测试；这些改动经常跨目录出现，不能按“非网关代码”误删。

13. 真实 Claude Code 客户端 prompt caching 兼容
- 真实 Claude Code 客户端需要跳过 body mimicry，才能保住 prompt caching 语义。
- 同步 upstream 的网关 body transform、Anthropic 兼容或请求 mimicry 代码时，不要把这条客户端识别与跳过逻辑覆盖掉。

## OpenAI / Codex 兼容经验

1. `tool_choice` 标准对象形态需要兼容转换
- Chat Completions 标准对象：
  - `{"type":"function","function":{"name":"X"}}`
- Responses 上游期望：
  - `{"type":"function","name":"X"}`
- 这条兼容 bug 已经修过；后续再碰 `tool_choice` 时，先确认有没有把 nested `function` 原样透传。

2. built-in tools 当前稳定边界
- `web_search`：当前稳定可用。
- `code_interpreter`：当前不支持，不要在 phase 1 假设能用。
- `image_generation`：只能通过本地 carrier + 能力过滤 + OpenCode image variant 受控启用；在当前 Codex OAuth/provider 链上不能当作所有账号都稳定可用的 built-in tool。

3. `/v1/responses` continuation 的现实边界
- 当前不支持 `previous_response_id + function_call_output` 增量 continuation。
- 现有显式错误应保持为：
  - `previous_response_id + function_call_output is not supported on /v1/responses; replay the full conversation instead`
- 如果要排查这条链，先区分：
  - 本地显式拒绝
  - 上游 raw error
  - 已开流后的 `stream_read_error`

4. `metadata` 不是上游可直接接受的字段
- 在 `/v1/responses` 主链上，如果读取了 `metadata.builtin_tools` 作为本地 carrier，就必须在发往上游前把整段 `metadata` 去掉。
- 只删除 `metadata.builtin_tools` 子键是不够的；上游会直接报：
  - `Unsupported parameter: metadata`

5. stream failover 的安全边界
- 只有在真正**还没开始向客户端发正文级事件**之前，streaming failover 才安全。
- `response.created` / `response.in_progress` 这种前导事件也不能提前开流。
- 一旦已经发出正文级事件，再自动切号重试就会污染输出语义。

6. GPT-5.x / Codex 上游模型归一化的当前本地主线
- `gpt-5.5` / `GPT-5.5-Sys` 这类已确认支持的新模型，不得再被 OAuth 上游归一化错误改写成 `gpt-5.1`。
- 当前本地主线是：显式保留 `gpt-5.5`；未知未来 `gpt-5.x` minor 仍可按现有保守策略回落，不要在同步 upstream 时把“保留所有未知 minor”或“全部回退到 5.1”任一极端误带回来。

7. unknown model 的当前边界
- unknown model 仍然要 fail-closed；不能因为 active/any 放开 reserve 就重新 live 放开 unknown model。
- 请求侧最多只能写受控的 refresh signal（如 `openai_unknown_model_refresh_request`），不能直接把 unknown model 当成 catalog success/source-of-truth 写入持久能力状态。
- 如果后续真的接入外部/异步目录刷新，也要保持“请求侧发 signal，目录源刷新后再重建 projection”的边界。

## Token / 缓存统计口径

这些口径已经反复踩坑，后续改 dashboard、usage、ops 时必须统一：

1. `usage_logs.input_tokens` 是**净输入**
- 它已经扣掉 `cache_read_tokens`。
- 不能把它当“总输入 prompt token”。

2. 前端展示的推荐口径
- `总输入 = input_tokens + cache_read_tokens + cache_creation_tokens`
- `净输入 = input_tokens`
- `总 token = 总输入 + output_tokens`

3. 缓存命中率公式
- 不要再用：
  - `cache_read / (cache_read + cache_creation)`
- 正确应按 prompt token 总量算：
  - `cache_read / (input + cache_read + cache_creation)`

4. TPM / TPS 统一要求
- 统计总 token 时，要确认缓存 token 是否计入。
- 后端聚合和前端展示必须一起核，不要只改一层。

## 部署经验

### 仓库级事实

仓库内已经存在多套部署材料，但嵌入版前端始终遵循同一条事实：

- `backend/internal/web/embed_on.go` 嵌入的是 `backend/internal/web/dist`
- 所以前端有改动时，构建顺序必须是：
  1. `pnpm build`
  2. 再 `go build -tags embed`

### 当前生产实例经验（不是仓库硬编码契约）

我实际排查过的那台生产机使用的是 **binary + systemd**，而不是 compose 主链。对后续排障有参考价值，但这些是实例事实，不是仓库通则：

- 二进制路径：`/opt/sub2api/sub2api`
- systemd 单元：`sub2api.service`
- 当前实例常见健康检查端口：`18080`
- 具体 env 文件位置、监听端口仍以线上 `systemd unit` / 实际部署文件为准，不要先入为主假设所有机器都一样

部署时必须注意：

1. 前端先 build，再 Go embed build
- 先 `pnpm build`
- 再 `go build -tags embed`
- 不要并行跑，否则 embed 可能打进旧的 `backend/internal/web/dist`

2. 部署后至少验证三件事
- `systemctl status sub2api --no-pager -l`
- `curl http://127.0.0.1:18080/health`
- 如有前端改动，直接抓线上首页和对应 chunk，看新资源是否真的被引用

3. 如果对“最新二进制是否真上到 VPS”有怀疑
- 直接对比：
  - 本地 `backend/bin/sub2api-linux-amd64` 时间戳 / 大小
  - VPS `/opt/sub2api/sub2api` 的 `stat`
- 不要只凭“我记得刚才部署过”判断

4. VPS SSH 不稳定时的经验
- `scp`/`ssh` 偶发 `Connection closed by ... port 22` 不一定代表构建或代码问题。
- 先用一次 `ssh ... echo ok` 探测恢复，再重试上传/部署。

5. 从本机发版到 Linux VPS 时，必须显式交叉编译目标平台
- 不要直接在 Windows 本机 `go build` 后就上传；那会产出错误平台二进制，线上会报：
  - `Exec format error`
- 当前这条生产线默认目标是 Linux x86_64，至少显式指定：
  - `GOOS=linux`
  - `GOARCH=amd64`
  - `CGO_ENABLED=0`
- 上传前可先记一次本地产物的时间戳 / 大小，方便和 VPS 侧对比。

6. 替换线上二进制前，先做“接入生产环境但不公开提供”的服务端预发布验证
- 推荐流程：
  1. 上传候选二进制到临时路径，例如 `/tmp/sub2api-linux-amd64.new`
  2. 用 `file` 确认它确实是 Linux ELF，例如 `ELF 64-bit LSB executable, x86-64`
  3. 用正式 env 启动临时实例，但只绑定本机回环地址和备用端口，例如：
     - `SERVER_HOST=127.0.0.1`
     - `SERVER_PORT=18081`
  4. 至少验证：
     - `curl http://127.0.0.1:18081/health`
  5. 如果本次改动影响网关 / provider / OpenAI 兼容链，尽量再补一条真实请求链路 smoke（接生产上游，但不对外暴露）。
  6. smoke 通过后，再把候选二进制提升为正式 `/opt/sub2api/sub2api` 并重启 systemd。

7. 如果替换后服务没起来，先看 `systemd` 错误类型，再决定回滚或修复
- `Exec format error` 优先怀疑上传了错误平台二进制。
- 这类问题不要继续硬重启；应先立刻回滚到最近备份二进制，恢复 `active + /health ok`，再重新构建正确产物。

8. 正式部署验收不能只看 `systemd + /health`
- 对当前这类 OpenAI 网关实例，`systemctl is-active` 和 `/health` 只能证明服务起来了，不能证明关键模型路由可用。
- 如果本次改动涉及 OpenAI 调度、projection、`-Sys`、reserve、tool continuation 或模型兼容链，正式验收至少补一条真实模型请求。
- 当前生产侧已经验证过的最小三模型探针是：
  - `GPT-5.4-Sys`
  - `gpt-5.5`
  - `GPT-5.5-Sys`
- 这三者必须分别记录 `HTTP status` 与错误体；不要只记“好像能用”。

9. `gpt-5.5` 与 `GPT-5.5-Sys` 的排障必须分开看
- 当前本地主线语义里，`GPT-5.5-Sys` 会先去掉 `-Sys`，再按 exhausted-class / projection 语义调度；plain `gpt-5.5` 则走 active 语义。
- 因此线上现象如果是：
  - `GPT-5.5-Sys = 200`
  - `gpt-5.5 = 503`
  不能直接下结论说“上游账号整体坏了”或“部署失败了”。
- 先区分：
  - plain active 路由无可用账号
  - exhausted / reserve 路由可用
  - 还是账号本身完全不可用

10. 线上 `503` 要先抓 request id，再回看日志与 Redis bundle
- 对 `/v1/responses` 的 `503`，优先保留：
  - 请求模型
  - `request_id`
  - 返回错误体
- 当前实例中，`request_id` 能直接在 `journalctl -u sub2api.service` 中反查到 `openai.account_select_failed` 等日志，足够先判断是“无可用账号”还是别的错误。
- 如果错误体是：
  - `No available accounts in target group (active)`
  或日志里是：
  - `no available OpenAI accounts supporting model: ...`
  先查当前 scheduler bucket 的 Redis 活跃 bundle，而不是先怀疑 deploy 没生效。

11. 当前生产侧 OpenAI bundle 的关键观察点
- 当前 Redis key 约定已经确认：
  - active version: `sched:active:<group>:openai:single`
  - bundle payload: `sched:openai:<group>:openai:single:v<version>`
- 对 group `2` 的生产实例，排查 `gpt-5.5` / `GPT-5.5-Sys` 时，至少看：
  - `projection_version`
  - `built_at`
  - `projection.Models.<canonical model>.ExhaustedBaseIDs`
  - `projection.Models.<canonical model>.ReserveOverflowIDs`
  - `accounts`
  - `projection_accounts`
- 如果 `accounts` 里有账号、`projection_accounts` 里也有账号，但某模型视图把它排除，优先怀疑调度 / projection 语义问题，不要直接归因到账号离线。

12. 排查 team 账号是否属于 exhausted 时，不要只看 `status=active`
- `TEAM-*` 这类 OpenAI OAuth team 账号在排障时，不能只看 `status=active` / `schedulable=true` 就断定它属于 active 路由。
- 应先核当前 exhausted 判定所依赖的 quota / usage 字段，再判断它在当前模型请求里应落在 active 还是 exhausted 语义下。
- 换句话说：账号“活着”不等于该模型请求一定会按 active 身份命中它。

## 磁盘与运维经验

生产机曾出现 `No space left on device`，根因不是数据库，而是：

- `/opt/sub2api/sub2api.before-*` 历史备份二进制堆积
- `/var/lib/docker/containers/*-json.log` 容器 JSON 日志膨胀

排查顺序建议：

1. `df -h`
2. `du -sh /opt/sub2api /opt/sub2api/data /var/lib/docker ...`
3. `find /var/lib/docker/containers -name '*-json.log' -printf '%s %p\n' | sort -nr | head`

已验证的安全缓解手段：
- 删除过时的部署备份二进制（注意仓库安装脚本更接近 `sub2api.backup*` 命名；`sub2api.before-*` 只是我排障时在某台机器上的实例命名）
- 截断最大的 Docker JSON logs（这是应急止血经验，不是通用长期策略）

## Upstream 同步 / 消灭 behind 经验

1. 语义吸收 != commit graph 同步
- 手工把代码改得“等价”不会降低 `behind`。
- 想把 `behind` 清零，最终还是要做真正的 `git merge origin/main`（或等价图同步）。

2. 先分桶，再决定是吸尾巴还是直接 merge
- 先把远端 commit 分成：
  - merge wrapper / 中间 merge
  - 已等价吸收
  - 必须保留的本地差异
  - 真正需要同步的尾巴

3. 真正 merge 前先保护当前工作
- 如果本地有未提交改动，先 stash / checkpoint。
- merge 过程中如果出现大量冲突，优先按“领域”拆开并行处理。

4. 冲突解决后的必做验证
- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1`
- `go build ./cmd/server`
- `pnpm typecheck`
- `git diff --check`

## 子代理 / worktree 经验

1. Go 路径必须显式告诉子代理
- 很多环境里 `go` 不在 PATH。
- 后续所有新开的子代理都应明确写死：
  - `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe`

2. 大功能优先用项目内 `.worktrees/`
- 当前仓库已经长期使用 `.worktrees/` 进行隔离开发。
- 大 feature / upstream PR / 高风险同步，优先单开 worktree。

3. review 子代理要拿到真实 spec/plan 路径
- 不要手抄摘要给 reviewer。
- 直接给它们完整 spec/plan 路径，让它们自己读文件。
- 当前仓库的真实约定目录就是：
  - `backend/docs/superpowers/specs/*.md`
  - `backend/docs/superpowers/plans/*.md`

4. 异步 usage 记录前必须先抓快照
- 只要某条 handler 会把 `RecordUsage(...)` 或类似写库动作放进 worker pool / 异步闭包，就不要在闭包里再去读 live `gin.Context` 或共享 snapshot 指针。
- 必须在入队前先捕获：
  - `RoutingSnapshot`（必要时深拷贝）
  - `InboundEndpoint`
  - `UpstreamEndpoint`
  - 以及任何后续要写库的 request-scoped 值
- 这是高频回归点，已经被专门测试锁过；以后子代理碰到 async usage 路径时要默认先检查这一点。

## 当前容易再踩的坑

1. 不要把 `reserve` 当第三 target group
- `reserve` 是 exhausted-class overflow 子组。
- `routing_target_group` 继续是请求语义组；`routing_selected_group` 才是实际命中子组。

2. 不要把 upstream runtime 结构当成 upstream config 契约
- OpenCode upstream 的 `experimental.modes -> *-fast` 是 runtime/provider model materialization 逻辑。
- 我们的 `sub2api-openai` 推荐配置只是参考这层语义，不是原样照搬 upstream config 结构。

3. 不要把本地私有 carrier 当成 upstream 原生字段
- 比如：
  - `builtin_tools`
  - `metadata.builtin_tools`
- 这些都属于本地扩展，需要在转发前吃掉，不应原样透传给上游。

4. `metadata.builtin_tools` 的当前运行时基线
- 当前 `/responses` 主链在消费 `metadata.builtin_tools` 后，是**整段删除 `metadata`**，不是只删子键。
- 不要默认以为 `trace_id` / `client` 这类 metadata 兄弟键还能继续透传上游；先核当前实现。
