# 上游同步验证（2026-11-04）

基线 `b7a2bb45e19c5c7577364b2696de8d42706dd760`；冻结上游
`d851a38477597157e78450d30af2d8504764d085..6f92090d7f540efd88cd4b81e651120cb36e3ff5`。
[逐 PR 台账](UPSTREAM_PRS.json) 区分 6 项适配关闭和 #8476 的继承专项/有意差异；
[SOURCE_HASHES.json](SOURCE_HASHES.json) 固定修改后的生产源码及测试。

## Docker 检查

使用项目 Go 1.26.10、Node 24、pnpm 11.17.0；本机 Docker Linux arm64。

| 检查 | 结果与范围 |
| --- | --- |
| `go test -tags=unit ./... -count=1` | 全包通过；容器内可写副本供 Ent schema 测试创建临时 `.entc`，工作区生成代码未改动 |
| 官方 MCP Go SDK OAuth 互操作 | 通过，执行仓库 `test-unit` 要求的独立 tag/用例 |
| 最终源码相关协议回归 | Antigravity、Grok、Kimi、指令回显、先进及两条旧图片调度路径通过；最后的 null 文本与无改动快速路径补做相关回归 |
| `go test -tags=integration ./...` | PostgreSQL/Redis、application、repository、middleware、handler/routes 等包通过；内部网络仅阻止两个公网 DNS/TLS 包，二者在普通 Docker 网络单独补跑通过 |
| 前端 ESLint / vue-tsc | 通过 |
| 网络客户端、usage、admin-accounts | 64 文件 / 550 用例通过 |
| Makefile 必需前端集 | 31 文件 / 280 用例通过；镜像无 make，直接执行从 Makefile 提取的同一列表 |
| 正式 Dockerfile 构建 | 通过，Node 24/pnpm 11.17.0 前端及嵌入式 Go 后端；二进制报告 0.1.231 |
| 同库 BASELINE → MODIFIED → ROLLBACK → RESTORED | 四阶段通过，20 个真实 HTTP 场景；见 [结果](runtime/result.json) |
| Playwright 实际组件夹具 | 中文 1280×800、英文 390×844；无横向溢出，最后一个档位不可取消，批量默认不修改；见 [UI.json](UI.json) |

全包单元及 SDK 在最后身份转换小修前完成；最终小修后再次执行相关协议回归与性能基准。
提交的 PR 检查会在精确 Git SHA 上重新执行完整 CI。这里不把本地分段验证冒充单条完整 integration
命令的零退出码，也不把组件夹具称为整个管理端的端到端验收。

集成脚本 [run_integration.py](run_integration.py) 使用仓库已提供的 external-test-DSN 入口，在
独立 Docker 内部网络启动一次性 PostgreSQL/Redis，不挂载 Docker socket、不连接生产数据库，结束
清理自己的容器和网络。第一次离线尝试缺少已锁定的两个 Testcontainers 模块，补齐 Go module 缓存后重跑；
并行编译曾触发编译器被终止，最终采用 2 CPU / 4 GiB、GOMEMLIMIT=1400MiB、GOGC=50 顺序验证。
公网例外补跑命令为 `go test -tags=integration ./internal/integration ./internal/shared/tlsfingerprint -count=1`。

最终输出压缩保存在 [records](records/)。前端正式构建仍有基线的大 chunk 提示，没有修改分包策略。

## 平滑升级与账务

同一 PostgreSQL/Redis 跨四阶段复用。已有用户身份、密码摘要、API Key、订阅和历史用量保持完整；
迁移数恒为 **313**。每个成功请求只落一条用量，`余额 + 累计 actual_cost = 初始余额` 在每阶段成立。
Images 返回 2 张图片，各版本均结算 $0.20；Responses HTTP JSON/SSE 均记录 1000/100 token、$0.004。

新镜像把显式 4K 请求交给允许 4K 的账号；Grok 委派消息按片段顺序恢复。Kimi 并发限制在旧镜像进入
error，在新镜像保持 active 并设置短限流；仅有一个账号的夹具对外仍返回 502，验证重点是账号恢复策略。
回退前清除新档位限制，避免旧节点忽略该字段；不执行 Down SQL。所有认证和测试密钥只在夹具内存/临时
数据库中使用，没有写入结果文件。

## 性能

[PERFORMANCE.json](PERFORMANCE.json) 保留每次样本和中位数。Docker 2 CPU / 4 GiB、GOMAXPROCS=2、
GOMEMLIMIT=1400MiB、GOGC=50、CGO_ENABLED=0、GOFLAGS=-p=1；禁网，其他验证结束后顺序执行，200ms × 3。
上游参考实现仅存在于测试文件。B/op 表示累计分配量，不是峰值 RSS。

| 固定夹具 | 上游中位数 | 适配后中位数 | 累计分配变化 |
| --- | --- | --- | --- |
| 账号档位过滤 | 53.57 ns/op | 26.17 ns/op | 48 B / 1 次 → 0 B / 0 次 |
| 1 MiB 历史、32 个待修正系统片段 | 37.82 ms/op | 35.31 ms/op | 43.62 MB → 9.60 MB，约 -78% |
| 同样历史、无需身份修正 | 29.96 ms/op | 29.40 ms/op | 8.34 MB → 7.28 MB |
| 1 MiB 文本 delta 的回显处理 | 287.16 µs/op 扫描 | 直接事件分支跳过 | 均为零分配；不再扫描响应体 |

身份集中改写使用小对象解码，修正夹具的分配次数从 656 增至 998，但大块复制显著减少；无需修正时
跳过额外解码，分配次数从 236 降至 156。delta 的极小分支耗时仅代表 helper 快速路径，不作为吞吐倍数。
这些夹具不证明生产吞吐、真实上游时延或故障率。

## 既有门禁

`make check-docs` 仍因根文档索引指向缺失的 `PR169_MEDIA_STUDIO_REVIEW.md` 失败；修改文档中的本地链接
另行检查通过。源码布局门禁仅复现 `payment_fulfillment_test.go` 1201 行和 `pricing_service.go` 1218 行，
两文件与基线完全相同。没有扩大 allowlist、放宽检查或重复重构既有问题。

Git 祖先关闭使用保留当前实现树的 `ours` merge。合并后必须验证冻结上游 SHA 已是 main 的祖先，
并确认 compare 的 `behind_by=0`。该登记不会把 #8319/#8476 和 #8006/#8369 暂缓能力标记为已实现。

PR 首轮 Go lint 发现位掩码替代后残留的未使用排序 helper，已删除；没有改变运行分支，并补跑图片调度回归。
