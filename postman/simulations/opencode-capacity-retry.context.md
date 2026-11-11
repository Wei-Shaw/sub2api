# opencode-capacity-retry

2026-10-26：本地替身复现 OpenCode 的 `ttft_admission_refused` 与参数错误响应结构，使用合成账号和请求数据。

## Purpose

唯一成员 OpenCode upstream 为真实网关 handler、调度、限流处理及仓储 HTTP 传输提供容量拒绝和成功响应，检查等待、同账号恢复、耗尽后切号与取消。

## Execution

```sh
postman simulation run postman/simulations/opencode-capacity-retry.sim.yaml --port 4907 --no-history --output ndjson
cd backend
TEST_OPENCODE_CAPACITY_SIMULATION_URL=http://127.0.0.1:4907/opencode go test -tags=unit,simulation ./internal/handler -run '^TestOpenCodeCapacityPostmanSimulation$' -count=1
```

测试通过 `/_requests` 验证调用账号、次数与实际等待；检查模拟器 request 事件，结束后 Ctrl-C 停止。每轮需新启模拟器以清空记录。

## Assumptions

- 2026-10-26：账号和配额快照为合成数据，HTTP 传输使用正式仓储实现；不访问生产，不验证真实上游容量恢复概率。

## Observations

<!-- begin:observations -->
2026-10-26：唯一成员 OpenCode upstream 实际承接全部 HTTP 流量，8 个场景通过：三个入站协议与流式请求等待约 2 秒后同账号恢复；首次加三次重试后才切备用账号；单账号耗尽返回 429；15 秒等待可在约 300ms 取消；参数 400 仅请求一次。配额和临时停调写入均为零。未验证真实上游恢复概率。
<!-- end:observations -->

## 2026-11-11 容量拒绝补充回归

补充 [PR #8359 的响应样本](https://github.com/Wei-Shaw/sub2api/pull/8359#issuecomment-6671915071)：`prefill_cap_refused` 提示等待 5 秒，`overloaded` 没有等待提示；另加入已观测的完整 `server_error` 消息 `The inference service is overloaded. Please try again later.`。这些样本只描述错误形态，不证明上游何时恢复。无标记整句仅在错误类型匹配、没有非空错误码时识别；显式错误码优先、真实配额耗尽与无关请求回显的处理保留。

Go 1.27.3 与 Postman CLI 1.73.0 本地回归通过 29 个 HTTP 场景：四种容量形态各覆盖 Chat Completions、流式 Chat、Responses、Messages、同账号重试耗尽后切号、单账号耗尽最终返回 429，以及等待期间取消；参数错误仍只请求一次。前两种带提示的恢复场景分别核对至少约 2 秒和 5 秒等待，无提示的两种形态核对至少约 500ms 默认等待。测试同时确认容量错误没有写入配额冷却或临时停调。

模拟器记录 73 个实际模型 POST（52 个容量 429、20 个成功 200、1 个参数 400）和 29 个采集 GET，全部由唯一 OpenCode 成员承接。`overloaded` 与共享容量策略叠加时，专用分支保留上游 429，避免重试耗尽后被映射为 503。既有其他共享错误策略不因这次 OpenCode 分类扩展而收窄。没有上传或发布模拟资源；结果仅支持这些本地合成场景，不证明真实上游恢复概率或实际部署效果。
