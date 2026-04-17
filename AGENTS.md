# Sub2API Repo Notes

本文件用于沉淀这个仓库里已经反复验证过、适合长期复用的实现经验。目标不是重复全局规则，而是给后续子代理一个**项目级事实基线**，减少反复踩坑。

## 优先级

- 先遵守用户当前指令。
- 再遵守本仓库 `AGENTS.md`。
- 再遵守全局 `AGENTS.md` / 平台默认规则。

## 长期保留的本地语义

以下几条不是“还没同步 upstream”，而是当前仓库已经确认要保留的本地主线差异。后续同步 upstream、解冲突、做重构时，默认要保住它们：

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
- 不要把它误改成 `provider.openai`，也不要假设 upstream 会直接消费我们的私有字段。

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
- `image_generation`：在当前 Codex OAuth/provider 链上不能当作稳定可用 built-in tool；不要轻易承诺。

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
