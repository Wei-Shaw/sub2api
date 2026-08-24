## 1. 契约与迁移

- [x] 1.1 增加 provisioning state、policy/profile/slot/binding SQL migration 与 Ent schema
- [x] 1.2 定义强类型 provisioning/profile/session/proxy/attempt-plan 契约和稳定错误码
- [x] 1.3 添加 OpenSpec capability requirements 与 ADR

## 2. 原子 Provisioning

- [x] 2.1 实现统一 AccountProvisioningService 和 repository 原子 create/update/activate
- [x] 2.2 接入普通创建、编辑、OAuth/PAT、Codex session/RT、JSON、CRS、批量入口
- [x] 2.3 确保 pending 在 DB、scheduler cache、sticky 和所有候选路径不可调度
- [x] 2.4 添加并发导入、回滚、outbox、seed、proxy 和幂等测试

## 3. Profile、设备与会话

- [x] 3.1 实现 classifier、封闭 catalog、policy validator 和 compatible account filter
- [x] 3.2 实现稳定 slot resolver、binding、epoch/draining 和 proxy precedence
- [x] 3.3 实现四种 session policy 与并发约束
- [x] 3.4 实现 Attempt Plan、HMAC 映射和旧模式兼容

## 4. Adapter 与响应恢复

- [x] 4.1 实现 Windows/macOS/Linux Desktop/CLI 和 Generic SDK/Third-party Profile Adapter
- [x] 4.2 接入 HTTP 非透传、透传、compact、SSE 和 WS 所有请求构造路径
- [x] 4.3 实现 JSON/SSE/WS 结构化 response restorer
- [x] 4.4 添加跨 OS 禁止、header/body一致性、workspace和普通文本不误改测试

## 5. Affinity 与 failover

- [x] 5.1 新增 Profile sticky namespace、TTL 和 account+slot binding
- [x] 5.2 在 previous-response、sticky 和普通候选路径执行 Profile兼容过滤
- [x] 5.3 扩展第二回合429测试，验证同 Profile换号、绑定更新和不反跳
- [x] 5.4 回归 PR #2 HTTP/WS/state隔离与账号级预算

## 6. 前端

- [x] 6.1 新增强类型 API/types/composable 和共享 CodexIdentityPolicyEditor
- [x] 6.2 接入创建、编辑、OAuth、PAT、session、RT、JSON、CRS和批量编辑
- [x] 6.3 实现 Profile槽位、代理覆盖、会话策略、pending/active、draining和逐行导入状态
- [x] 6.4 添加桌面/窄屏、键盘、可访问性、payload矩阵和失败状态测试

## 7. 验证与发布

- [x] 7.1 运行 OpenSpec strict validation
- [x] 7.2 运行聚焦 Go unit/integration/race 和迁移测试
- [x] 7.3 运行 frontend lint/typecheck/Vitest/build
- [ ] 7.4 运行完整 GitHub CI
- [ ] 7.5 提交并推送独立分支，创建以 PR #2 分支为 base 的 Draft PR
- [ ] 7.6 按目标逐项审计证据；不部署生产
