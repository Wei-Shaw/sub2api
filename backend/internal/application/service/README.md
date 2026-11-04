# Application Services

本包是现有业务兼容层，集中保存应用端口、核心用例和跨模块编排。package 名仍为 `service`，以保持现有构造器和调用方稳定。

## 文件索引

| 前缀 | 职责 |
| --- | --- |
| `account*`, `admin_account*`, `admin_group*`, `admin_user*` | 账号与管理用例 |
| `auth*`, `oauth*`, `token*`, `totp*` | 身份、会话和凭据刷新 |
| `gateway*` | Anthropic/Claude 通用网关请求、调度、转发和计费 |
| `gateway_systemone.go`, `account_test_service_typesafe.go` | TypeSafe 原生非流式转发与账号测试，复用完整账号出口和可靠计费 |
| `openai*` | OpenAI/Codex/Responses/Images/WS 管线 |
| `openai_request_set_patches.go` | 大 JSON 的多项普通 set 局部改写与一次拼接，保留 sjson 回退路径 |
| `openai_capacity_retry.go` | 请求级容量错误的 HTTP/SSE/WS 等待提示解析；有限数值先限幅，沿用现有重试预算 |
| `openai_ws_size_recovery.go` | 内嵌图片/大 JSON 请求发送前的 HTTP 选路、1009/断连后一次安全回退与完整历史重放边界 |
| `openai_ws_fallback_response.go` | WS 终止错误的 HTTP/SSE 映射、1009 大小限制提示与已提交响应保护 |
| `openai_live*` | Codex CLI/旧 Desktop Live、Sideband 控制、分钟结算和持久恢复 |
| `gemini*`, `grok*`, `antigravity*`, `bedrock*` | 各上游协议适配与重试 |
| `scheduler*`, `channel*`, `ratelimit*`, `concurrency*` | 调度、通道和并发控制 |
| `billing*`, `usage*`, `pricing*`, `subscription*` | 计费、用量和订阅 |
| `media_studio*`, `model_attribute*`, `multimodal_media*`, `video_balance_hold*` | 媒体配置/预估、编译模板属性、有界媒体协议与视频冻结编排；浏览器历史不入服务端存储 |
| `payment*`, `batch_image*` | 支付应用编排与批量图片任务 |
| `ops*`, `audit*`, `content_moderation*` | 运维、审计和内容策略 |
| `setting*`, `notification*`, `backup*` | 配置、通知和维护用例 |
| `proxy_auto_assignment*`, `proxy_health*` | 代理池强制均衡分配、定期测活和失活迁移 |
| `oauth_model_sync_service.go` | OpenAI OAuth 实时模型能力快照与定时同步 |
| `oauth2_provider.go` | 管理员控制的 OAuth2 客户端、PKCE 授权码、token 校验与 userinfo |
| `cluster*` | 稳定节点身份、心跳清单、任务租约、就绪门禁与串行版本发布 |
| `wire.go` | application provider 集合 |

### 核心拆分索引

