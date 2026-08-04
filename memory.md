# 代理池批量绑定功能记录

## PR 评估

- 参考实现：`Wei-Shaw/sub2api#5253`，已获取 `upstream/pr-5253`（`6c6210a74`）及 merge ref（`b744d2f8f`）。
- 当前项目基线为 `00c08c574`。PR 基于更晚的仓库版本，混有大量 Ent 生成文件、迁移编号和无关业务变更，不能安全地直接 cherry-pick。
- 原 PR 提供代理池健康探测和重绑，但没有“选择多个账号后批量绑定代理池”的完整工作流。因此按当前版本结构移植核心机制，并补齐批量 API 和管理界面。

## 当前设计

- 迁移 `backend/migrations/092_proxy_pools.sql` 新增代理池、池成员健康字段、账号池绑定和重绑日志。
- `accounts.pool_id` 表示长期池绑定；实际请求仍读取现有 `accounts.proxy_id`，无需修改网关热路径。
- 批量绑定会过滤重复账号 ID，并按健康代理当前账号数选择最低负载目标；负载相同时按代理 ID 稳定分配。
- 后台每 30 秒扫描启用的池，各代理按池配置的检测间隔探测。探测成功立即恢复健康并清零失败次数；连续失败达到阈值后才标记异常。
- 开启 `auto_rebind` 时，异常或停用代理上的池账号会自动切换到健康代理；账号快照通过 scheduler outbox 按实际受影响 ID 刷新。
- 手动“立即检测”会强制检查全部启用成员。批量绑定会先检查未知或过期的成员，避免把账号分到未经验证的代理。
- PostgreSQL advisory lock 是生产环境主锁；无数据库构造场景才回退 Redis。另有进程内同池互斥，避免后台扫描、手动检测和批量绑定重复探测或重复累计失败。
- 请求被取消导致的探测错误不计入代理失败次数。Redis 延迟缓存不可用时仅降级出口信息展示，不影响数据库健康状态。
- 池成员响应使用脱敏 DTO，不返回代理密码；数据库唯一冲突映射为 `PROXY_POOL_NAME_EXISTS`。

## 产品边界

- 后端支持 `DELETE /api/v1/admin/proxy-pools/:id/accounts` 显式解绑；解绑保留最后使用的 `proxy_id`，只停止后续自动切换。
- 当前账号页提供批量绑定/重新绑定入口，尚未增加批量解绑按钮。
- 当前版本 Ent 的 Account 模型不知道新增的 `pool_id`。通用账号编辑器显式修改 `proxy_id` 时不会同时清除池绑定，之后池调度仍可能覆盖该值；需要解绑时应调用代理池解绑接口。若要在通用编辑器中统一处理，应单独设计明确的“退出代理池”动作并同步 Ent 模型，避免把普通账号保存误判成解绑。
- 代理跨池或移出池后，受影响账号会保留 `pool_id` 并由后续池调度重新分配；删除整个池时账号保留最后的 `proxy_id`。

## 验证记录（2026-08-04）

- `pnpm vitest run src/views/admin/__tests__/ProxyPoolsView.spec.ts src/components/admin/account/__tests__/BindProxyPoolModal.spec.ts`：2 个文件、5 个测试通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过；仅有既有 Browserslist、动态/静态 import 和 chunk size 警告。
- `go test ./internal/service -run '^TestProxyPool' -count=1 -p 1`：通过。
- `go test ./internal/repository -run '^TestProxyPoolRepository' -count=1 -p 1`：通过。
- `go test ./internal/handler/dto -run '^TestProxyPool' -count=1 -p 1`：通过。
- `go test ./internal/handler/admin -run '^$' -count=0 -p 1`：编译通过。
- `go test ./cmd/server -run '^$' -count=0 -p 1`：编译通过。
- `git diff --check`：通过。
- 全量前端测试存在 8 个与本功能无关的既有失败：`AccountUsageCell.spec.ts` 5 个、`AccountStatusIndicator.spec.ts` 2 个、`EditAccountModal.spec.ts` 1 个。
- Docker Desktop 未运行，尚未执行真实 PostgreSQL/Redis 集成联调；当前仓储测试使用 `sqlmock`，服务测试使用内存 fake。
