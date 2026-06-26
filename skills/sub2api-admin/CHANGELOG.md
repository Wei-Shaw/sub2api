# Sub2API Admin Skill Changelog

## 2026-06-13

- 同步上游 `Wei-Shaw/sub2api` 当前 `main` 的管理端路由范围，补充账号、分组、代理、兑换码、错误透传规则、TLS 指纹模板和 raw admin API 的说明。
- 新增用户侧辅助命令：`auth login`、`user-groups available`、`user-groups rates`、`user-keys list|get|create|update|delete|toggle`。
- 扩展账号命令：`refresh-tier`、`batch-refresh-tier`、`sync-models-preview`、`set-privacy`。
- 扩展分组命令：分页列表、用量/容量摘要、模型候选、统计、API Key 列表、用户倍率和 RPM 覆盖管理。
- 扩展代理命令：分页列表、测试、质量检查、统计、关联账号、导入导出、批量创建和批量删除。
- 在参考文档中标出更多已确认但未专门封装的 admin route family，建议用 `api <METHOD> <path>` 直通调用。
