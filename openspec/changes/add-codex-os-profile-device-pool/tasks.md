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
- [x] 7.4 运行插件 Go test/vet、Node checks、包 hash 校验和 unsigned 本地安装/进程集成测试
- [x] 7.5 静态核对 `usage_logs` schema/insert path 与当前官方主线一致
- [x] 7.6 在 Linux Go 1.27 环境运行 WS/Profile/plugin 聚焦 race；完整 service race 另暴露三个未改文件中的既有全局测试变量竞态
- [ ] 7.7 运行完整 GitHub CI
- [ ] 7.8 整理并提交基于当前官方主线的独立分支，创建 Draft PR
- [x] 7.9 按最终目标逐项审计证据；本 change 不部署生产
- [ ] 7.10 后续插件发布使用生产发布密钥生成签名包；不阻塞本核心 PR
- [ ] 7.11 后续运行真实 A2 usage-relay 到 B2 durable receipt/replay 非生产验收；需要另行授权且不阻塞本核心 PR

## 8. 插件前向兼容

- [ ] 8.1 刷新官方远端并将核心提交重放到当前官方主线
- [x] 8.2 定义 0.1.x 协议驱动兼容策略，区分 hard range、精确协议门和 tested 证据
- [x] 8.3 将插件版本提升为 0.1.5，并增加未来补丁版、0.2.0 prerelease 和协议不匹配测试
- [x] 8.4 重新构建插件并运行后端、前端、插件、OpenSpec 与账务不变量验收
- [x] 8.5 完成独立审查和干净本地提交

## 9. 模板控制面与多 Surface

- [x] 9.1 新增设置页命名模板 CRUD、revision 乐观锁和被引用删除保护
- [x] 9.2 账号创建、编辑和批量更新改为模板 assignment；模板更新经确认后事务传播
- [x] 9.3 Profile、binding、affinity 与 WS state 从 OS-only 升级为 `(OS, surface)`
- [x] 9.4 Windows/macOS/Linux Desktop 与 CLI 可同时启用并独立配置槽位/代理
- [x] 9.5 增加 233/234 正向迁移、旧策略模板迁移和旧二进制数据降级脚本
- [x] 9.6 运行真实 PostgreSQL 正向、幂等与降级演练