| 文件组 | 职责 |
| --- | --- |
| `content_moderation.go` | 内容审核常量、传输模型、端口和服务状态 |
| `content_moderation_config_api.go`, `content_moderation_config_rules.go`, `content_moderation_validation.go` | 配置读写、默认规则和校验 |
| `content_moderation_check.go`, `content_moderation_queue.go` | 同步审核决策与异步任务处理 |
| `content_moderation_runtime.go`, `content_moderation_cleanup.go`, `content_moderation_admin.go` | 运行快照、清理和管理查询 |
| `content_moderation_client.go`, `content_moderation_key_health.go` | 上游审核 API 和密钥健康状态 |
| `content_moderation_side_effects.go`, `content_moderation_cyber_policy.go` | 命中后的账户、通知和网络安全事件处理 |
| `content_moderation_test_input.go` | 管理端测试输入和确定性评分辅助 |
| `setting_parse.go`, `setting_parse_core.go` | 持久设置默认值、解析编排与基础站点设置 |
| `setting_parse_identity.go`, `setting_parse_oidc.go` | LinuxDo、DingTalk、OIDC、OAuth 与微信身份源设置 |
| `setting_parse_features.go`, `setting_parse_gateway.go`, `setting_parse_notifications.go` | 功能开关、网关调度与通知展示设置 |
| `setting_update.go`, `setting_update_prepare.go` | 持久设置更新编排、首错顺序与跨域预处理 |
| `setting_update_core.go`, `setting_update_identity.go`, `setting_update_product.go` | 注册访问、身份源与产品默认设置写入 |
| `setting_update_gateway.go`, `setting_update_notifications.go` | 网关调度、通知与平台额度设置写入 |
| `setting_codex_simulation.go`, `setting_codex_turn_state_replay.go`, `openai_codex_turn_state_auto_replay.go`, `openai_codex_turn_state_probe.go` | Codex A/B/C 数据库覆盖、强制关闭、Turn State 手工随机池/质量同步、关注模型的可配置字符长度自动池、首次无效立即探测与 45 分钟刷新、默认走账号正常出口的 32 目标乘 4 尝试有界并发、可选专用/代理池竞速和 5 秒无限轮次恢复，以及身份密钥生成与后台同步的无 DB 热路径快照 |
| `wire_openai_gateway.go` | OpenAI gateway provider、代理库与跨实例探测锁装配及后台探测启动 |
| `api_key_group_routing.go` | API Key 有序分组候选、倍率保护过滤和请求内实际分组切换 |
| `openai_codex_request_headers.go`, `openai_codex_metadata_projection.go`, `openai_gateway_request_decode.go`, `openai_ws_handshake_compatibility.go` | OAuth 出站身份/metadata 投影、metadata 保精度解码与 WS 不可变握手身份 |
| `openai_codex_identity_plan.go`, `openai_codex_simulation_state.go` | Codex request root、per-principal 身份计划与短状态 fallback |
| `openai_codex_cache_identity.go` | 按主体与会话 root 原子固定首次 UUIDv7，会话/缓存键跨请求、重连和实例复用及有界本地 fallback |
| `openai_codex_continuation.go` | Codex continuation 分类、owner 策略、跨主体 sanitizer 与成功回写 |
| `openai_oauth_gateway_rate_limit.go` | 统一配置、按账号分桶且跨实例共享的 OpenAI OAuth RPM/burst 准入、逻辑请求去重与协议错误投影 |
| `openai_request_timezone.go`, `setting_openai_request_timezone.go` | OpenAI HTTP/WS 用户环境与搜索时区替换、管理员全局开关与 IANA 目标、60 秒缓存及写后立即刷新 |
| `openai_request_integrity_observe.go` | OpenAI OAuth 兼容转换前后语义字段的只观察差异诊断 |
| `openai_chatgpt_headers.go`, `openai_codex_request_headers.go`, `openai_codex_metadata_sanitize.go` | OpenAI OAuth 凭据 principal 校验、HTTP JSON/SSE/Compact 媒体类型和证明/完整性状态的跨账号边界 |
| `openai_integrity_state.go`, `openai_oauth_proof.go`, `openai_proof_binding.go`, `openai_desktop_devicecheck.go` | 发送点 proof 生成、immutable receipt、principal/installation/origin 状态隔离与 Redis 原子 CAS；独立 Desktop DeviceCheck 注册/cookie 链 |
| `openai_quota_account_transport.go` | OpenAI quota/reset 通过账号级 HTTP/TLS upstream 的辅助请求路径 |
| `account_quality_runtime_snapshot.go` | 质量巡检私有 artifact 的非敏感请求运行画像快照 |
| `ratelimit_oauth_401_delete.go` | 网关 OAuth 401 自动删除准入、凭据 CAS 结果和运行态清理 |
| `openai_codex_remote_control_client.go` | 绑定 OAuth credential owner、安装状态与账号出口的 native controller client 工厂 |
| `openai_codex_remote_control_store.go` | Codex Remote Control enrollment token 的账号级加密持久化适配 |
| `openai_gateway_forward.go`, `openai_gateway_forward_prepare.go`, `openai_gateway_request_build.go` | OpenAI 转发编排与 HTTP 上游请求构造 |
| `openai_responses_image_api_bridge.go`, `openai_responses_image_plan.go`, `http_upstream_profile.go` | Responses 图片/遮罩解析、共享下载与单图子请求；用户远程输入携带独立公开地址策略，管理员私网上游配置不放宽此策略 |
| `invalid_auth_abuse_limiter.go`, `cloudflare_ingress_settings.go` | 无效 API Key 来源计数、本地临时封禁，以及 Access Rule/WAF 双模式的 Cloudflare 加密持久设置与边缘端口 |

