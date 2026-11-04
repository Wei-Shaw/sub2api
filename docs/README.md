# Sub2API 文档中心

本页是开发、二次开发和代码代理的统一入口。面向部署用户的快速说明仍在根目录三语 README；本目录重点回答“系统如何组成、代码在哪里、改动应验证什么”。

## 新贡献者阅读顺序

1. [开发指南](../DEV_GUIDE.md)：搭建环境、运行项目、测试和生成代码。
2. [架构总览](ARCHITECTURE.md)：进程、层级、依赖方向和数据边界。
3. [代码地图](CODE_MAP.md)：按功能和任务快速定位实现与测试。
4. [关键请求链路](REQUEST_LIFECYCLES.md)：API Key 网关、管理端 API、计费与前端构建链路。
5. [后端目录索引](../backend/README.md) 或 [前端目录索引](../frontend/README.md)：进入具体子系统。

代码代理还应先读取根目录 [AGENTS.md](../AGENTS.md)。进入子树后继续读取 [backend/AGENTS.md](../backend/AGENTS.md) 或 [frontend/AGENTS.md](../frontend/AGENTS.md)；这些文件只保存高频、稳定的操作约束，细节以本目录和源码为准。

## 按功能查阅

从 [功能总览与文档索引](FEATURES.md) 查找用户端、管理端、网关和后台能力，以及每个前端 feature 和后端模块的目录 README。页面入口、嵌入式组件和后台接口在索引中分别标注。

