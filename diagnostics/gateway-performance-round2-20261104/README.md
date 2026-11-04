# 第二轮网关性能调查

本文记录第二轮热点调查与已实施的生产优化。调查结论见 [REPORT.md](REPORT.md)，实现和最终验证见 [implementation/README.md](implementation/README.md)。

基线是合并 PR #177 后的 `origin/main`：`68c96552b040b943f3a04484c798efd30110f2ed`。执行探针的工作区 HEAD 为 `772960faf18a3032466cb25914f84e568d53aa2f`，两者 Git tree 都为 `f03e7fcfd860ee1d925a3c16c243ecc7f147e4f6`，源码完全相同。

环境：Docker Linux arm64、`golang:1.26.10-alpine`、`CGO_ENABLED=0`、2 CPU、4 GiB RAM、`GOMAXPROCS=2`、`GOMEMLIMIT=1400MiB`、`GOFLAGS=-p=1`。使用上轮预热的 Go module/build volume；源码只读挂载、容器禁网。基准顺序执行，定时测量不与其他验证并行。每项取 3 次样本的中位数，B/op 为累计分配量，不是峰值 RSS。

`probes/*.go` 通过 [overlay.json](overlay.json) 作为额外测试文件注入 Go 包，生产文件无替换。`view_probe` 和 `reused_validity_probe` 只用于合法 fixture 的方案验证，不是已实施的修复，也不代表完整的畸形数据、长度边界、连接恢复或计费兼容性验证。探针保留持久 ID 的独立字符串，避免省去复制却导致小 ID 长期引用整个大响应。

| 文件 | 内容 |
| --- | --- |
| [probes.txt](probes.txt) | 30 个 WS、图片计数、tokenizer、工具流场景，200 ms × 3 |
| [controls.txt](controls.txt) | 22 个现有解析、命名空间、会话种子、自定义输入和快照拷贝对照，150 ms × 3 |
| [medians.json](medians.json) | 两组原始样本的中位数 |
| [verification.txt](verification.txt) | 相关 envelope、用量、图片及工具流测试结果 |
| `*-cpu-top.txt` / `*-alloc-top.txt` | 2 秒 CPU/累计分配 profile 的 top 输出 |
| `*-profile-benchmark.txt` | 开启 profile 时的基准记录；报告性能表使用未开启 profile 的样本 |
| [implementation/](implementation/README.md) | 第二轮实际实施、优化后基准、回归和 fuzz 结果 |

从仓库根目录复跑：

```sh
sh diagnostics/gateway-performance-round2-20261104/run-probes.sh probes rerun
sh diagnostics/gateway-performance-round2-20261104/run-probes.sh controls rerun
sh diagnostics/gateway-performance-round2-20261104/run-probes.sh verify rerun
sh diagnostics/gateway-performance-round2-20261104/run-probes.sh admission rerun
```

脚本拒绝覆盖已有结果。需要本机已缓存镜像和依赖；因为 `GOPROXY=off` 且容器禁网，空缓存会直接失败。

采集 profile 使用相同 Docker 参数，在容器内依次执行下列模板。`PACKAGE`、`FILTER` 分别取下表值；测试二进制与 profile 留在容器 `/tmp`，仅导出文本 top 和基准输出到本目录（容器另挂载为 `/out`）。

```sh
go test -overlay=/src/diagnostics/gateway-performance-round2-20261104/overlay.json \
  PACKAGE -run '^$' -bench FILTER -benchtime=2s -count=1 \
  -o /tmp/probe.test -cpuprofile=/tmp/probe.cpu -memprofile=/tmp/probe.mem
go tool pprof -top -nodecount=15 /tmp/probe.test /tmp/probe.cpu
go tool pprof -top -sample_index=alloc_space -nodecount=12 /tmp/probe.test /tmp/probe.mem
```

| 场景 | PACKAGE | FILTER |
| --- | --- | --- |
| 工具参数 | `./internal/shared/apicompat` | `^BenchmarkRound2ChatToolArguments$/256KiB$` |
| 图片计数 | `./internal/application/service` | `^BenchmarkRound2ImageAccounting$/text_delta$/1024KiB$` |
| tokenizer | `./internal/application/service` | `^BenchmarkRound2Tokenizer$/fallback_full$/1024KiB$` |

所有输入均为合成数据。工具参数按 1 KiB 分片；大参数用于检验复杂度，不代表线上参数分布。图片计数探针使用无图片的 text delta / terminal，因此结果为 0 是正确性要求。WS envelope fixture 的大文本在 `response.output`；字段排列改变时扫描耗时也会变化。Tokenizer 使用重复英文文本，词表已预热；真实中文、代码与特殊符号需要独立验收。

日志仅规范化行首缩进及行尾空白，数值不变。Go 格式、shell 语法和新增文档链接检查通过；仓库 `make check-docs` 仍因原有的 PR169 文档缺失链接失败。
