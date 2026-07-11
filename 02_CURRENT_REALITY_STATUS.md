# Sub2API 当前现实状态

更新时间：2026-07-11
状态：**内部 mock 可演示 / 脏树本地收口条件通过 / 非生产 READY**

## 已有证据

- 2026-07-11 本机构建机完成 Docker mock 彩排：健康检查、admin 登录、首次合规确认、创建绑定分组的 API Key、mock create/poll、`succeeded + result_url`。
- mock 状态机观测为 `queued → submitted → running → succeeded`。
- 交付包存在于 `sub2api-delivery/`；密钥、备份、镜像与安装包不进入 Git，也不作为审查内容读取。
- 2026-07-11 Grok cleanup：脏工作树已分批本地提交并收口；证据见 `docs/superpowers/codex-handoff/deliverables/2026-07-11-GROK-CLEANUP-MERGE-review.md`。

## 本轮已收口

- Cancel provenance：取消响应保留 trial/production events；lookup 失败不再默认 production。
- Integration isolation：删除全表 DELETE 与批量取消其他任务；按用例 ID cleanup/assert。
- 视频契约：UTF-8 完整契约恢复 admin/QCanvas 端点、字段、计费与三种 boundary。
- 配置 defaults / Validate / example / compose 透传、专用视频加密密钥、worker 关闭可观察。
- 前端视频 console 缺失模块（productMode / display currency / lifecycle 测试）已提交。
- 本地 Git clean；`sub2api-delivery` / `.worktrees` / `.delivery-tools` 仅 ignore，未删除。

## 尚未证明 / 外部门禁

- 真实 Seedance / 其他付费 Provider。
- 真实支付、生产数据、生产部署、公网暴露。
- Windows Testcontainers：`rootless Docker is not supported on Windows` → INTEGRATION_BLOCKED。
- 新 Docker 镜像与容器 `/health`：镜像代理 HTTP 429 外部门禁，本轮未重试。
- 浏览器 UI 截图：本轮未启动合成 mock 环境。

## 事实边界

mock 成功不等于生产可用；`result_url` 存在不等于资产已持久交付；integration 命令 exit 0 只有在测试确实执行时才可记为通过。
