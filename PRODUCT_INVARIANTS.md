# Sub2API 产品不变量

更新时间：2026-07-11

1. **任务状态**：视频状态迁移使用 version CAS；terminal 状态不被事件、采集、归档或通知失败改写；unknown dispatch 不自动重复创建付费任务。
2. **账务一致性**：flag-on 计费路径的 reservation、不可变交易、余额与终态同事务且幂等；mock 路径不产生 reservation 或 charge。
3. **QCanvas mock 契约**：API Key create/poll 保持 `id`、`status`、`result_url`、`error_message`、`provider`；mock 成功必须为 `succeeded + result_url`，真实 Provider dispatch 计数为 0；取消响应必须保留 provenance。
4. **密钥域隔离**：`VIDEO_GATEWAY_ENCRYPTION_KEY` 与 JWT、TOTP 密钥相互独立；日志、测试、文档与审查包不得输出凭证或完整敏感 URL query。
5. **兼容与回滚**：公开状态枚举和 flag-off 旧路径保持兼容；可靠性开关默认关闭；回滚使用 `git revert` 与向后兼容迁移，不破坏账本。
6. **交付事实**：任务 succeeded、URL 存在和资产可预览/下载是三个不同层级，UI 与审查包必须如实区分。
7. **测试诚实**：未执行、skip、外部依赖缺失必须显式报告，不能用 exit 0 冒充运行通过；integration 不得清库换绿。
8. **边界**：本阶段不调用真实/付费 Provider，不触碰生产数据、真实支付或公网部署。

详细不变量以 `docs/superpowers/specs/2026-07-10-reliability-core-design.md` 为准；本文件是导航摘要，不创建平行规则。