## 拆分约定

JSON patch、WS 握手快照复用及流式编解码的性能边界与 Docker 对照见 [网关性能优化](../../../../docs/GATEWAY_PERFORMANCE_OPTIMIZATION_CN.md)。

`openai_request_integrity_observe.go` 在诊断准入后捕获原始请求快照；`image_output_accounting_view.go` 为图片计数提供局部只读投影，并在重复字段时保留原路径查询语义。WS JSON 修复/校验状态由 `openai_sse_json_documents.go` 管理，envelope 的持久身份字符串独立保存。

单个功能按 `types/plan/request/forward/response/billing/runtime` 职责拆文件，不按“公共 helper”堆积。新增功能若不需要访问本包大量私有状态，应建立 `modules/<domain>` 并通过接口接入。

本包禁止导入 `internal/infrastructure/repository`；例外只能在 lint 配置中显式记录并附迁移原因。

Codex manifest 的显式管理员模型映射投影归属 `openai_codex_models_mapping.go`；不修改共享缓存、不启用 pinned 聚合前置。WS 逐轮分组价格刷新与混合版本 TPS 行为见 [本轮同步审查](../../../../docs/UPSTREAM_SYNC_20261008.md)。

OAuth 历史 `web_search_call` 的缓存搜索声明由 `openai_oauth_web_search_history.go` 负责；标准/Lite、HTTP/WS、compact/API-key 隔离和工具链修补分支见 [2026-10-09 同步记录](../../../../docs/UPSTREAM_SYNC_20261009.md)。

GPT-6.1 Sol 的订阅模型长上下文豁免由 `openai_subscription_long_context.go` 判断实际凭据与发送模型，通过 `CostInput.DisableModelLongContextPricing` 接入既有计费流程；自定义价格区间保留。分组展示译文由 `group_i18n.go` 规范化，HTTP 输出负责语言选择。同步和回退证据见 [2026-10-12 同步记录](../../../../docs/UPSTREAM_SYNC_20261012.md)。

`subscription_progress.go` 保存订阅进度值；名称译文仅作为不序列化的输出参数从已加载分组带到 handler，单条与批量用户进度保持同一语言规则，不增加数据库查询。

`payment_fulfillment.go` 在本地订单校验后对首次可支付的回调执行同实例确认，已完成回调和上游查询对账复用原有路径。`payment_refund.go` 将 pending 扣减凭证和状态同事务保存，成功不重复扣减，失败同事务归还；被撤销的订阅通过既有订阅 owner 恢复。旧审计读取、混合版本退款管理暂停与回退前结清要求见 [2026-10-16 同步审查](../../../../docs/UPSTREAM_SYNC_20261016.md)。

临时不可调度的可选规则范围由 `temp_unsched_scope.go` 选择；重试耗尽、Bedrock 和 token 计数透传调度器使用的模型键。`openai_responses_input_rejection.go` 只识别转换后结构化 input 的明确类型拒绝，Chat 回退保留原始开始时间。冻结差异、滚动升级启用顺序和新缓存能力的专项边界见 [2026-10-13 同步审查](../../../../docs/UPSTREAM_SYNC_20261013.md)。