| 文档 | 内容 |
| --- | --- |
| [活动中心](ACTIVITY_CENTER.md) | 抽奖、签到、兑换膨胀、资格与奖励记录 |
| [在线客服](SUPPORT_CHAT.md) | 会话、图片、已读、转账、保留清理与多实例边界 |
| [自定义模型与媒体工坊](CUSTOM_MODELS_AND_MEDIA.md) | 能力配置、请求模板、媒体生成与分组路由 |
| [PR #169 媒体工坊同步审查](PR169_MEDIA_STUDIO_REVIEW.md) | 浏览器历史边界、原子视频计费、性能、范围关闭与升级回退 |

## 接口与集成

| 文档 | 内容 |
| --- | --- |
| [权限组与客服账号教程](PERMISSION_GROUPS.md) | 启用客服、自定义权限组、分配员工账号、验收和回退 |
| [Admin API](ADMIN_API.md) | Admin API Key、权限范围和管理接口调用方式 |
| [批量同步账号上游模型](ACCOUNT_UPSTREAM_MODELS_SYNC.md) | OAuth/API Key 账号的一键模型同步、保留规则、逐账号结果与验证 |
| [管理 MCP](MANAGEMENT_MCP.md) | OAuth/PKCE 管理工具、权限与审计、启用和升级回退 |
| [OAuth2 对外授权服务](OAUTH2_PROVIDER.md) | 管理员控制的客户端、PKCE 授权码、用户信息与撤销接口 |
| [降智账号 API](ACCOUNT_QUALITY_DEGRADED_ACCOUNTS_API.md) | 读取降智 OAuth 账号邮箱、account ID 及 401 认证错误 |
| [Cloudflare 接口分类与边缘规则建议](CLOUDFLARE_EDGE_RULES.md) | 按浏览器页、自动化 API、纯 API 和强人机验证入口分类，方便编写 CF/WAF 规则 |
| [管理端支付集成 API](ADMIN_PAYMENT_INTEGRATION_API.md) | 外部支付页面与余额/兑换集成 |
| [异步图片任务](ASYNC_IMAGE_TASKS.md) | 异步 Images API 的启用、提交和轮询 |
| [批量图片 MVP](BATCH_IMAGE_MVP.md) | 批量图片任务的接口与状态模型 |

路由的最终事实来源是 `backend/internal/transport/http/server/routes/`。文档不维护一份完整路由副本，以免与代码产生双重事实。

## 功能设计

| 文档 | 内容 |
| --- | --- |
| [TypeSafe 与充值阶梯专项适配](UPSTREAM_ADAPT_20261006.md) | 原生协议、计费探测、优惠快照、升级回退和验证 |
| [组合分组](COMPOSITE_GROUPS.md) | Composite group 的平台解析和使用约束 |
| [Excel BPS 与 DeepSeek 兼容](BPS_DEEPSEEK_COMPAT.md) | 账号级 BPS 开关/筛选/批量更新、Responses 与 Chat Completions 兼容入口 |
| [Codex OAuth 模拟的有意差异](codex/intentional-divergences.md) | 固定 Codex 源码 revision、纯 Go A/B 身份与 continuation 边界、暂缓的传输层差异 |
| [OpenAI OAuth 与 Codex 桌面请求对照](codex/desktop-request-alignment.md) | 桌面静态报告的端点边界、OAuth principal 校验、HTTP 媒体类型与证明字段归属 |
| [Codex 桌面深度报告与请求还原补充](codex/ChatGPT_Codex_桌面端_深度分析报告_请求还原补充.md) | 完整头部清单、逐端点模板、真实值来源、bundle 证据与项目待补齐字段 |
| [Ghidra 原生证明与设备密钥对照](codex/native-device-key-alignment.md) | TPM/SE native 桥、controller step-up 与签名 challenge、CBOR 和静态报告校正 |
| [Codex CLI 语音](CODEX_VOICE.md) | `/voice` 的 V3 协议、Sideband 配置、分钟价格和结算恢复 |
| [CPA 多号池动态负载](CPA_POOL_DYNAMIC_LOAD_BALANCING_CN.md) | CPA 凭据容量、动态分流和运维检查 |
| [调度候选索引优化](SCHEDULER_CANDIDATE_INDEX_OPTIMIZATION_CN.md) | 实验调度引擎、索引一致性和回退行为 |
| [账号级 IPv6 出口](IPV6_EGRESS.md) | 稳定账号源地址、失败关闭、HE 6in4 和 Linux/Docker 路由边界 |
| [支付系统（中文）](PAYMENT_CN.md) / [English](PAYMENT.md) | 支付服务商、订单、回调和配置 |
| [上游同步审查记录（2026-08-19）](UPSTREAM_SYNC_20260819.md) | 冻结上游 SHA、语义移植、复用和迁移冲突决策 |
| [上游同步审查记录（2026-08-21）](UPSTREAM_SYNC_20260821.md) | 最新上游 PR 的选择性移植、性能边界、Docker 与发布验证 |
| [上游同步审查记录（2026-08-22）](UPSTREAM_SYNC_20260822.md) | 增量 PR 的兼容性筛选、性能边界、Docker 与精确 SHA 发布验证 |
| [上游同步审查记录（2026-08-25）](UPSTREAM_SYNC_20260825.md) | v0.1.181 主线 PR 的模块化移植、性能边界、Docker 与精确 SHA 发布验证 |
| [上游同步审查记录（2026-08-28）](UPSTREAM_SYNC_20260828.md) | v0.1.183 主线 PR 的选择性移植、平滑升级边界、Docker 与 CI 发布验证 |
| [PR #25 选择性同步审查记录（2026-08-30）](UPSTREAM_SYNC_20260830_PR25.md) | 自定义模型与媒体工坊移植、已替代差异、性能和升级边界 |
| [PR #25/#28/#29 安全审查（2026-08-31）](SECURITY_REVIEW_20260831_PR25.md) | 客服、能力令牌、自定义模板、媒体工坊和 Agnes 的安全发现、修复与验证 |
| [支付权限与公开入口安全修复（2026-10-07）](SECURITY_FIXES_20261007.md) | 商户配置加密、支付权限、限流、可信代理、结果图片与升级回退边界 |
| [Responses 图片编辑 SSRF 修复（2026-10-06）](SECURITY_REVIEW_RESPONSES_IMAGE_SSRF_20261006.md) | 用户图片/遮罩的独立公开地址策略、DNS 固定、代理与重定向边界、Docker 复现及回归 |
| [上游同步审查记录（2026-08-31）](UPSTREAM_SYNC_20260831.md) | v0.1.184 主线 PR 的模块化移植、性能边界、平滑升级、Docker 与 CI 发布验证 |
| [上游同步审查记录（2026-09-05）](UPSTREAM_SYNC_20260905.md) | v0.2.0 主线 PR 的选择性移植、已覆盖差异、性能边界、Docker 与 CI 发布验证 |
| [上游同步审查记录（2026-09-08）](UPSTREAM_SYNC_20260908.md) | v0.2.2 主线 PR 的选择性移植、平滑升级、性能 benchmark、Docker 与回退验证 |
| [上游同步审查记录（2026-09-09）](UPSTREAM_SYNC_20260909.md) | 新增主线 PR 的选择性移植、代理/Redis/协议性能边界、Docker 与回退验证 |
| [上游同步审查记录（2026-09-13）](UPSTREAM_SYNC_20260913.md) | 上游 main 至 `bdb42e22` 的选择性同步、WS/Gemini/认证性能与平滑升级边界、Docker 和 CI 跟踪 |
| [上游同步审查记录（2026-09-15）](UPSTREAM_SYNC_20260915.md) | 上游 main 至 `badfad8b` 的增量修复、国产渠道补齐、性能边界、Docker 与 CI 跟踪 |
| [上游渠道适配同步审查记录（2026-09-16）](UPSTREAM_SYNC_20260916.md) | 上游 main 至 `881f3202` 的渠道功能核对、多协议/OpenCode 补齐、Docker 运行态与浏览器验收 |
| [上游主线同步审查记录（2026-09-18）](UPSTREAM_SYNC_20260918.md) | 上游 main 至 `efe9aab1` 的协议/刷新/用量性能选择性移植、前端稳定性修复、Docker 与 CI 跟踪 |
| [上游主线同步审查记录（2026-09-23）](UPSTREAM_SYNC_20260923.md) | 上游 main 至 `7c0a2a55` 的逐 PR 台账、选号性能与用户属性请求隔离、Docker 和 CI 跟踪 |
| [上游主线同步审查记录（2026-09-27）](UPSTREAM_SYNC_20260927.md) | 11 个上游 PR 的已覆盖/已移植/暂缓台账、加密与视频价格修复、Docker 及发布证据 |
| [上游主线同步审查记录（2026-09-30）](UPSTREAM_SYNC_20260930.md) | 48 个冻结主线 PR 的当前 owner 台账、协议/统计与工具改写性能修复、Docker 升级回退和 CI |
| [上游主线同步审查记录（2026-10-05）](UPSTREAM_SYNC_20261005.md) | 12 个增量 PR 的关闭台账、验证码原子重构、平滑升级、Docker 与 CI 跟踪 |
| [上游主线同步审查记录（2026-10-07）](UPSTREAM_SYNC_20261007.md) | 3 个增量 PR 的首次安装适配、已覆盖差异关闭、性能与 Docker 升级回退验证 |
| [上游主线同步审查记录（2026-10-08）](UPSTREAM_SYNC_20261008.md) | 57 个冻结主线 PR 的关闭/重开台账、WS 逐轮计费、TPS 与 namespace 性能、Docker 平滑升级和发布跟踪 |
| [上游主线同步审查记录（2026-10-09）](UPSTREAM_SYNC_20261009.md) | 两个冻结 PR 的精准修复、安全工具链、搜索历史性能、关闭/专项暂缓台账和 Docker 验证 |
| [上游主线同步审查记录（2026-10-12）](UPSTREAM_SYNC_20261012.md) | 17 个增量 PR 的当前 owner 适配、分组多语言、计费与审核并发性能、Docker 与升级回退边界 |
| [上游主线同步审查记录（2026-10-13）](UPSTREAM_SYNC_20261013.md) | 7 个增量 PR 的当前 owner 适配、明确暂缓、范围隔离、Docker 升级回退与 CI |
| [上游主线同步审查记录（2026-10-15）](UPSTREAM_SYNC_20261015.md) | 10 个冻结 PR 的当前 owner 适配、图片解码重构、升级回退和关闭/专项暂缓台账 |
| [上游主线同步审查记录（2026-10-16）](UPSTREAM_SYNC_20261016.md) | 两个支付 PR 的模块适配、pending 退款事务、易支付确认、混合版本边界与 Docker 验证 |
| [上游主线增量审查（2026-10-16，后续批次）](UPSTREAM_SYNC_20261016_FOLLOWUP.md) | Gemini 思考、订阅退休、搜索和 Responses 时间戳的当前 owner 适配、性能及升级回退证据 |
| [上游主线增量同步审查（2026-10-19）](UPSTREAM_SYNC_20261019.md) | Gemini 断流可靠结算、缓存用量下限、图片结果类型检查、关闭台账与 Docker 升级回退 |
| [上游主线增量同步审查（2026-10-22）](UPSTREAM_SYNC_20261022.md) | service account 配额、Gemini 图片门禁、兼容流断连结算、性能与 Docker 升级回退 |
| [上游 #8170 专项同步（2026-10-22）](UPSTREAM_SYNC_20261022_PR8170.md) | Grok 4.7 Fast 价卡、长上下文边界、动态/渠道价格优先级与回退 |
| [上游主线增量同步审查（2026-10-24）](UPSTREAM_SYNC_20261024.md) | #8305 有界重试等待、已覆盖差异关闭、显式身份兼容与 Docker 升级回退 |
| [上游主线同步审查（2026-10-26）](UPSTREAM_SYNC_20261026.md) | 图片按张计费、密钥独立限流、资源级授权专项边界、Docker 升级回退与 CI |
| [上游主线增量同步审查（2026-10-27）](UPSTREAM_SYNC_20261027.md) | Terra 兼容缺口、Grok 目录诊断与模型审计、已有快照复用、Docker 与 CI |
| [上游主线增量同步审查（2026-10-28）](UPSTREAM_SYNC_20261028.md) | 10 个增量 PR 的当前 owner 适配、计费与协议修复、性能和 Docker 升级回退 |
| [上游主线增量同步审查（2026-10-29）](UPSTREAM_SYNC_20261029.md) | Gemini 重试、OAuth 非流式 Responses、管理员保护、渠道配置保留、性能边界与 Docker 验证 |
| [上游主线增量同步审查（2026-10-30）](UPSTREAM_SYNC_20261030.md) | 管理 MCP PR 的分层/性能审查、前端依赖安全覆盖、平滑升级与 Docker 验证 |
| [API Key 删除与计费竞争条件审查（2026-10-06）](../diagnostics/billing-key-deletion-20261006/REPORT.md) | 旧版本漏扣回放、当前修复边界、真实数据库与 24 个 Docker HTTP 场景 |
| [GPT-6 Astra 接入记录](GPT6_ASTRA.md) | OpenAI 官方模型目录、能力、价格和本项目兼容计费映射 |
| [GPT-6 模型目录与价格](GPT6_MODELS.md) | GPT-6 Astra、Sol、Luna、GPT-6.1 Sol/Terra 的官方能力、价格与本项目实现位置 |
| [OpenAI 请求计时](OPENAI_TIMING.md) | 上游 timing 采集、用户明细投影与管理员本地/上游对比 |


## 长期优化计划

| 文档 | 内容 |
| --- | --- |
| [前端架构优化计划](FRONTEND_ARCHITECTURE_OPTIMIZATION_PLAN.md) | Feature owner 收口、DTO、Query/Action、依赖门禁与分阶段迁移路线 |
| [网关性能优化与 Rust 评估](GATEWAY_PERFORMANCE_OPTIMIZATION_CN.md) | JSON/WS/SSE 热路径优化、Rust 候选边界与 Docker 对照证据 |

处于方案或交付阶段的功能记录在 `openspec/changes/`。使用这些文档前，应同时核对实现和测试，不能把 proposal 当作当前运行事实。

## 部署与运维

| 文档 | 内容 |
| --- | --- |
| [部署索引](../deploy/README.md) | 部署方式和配置入口 |
| [Docker 部署](../deploy/DOCKER.md) | Compose 部署与升级 |
| [多实例部署](../deploy/MULTI_INSTANCE.md) | 多实例拓扑、共享依赖与管理页指标速查 |
| [Redis 调优](../deploy/REDIS_TUNING.md) | 小机器默认值和高吞吐调优 |
| [边缘安全](../deploy/EDGE_SECURITY.md) | 反向代理与入口防护 |
| [二开前端同源与 CORS 部署指南](FRONTEND_CORS_DEPLOYMENT_CN.md) | 避免前端静态资源、API、WebSocket 和登录链路出现大面积跨域错误 |
| [Apple container](../deploy/APPLE_CONTAINER.md) | macOS container 部署 |

生产配置字段以 `deploy/config.example.yaml` 为准，Compose 环境变量以 `deploy/.env.example` 为准。

## 合规文档

- [中文部署与运营合规承诺](legal/admin-compliance.zh.md)
- [English deployment and operation compliance commitment](legal/admin-compliance.en.md)

## 文档维护约定

本次指定 PR 的审查、升级回滚与性能证据见 [PR #153 渠道监控同步审查](PR153_CHANNEL_MONITOR_REVIEW.md)。

- 代码是事实源，文档负责解释边界、入口和决策，不复制大段实现。
- 新增专题文档时在本页登记，并说明目标读者和事实源。
- 新增前端 feature 或后端垂直模块时，补齐目录 README 并登记到 [功能索引](FEATURES.md)；`make check-docs` 会检查覆盖和链接。
- 修改目录结构、启动入口、验证命令、请求链路或配置来源时，同步更新相关文档。
- 每个后端目录 README 维护“职责和文件前缀”，不要逐个列出所有文件。
- 文档中的命令必须标明执行目录；示例凭据只能使用占位符。
- 提交前在仓库根目录运行 `make check-docs`。
