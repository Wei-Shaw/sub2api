# 上游同步证据

范围、兼容边界与决策见 [审查文档](../../docs/UPSTREAM_SYNC_20261104.md)；验收见 [VERIFICATION.md](VERIFICATION.md)。

- [UPSTREAM_PRS.json](UPSTREAM_PRS.json)：冻结 PR/head/merge、diff 摘要、owner、关闭和重开条件。
- [SOURCE_HASHES.json](SOURCE_HASHES.json)：修改源码与测试的 SHA-256。
- [PERFORMANCE.json](PERFORMANCE.json)：Docker 上游/适配实现的逐次基准与中位数。
- [runtime](runtime/)：临时数据库与假上游的真实 HTTP 升级回退夹具、四阶段结果。
- [UI.json](UI.json)：实际 feature 组件的语言、布局与交互证据。
- [records](records/)：验证输出压缩记录；环境限制与补跑情况在验证文档中说明。

从仓库根目录执行 `python3 diagnostics/upstream-sync-20261104/run_integration.py` 可在已缓存依赖的 Docker
环境复跑数据库集成；公网 DNS/TLS 包另用普通 Docker 网络补跑。`runtime/check.py` 需要本轮基线和候选
镜像，创建并清理 `sub2api-sync-20261104` Compose 项目，仅使用 127.0.0.1:18444。

`ui/` 是挂载到前端 Docker builder 的 `/app/frontend/ui-fixture` 后启动的 Vite 夹具，`frontend/src`
同时挂载到 `/app/frontend/src`。它不连接应用或生产服务器。截图位于仓库 `output/playwright/`。