`apicompat_config.go` 将可配置输出下限带入现有协议转换；`gateway_anthropic_credentials.go` 在原生 Anthropic 构造器和 token 计数入口检查凭据协议。`gemini_vertex_token_error.go` 保存固定 token 端点的明确 SA 失效分类，错误策略结果只在同一次响应内复用。`billing_gemini_argon.go` 与 `pricing_antigravity_aliases.go` 分别保存 Argon 回退价和模型别名规范化。升级、端点发现与未关闭子项见 [2026-10-15 同步审查](../../../../docs/UPSTREAM_SYNC_20261015.md)。

`gemini_compat_thinking.go` 管理兼容入口的档位与历史思考清理；实际转发配置决定用量展示。`openai_model_retirement.go` 复用当前映射链检查 ChatGPT 退休目标，`openai_responses_created_at.go` 仅修补普通兼容出口缺失时间戳。搜索模拟的共享 gate/writer 保留完整账号出口；详见 [本轮增量审查](../../../../docs/UPSTREAM_SYNC_20261016_FOLLOWUP.md)。

`gemini_messages_stream.go`、`gemini_native_stream.go` 和 `gemini_stream_client_abort.go` 分别维护 Messages/Native SSE、请求内写失败和下游取消后的累计用量保留；Chat 保持已支持的图片 Markdown，并只对中断路径按成功写出数计图。`image_storage.go` 继续通过既有 public-destination HTTP 端口下载结果，拒绝声明、嗅探均非图片的内容。缓存专有用量、性能、升级和关闭边界见 [2026-10-19 同步审查](../../../../docs/UPSTREAM_SYNC_20261019.md)。

`gateway_forward_compat_stream.go` 和 `gateway_forward_compat_idle.go` 管理 Anthropic 兼容流的字节级扫描、阻塞读取超时和断连后最终用量；`account_quota_support.go`、`request_metadata_single_account.go` 分别维护 service account 配额类型和请求内延迟单账号探测。Gemini 图片权限、工具 schema 和关闭台账见 [2026-10-22 同步审查](../../../../docs/UPSTREAM_SYNC_20261022.md)。

`billing_grok_fast.go` 持有 Grok 4.7 Fast 的明确 ID 与内置兜底价；捆绑快照使用本项目已支持的显式长上下文字段。两个 Fast ID、边界与价格优先级见 [#8170 同步记录](../../../../docs/UPSTREAM_SYNC_20261022_PR8170.md)。

`grok_model_catalog.go` performs strict Grok catalog parsing and computes a source fingerprint. It reuses modelcatalog persistence and the existing OAuth model worker. Detail diagnostics are pure; `openai_compat_tool_model.go` enforces mapped Sol/Terra tools on Responses-capable paths. See [2026-10-27 sync](../../../../docs/UPSTREAM_SYNC_20261027.md).

Claude 指纹的 UTF-16 前缀、Gemini fileData 计图、DeepSeek Pro/缓存读取、OpenCode 角色、模型广场固定价及上传完整性见 [2026-10-28 同步记录](../../../../docs/UPSTREAM_SYNC_20261028.md)。非流式畸形工具调用复用有界 collector 并返回明确 502；没有替换既有调度与可靠结算。

`api_key_auth_lookup.go` 拥有 API Key 入口查找；`api_key_auth_bloom.go` 在本地鉴权缓存之后、Redis L2/数据库之前预筛不存在的凭据，维护有界后台预热与回退计数。共享位图只用于否定判定，可能存在时仍走完整鉴权。

`gemini_declared_models.go` 对原生模型元数据复用单次账号读取；Gemini 可配置同号重试、渠道平台维度合并和 OAuth 非流式终止响应聚合见 [2026-10-29 同步记录](../../../../docs/UPSTREAM_SYNC_20261029.md)。

`mcp_oauth*` owns OAuth/PKCE management authorizations and repository ports; tool schemas remain in `modules/managementmcp` and HTTP/SDK adaptation stays in transport.
