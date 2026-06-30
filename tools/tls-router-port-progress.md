# 进度日志:TLS 指纹路由器 + 采集器移植

> 配套方案:`tools/tls-fingerprint-router-port.md`。本文件是 /goal 无人值守执行的**唯一进度账本**。
> 规则:每个 Phase **完成或受阻**都必须在此追加一条;只勾【静态】验证,【运行时·人工】项一律留给人工。
>
> 分支:`feat/tls-fingerprint-router`(从 `feat/req-resp-archive` 切出)。基线 commit:`330c35c1`。

## 阶段状态总览

| Phase | 名称 | 状态 | commit | 静态验证(build/vet/test) | 备注 |
|---|---|---|---|---|---|
| A | tlsfingerprint 包补 3 文件 | ⬜ 未开始 | — | — | |
| B | ent schema + model + 生成 | ⬜ 未开始 | — | — | |
| C | 迁移 158_add_tls_fingerprint_routers.sql | ⬜ 未开始 | — | — | |
| D | repository(router repo + cache) | ⬜ 未开始 | — | — | |
| E | service(router + collector)+ config | ⬜ 未开始 | — | — | |
| F | handler + 路由 + wire | ⬜ 未开始 | — | — | |
| G | OpenAI HTTP 集成 | ⬜ 未开始 | — | — | 硬骨头;运行时验证留人工 |
| H | OpenAI WS 集成 | ⬜ 未开始 | — | — | 硬骨头;连接池 key 须含指纹 |
| I | cmd/server 优雅关闭 | ⬜ 未开始 | — | — | |
| J | 代码生成 + 编译 | ⬜ 未开始 | — | — | |
| K | 前端 | ⬜ 未开始 | — | — | 工具链不可用则记待人工 |

状态图例:⬜ 未开始 / 🟡 进行中 / ✅ 完成(静态绿+commit)/ ⛔ 受阻(见下方记录)

## 执行日志(按时间追加)

<!-- 每条格式:
### Phase X — <名称> — <完成/受阻>
- 改动文件:...
- 命令与结果:go generate/build/vet/test → 通过 or 失败摘要
- commit:<hash>
- 遗留/风险:...
-->

(待 /goal 填写)

## 待人工验证(运行时)

<!-- 无人值守做完后,把所有【运行时·人工】项列在此,供回来逐项验证 -->

- 待补:Phase C 迁移幂等(测试库)、G HTTP JA3、H WS 不串号、采集器抓取、OAuth 换 token 等(见方案 §5)。

## Blockers / 需人工决策

<!-- 任何"不猜不编"触发的停机点记在此:是什么、卡在哪、需要什么决策 -->

(暂无)
