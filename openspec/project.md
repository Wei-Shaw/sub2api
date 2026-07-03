# Project Context

## Purpose

Sub2API 是一个 AI API 网关，负责把多种上游账号能力包装成统一 API，对外提供认证、计费、调度、限流、并发控制和协议兼容。当前项目重点之一是稳定承接 Codex 官方客户端流量，在不破坏官方语义的前提下提供低延迟中转。

## Tech Stack
- Go 1.25.x, Gin, Ent
- Vue 3, Vite, TailwindCSS
- PostgreSQL, Redis
- WebSocket / SSE / HTTP 协议转发

## Project Conventions

### Code Style
- 标识符与类型名使用英文，设计说明与提交说明优先中文
- 代码注释尽量少，只在复杂状态机或协议兼容逻辑前补充必要说明
- 兼容层优先保持输入输出语义稳定，不用“大而全”抽象掩盖协议差异

### Architecture Patterns
- `handler` 负责请求校验、鉴权上下文和响应编码，`service` 负责路由、调度和协议转发
- 账号调度、会话粘连、传输连接池、协议修正属于不同职责边界，不应共享隐式状态
- WebSocket 转发中的 `session/thread`、`turn`、`transport connection` 是不同层级，不能混为同一状态机

### Testing Strategy
- 优先补服务层单测与 WebSocket 回归测试，覆盖请求改写、重试、恢复和事件流
- 对协议兼容问题，测试必须验证实际上游请求内容，而不只验证最终返回状态
- 涉及 Codex / OpenAI Responses 的改动，至少覆盖普通 turn、`previous_response_id`、`function_call_output` 三类场景

### Git Workflow
- 架构调整、行为变化、性能优化先写 OpenSpec proposal，再实现
- 提交按功能边界原子化，不把无关重构与协议修复混在一起
- 若改动影响兼容行为，必须先补回归测试再改生产代码

## Domain Context

Sub2API 需要同时服务通用 OpenAI 兼容客户端和 Codex 官方客户端。Codex 官方客户端对 `session_id`、`x-codex-turn-state`、`previous_response_id`、工具调用链等语义更敏感，不能简单复用普通 OpenAI 兼容逻辑。

## Important Constraints
- 低 TTFT 不能以破坏请求语义为代价，尤其不能靠删除工具链锚点来“优化”
- 非 Codex 客户端的现有兼容行为应尽量保持不变
- `prompt_cache_key` 可以作为缓存提示，但不应自动提升为 Codex 会话主键
- 跨 turn 状态必须显式、可推理，不能依赖连接池偶然复用

## External Dependencies
- OpenAI Platform `/v1/responses`
- ChatGPT `backend-api/codex/responses`
- Redis 会话/限流缓存
- PostgreSQL 持久化与后台管理数据
