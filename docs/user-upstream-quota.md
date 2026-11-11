# 用户端上游额度 / User-facing upstream quota

## 配置

此功能默认关闭。管理员在「系统设置 → 功能」的「用户端上游额度」中，填入要公开的账号 ID（英文逗号分隔，最多 200 个），启用后点击此卡片中的独立保存按钮。账号 ID 可在账号管理中查看。

**选中的账号会对全部已登录用户展示，不按用户分组过滤。** 请仅选择允许公开额度信息的账号。未选择的账号不会出现在该接口中，也不会由该页面触发额度查询。重新加载页面后，侧边栏显示「上游额度」，原生页面路径为 `/upstream-quota`。

展示字段仅包含匿名账号编号、状态、额度百分比、重置时间、采样时间和当前请求并发。账号名称、邮箱、备注、凭证、原始错误和验证链接不会下发。停用或异常账号不展示额度；查询失败或没有额度数据时显示未知，不推断为零额度或满额。

GPT、反重力、Gemini 按 Tab 查询，不预加载未选择的平台，也不自动轮询。手动刷新读取进程内共享的三分钟额度缓存，不能强制刷新上游。并发查询合并，每个平台最多三个并行额度查询，单批查询限时 30 秒。多实例各自持有缓存；采样时间沿用官方额度服务的数据，不将页面请求时间当作上游采样时间。当前并发单独读取，不缓存三分钟。

反重力沿用官方模型额度快照，不额外调用上游，不把模型快照复制为“7 天”窗口。同类模型中额度与重置时间完全相同的数据合并显示；不同值分别编号。完整的窗口分组应在官方数据源支持后再接入。

关闭功能或移除账号后，后续请求立即按最新配置拒绝或过滤；慢查询返回前再次检查分享配置。已经送达浏览器的数据无法撤回。修改分享配置需要完整管理员权限，受限管理 API Key 不可访问该配置。

## Configuration

Sharing is disabled by default. In **System settings → Features → User-facing upstream quota**, enter the account IDs to share (comma separated, at most 200), enable sharing, and use the card's own save button. Reload the page to update navigation. The native page is `/upstream-quota`.

**Selected accounts are visible to every signed-in user, regardless of group membership.** Only opt in accounts whose quota information may be shared. Unselected accounts are neither returned nor probed by this feature. The settings endpoints require full administrator access; restricted administrator API keys are denied.

The response uses explicit DTOs containing anonymous account aliases, status, quota percentages, reset/sample times, and concurrency. It never returns account names, email addresses, credentials, notes, raw provider errors or validation URLs. Disabled/error accounts have no quota bars. Missing or failed quota data stays unknown.

Platform tabs load on demand; there is no background polling. All viewers share a three-minute, per-process quota cache with singleflight, at most three probes per platform, and a 30-second collection deadline. Refresh cannot force an upstream call. Each replica has its own cache. Concurrency is fetched separately; quota timestamps come from the existing official usage service.

Antigravity uses existing model snapshots without inventing 5-hour/weekly windows or implementing another provider fetcher. Identical quota/reset pairs are grouped within each model family; distinct values remain separate. True window groups can be integrated once supported by the official data source.

Sharing settings are read on every request and rechecked after slow quota collection. Disabling sharing or removing an account prevents subsequent responses from serving the old selection, including cached responses. Previously delivered browser data cannot be recalled.

This first contribution intentionally excludes the standalone board's sufficiency score and recent-user metrics, keeping the permission and data boundary reviewable. The standalone MIT project that motivated it is `galois21/sub2api-quota-board`.
