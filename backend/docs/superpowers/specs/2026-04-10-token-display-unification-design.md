# Token 展示口径统一设计

## 背景

当前 token 展示链存在明显口径不一致：

- 表格主格子里的 `input_tokens` 直接显示为“输入 token”，但它实际是净输入（不含缓存读/写）
- tooltip 中的 `totalTokens` 又按 `input + output + cache_read + cache_creation` 计算
- 顶部 summary、表格、tooltip、导出之间的“输入 token / 总 token”含义并不一致

这会让用户误以为不同位置展示的是同一口径，实际上却混用了“净输入”和“总输入（含缓存）”。

## 目标

统一前端展示口径，使用户和管理员在所有主要 UI 中看到一致的 token 语义：

- `总输入 token = input_tokens + cache_read_tokens + cache_creation_tokens`
- `净输入 token = input_tokens`
- `总 token = 总输入 token + output_tokens`

其中：

- `总输入 token` 作为默认主展示值
- `净输入 token` 在临近位置作为次级信息显示
- `cache_read_tokens` / `cache_creation_tokens` 继续单独保留

## 非目标

- 不修改后端 DTO 字段名
- 不修改 SQL/聚合逻辑
- 不修改计费逻辑
- 不改变 `usage_logs` 当前写库语义

## 展示设计

### 1. 管理端 `UsageTable.vue`

- token 主格子中的输入部分，主值改为 `总输入 token`
- 在主值下方增加次级小字：`净输入：<input_tokens>`
- `cache_read` / `cache_creation` 继续保留在同格子的第二行
- 管理端 token tooltip 也同步拆成：
  - 总输入
  - 净输入
  - 输出
  - cache read
  - cache creation
  - 总 token

### 2. 用户端 `UsageView.vue`

- token 主格子同样改为：主值显示 `总输入 token`
- 在主值附近或次级位置显示 `净输入 token`
- token tooltip 明确拆成：
  - 总输入
  - 净输入
  - 输出
  - cache read
  - cache creation
  - 总 token
- 用户侧导出（如当前页面已有导出逻辑）也同步改成同一口径，避免与页面主展示矛盾

### 3. 顶部 summary 卡片

- 管理端 summary 实际落点是 `frontend/src/components/admin/usage/UsageStatsCards.vue`
- 用户端 summary 实际落点是 `frontend/src/views/user/UsageView.vue` 顶部卡片区域
- 由于 summary API 当前只提供聚合字段：
  - `total_input_tokens`
  - `total_output_tokens`
  - `total_cache_tokens`
  - `total_tokens`
  所以本次前端统一按下面的聚合口径展示：
  - `displayTotalInputTokens = total_input_tokens + total_cache_tokens`
  - `displayTotalTokens = total_tokens`
- `净输入 token` 作为 summary 的次级信息显示，不改动后端 API 字段命名

### 4. 导出

- 管理端导出列中，同时导出：
  - 总输入
  - 净输入
  - 输出
  - cache read
  - cache creation
  - 总 token
- 用户端导出如果已有，也按同样口径同步调整；如果当前页面没有导出，则不新增导出功能

### 5. 特殊展示保持不变

- 图片请求（`billing_mode === image`）继续保留当前单独展示分支，不强行套用 token 口径
- cache TTL 细分明细继续保留：
  - `cache_creation_5m_tokens`
  - `cache_creation_1h_tokens`
  - `cache_ttl_overridden`
- 本次只统一 token 请求路径的“输入 / 总 token”语义，不删除现有 cache 细分明细

## 实现边界

本次只改前端展示和导出层，目标文件限定为：

- `frontend/src/utils/usageTokens.ts`（若需要共享前端口径计算）
- `frontend/src/components/admin/usage/UsageTable.vue`
- `frontend/src/components/admin/usage/UsageStatsCards.vue`
- `frontend/src/views/user/UsageView.vue`
- `frontend/src/views/admin/UsageView.vue`
- 如需补文案：
  - `frontend/src/i18n/locales/en.ts`
  - `frontend/src/i18n/locales/zh.ts`

所有计算都在前端本地完成：

- `displayInputTokens = input_tokens + cache_read_tokens + cache_creation_tokens`
- `displayTotalTokens = displayInputTokens + output_tokens`
- `netInputTokens = input_tokens`

## 验证

- `pnpm typecheck`
- 如现有前端测试覆盖相关展示，补/更新它们，优先关注：
  - `frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`
  - `frontend/src/views/user/__tests__/UsageView.spec.ts`
  - `frontend/src/views/admin/__tests__/UsageView.spec.ts`
  - 特别补齐管理端 token tooltip 口径断言，避免只改主格子不改 tooltip
- 不需要重新跑后端测试，因为本次不改后端逻辑

## 预期结果

- 用户端和管理端 token 主值口径一致
- tooltip、summary、导出不再互相矛盾
- cache token 仍然保持单独可见
