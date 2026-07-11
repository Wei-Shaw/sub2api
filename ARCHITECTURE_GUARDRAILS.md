# Sub2API 架构护栏

更新时间：2026-07-11

- 保持 Go 模块化单体；不新增独立微服务、外部 MQ 或新的部署依赖。
- 视频终态、账务结算、reservation 与 outbox 的事务边界不得拆散；副作用 fail-open，但必须可重试、可审计。
- 配置使用 typed config：默认值、Validate、example、环境变量/compose 透传与测试必须同步。
- 官方根 `Dockerfile` 是可重复构建主线；交付专用 Dockerfile 只能做短期离线适配，并通过差异说明和冒烟防漂移。
- Windows 本地交付默认 named volumes；bind mount 只能作为明确知晓 NTFS 权限风险的迁移选项。
- QCanvas 只依赖发布契约；字段或 reason 变化必须先更新契约测试和文档，禁止 silently drift。
- 取消响应必须保留 provenance events；不得在 lookup 失败后默认输出 production boundary。
- 不修改 QCanvas 仓库；不把 API Key 放入前端代码、日志、截图或审查包。
- 不因本轮硬化进行无关重构；优先修根因并用回归测试锁定。
- Integration 测试不得用全表 DELETE 或批量取消其他用例任务制造绿色。
