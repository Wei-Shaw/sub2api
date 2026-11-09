## Planned Evidence

- Go 1.27.3 容器中的 RED 输出：测试成功编译到新增 API 引用处，并因实现缺失失败。
- GREEN 输出：成功 turn 收到 delta/completed；上游 RPC error 原样映射；取消触发 fake 进程观察到 `turn/interrupt`。
- 新增包 coverage 与 race 结果。
- OpenSpec strict 校验与 diff whitespace 校验。

## RED Evidence

- `docker exec -w /src/backend/backend sub2api-go-test-40 go test ./internal/codexapp -count=1`
- 结果：退出码 1；测试包成功进入编译，因预期缺少 `Event`、`TurnRequest`、`InputItem`、`RPCError`、`Provider` 等生产 API 而失败。
- 边界：此前一次 bind mount 在当前 Docker daemon 上只得到空目录，`go.mod not found` 属于环境失败，不计入 RED；改用 `docker cp` 后才取得上述有效 RED。

## Acceptance Boundary

本变更只验证 sub2api 内部 provider 与 fake app-server 子进程之间的 hermetic 链路。它不访问真实 Codex binary、ChatGPT/OpenAI 账号或模型服务，不证明 OAuth、模型权限、配额、真实 streaming 或服务端协议兼容。

