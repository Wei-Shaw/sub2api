# AI Agent 运行边界与验证入口

本文说明内置管理端 AI Agent 当前可承诺的执行、回滚和部署边界。它是实现约束，不是功能愿景。

## 执行拓扑

Agent 配置接口返回：

```json
{
  "execution_topology": "single_instance",
  "multi_instance_safe": false
}
```

首版后台运行句柄、即时取消信号和并发槽属于当前 Go 进程。持久化记录可以在进程重启后恢复为可审计状态，但当前没有跨实例 lease、heartbeat、fencing token 或取消消息投递。

生产部署必须满足以下条件之一：

- 只运行一个负责 Agent 的后端实例。
- 在负载均衡层对同一管理员/会话做粘滞路由，并保证重启恢复不会被多个实例同时执行。
- 关闭 `ai_agent_enabled`。

在加入经过故障注入验证的分布式租约前，不应把同一 Agent 会话同时交给多个实例。后续多实例设计至少需要：唯一 owner、递增 fencing token、租约续期、过期接管、跨实例取消和幂等恢复。

## 回滚支持级别

`GET /api/v1/admin/ai-agent/rollback-capabilities` 返回所有写操作的机器可读矩阵。精确操作契约也包含 `rollback_support`。

### conditional

后端在运行时满足全部条件后，可以生成确定性补偿：

- `restore_fields`：PUT/PATCH 前后都能读取同一路径资源，且能确认 Agent 改动字段。
- `delete_created`：POST 响应返回稳定资源 ID，目录中存在匹配的审计删除接口。
- `rollback_plan`：多个已验证补偿按原计划依赖逆序组合。

`conditional` 不表示无条件可回滚。资源 ID 缺失、响应形状不匹配、目标不可读或后续管理员改动都会使补偿不可用或被漂移检查拒绝。

### assisted

后端没有足够信息证明确定性逆操作安全。Agent 可以基于保存的执行记录生成恢复建议，但：

- 必须由管理员确认。
- 即使 `auto_approve=true` 也不会自动执行。
- 不得声称是原子回滚。
- 不得覆盖无法解释的后续修改。

### unavailable

当前没有安全恢复路径。典型场景包括：

- DELETE 前没有可安全重建的完整快照。
- 支付、邮件、对象存储、外部 Provider 或异步任务已经产生副作用。
- 批量响应不能稳定映射每一个被修改资源。
- 创建接口没有稳定 ID 或匹配的删除接口。

审计记录证明发生过什么，不等于保证可以恢复。

## 隔离破坏性验证

项目使用 Testcontainers 启动独立 PostgreSQL 和 Redis，不连接开发或生产资源。

运行 Agent 专用持久化生命周期测试：

```bash
cd backend
make test-agent-destructive
```

该测试验证：

- PostgreSQL 中的 Agent 会话使用 AES-256-GCM 密文保存。
- 新 Service 实例能够恢复持久化会话。
- 删除会话后不会再次出现在历史中。
- Redis canary 只存在于测试命名空间并可完整清理。

运行仓库全部 integration 测试：

```bash
cd backend
make test-integration
```

这套入口建立了可重复的隔离环境，但尚未逐个执行全部 229 个写操作。新增高风险写工作流时，应补充成功、拒绝、幂等、部分失败、进程中断和补偿漂移用例。

## Provider 兼容验证

默认单元测试覆盖 Chat Completions、Responses 和 Messages 的原生 SSE，包括注释/心跳、`event:` 行、CRLF、无效帧、usage、reasoning 和签名 thinking block。

真实兼容服务测试通过环境变量显式启用：

```bash
cd backend
AI_AGENT_COMPAT_CHAT_BASE_URL=... \
AI_AGENT_COMPAT_CHAT_API_KEY=... \
AI_AGENT_COMPAT_CHAT_MODEL=... \
AI_AGENT_COMPAT_RESPONSES_BASE_URL=... \
AI_AGENT_COMPAT_RESPONSES_API_KEY=... \
AI_AGENT_COMPAT_RESPONSES_MODEL=... \
AI_AGENT_COMPAT_MESSAGES_BASE_URL=... \
AI_AGENT_COMPAT_MESSAGES_API_KEY=... \
AI_AGENT_COMPAT_MESSAGES_MODEL=... \
go test -tags=integration ./internal/service -run TestAIAgentLiveProviderStreamingCompatibility -v
```

缺少某组凭证时对应协议明确 skip。测试凭证不得写入配置文件、日志、测试快照或 PR。

## 路由和契约漂移

`TestAIAgentCatalogMatchesRegisteredAdminRoutes` 使用 Go AST 读取管理路由注册代码，并将真实 Method/Path 集合与 Agent 目录双向比较。新增、删除或改名管理路由但没有同步 Agent 契约时，CI 会失败。

当前目录覆盖 396 个管理操作，其中 229 个写操作；每个目录项都必须存在 body/query/path 分类记录。字段摘要不是完整契约，执行前仍以精确契约和现有 Handler 校验为准。
