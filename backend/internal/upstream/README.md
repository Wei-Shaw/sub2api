# package upstream — 上游适配层

把各上游平台在「错误分类」上的差异收敛到一个接口里。本包只依赖标准库。

在本 PR 中，它的唯一使用方是账号追溯（`service/ops_attempt_trace.go`）：
每一次失败的上游尝试落库到 `upstream_attempts` 时，用对应平台的适配器给它打上
归一化的 `failure_kind`，保证「库里记录的失败类型」和平台真实语义一致。

## 接口

```go
type Adapter interface {
    Platform() Platform
    DefaultBaseURL() string
    AuthHeaders(Credentials) (http.Header, error)
    BaseURL(Credentials) string
    Classify(now time.Time, status int, header http.Header, body []byte) Failure
    StickyKey(RequestMeta) string
}
```

`Failure.Kind` 取值：`none` / `auth`（凭据或账号失效）/ `rate_limit`（冷却到 RetryAt）/
`overload`（上游过载，换号重试）/ `transient`（瞬时故障，重试）/ `client`（请求本身的问题）。
`Classify` 显式接收 `now`，冷却窗口因此可测、可重放。

目前提供 Claude、OpenAI、Antigravity 三个实现，`Registry` 按平台查找。
鉴权头、base URL、会话粘性键这几个方法已经实现并有测试，但本 PR 不改动现有网关的这些路径。

## 已经吸收掉的平台差异

- Claude：余额不足以 **400** 返回（不是 402/429）；529 与 `overloaded_error` 都属于过载。
- OpenAI：`insufficient_quota` 挂在 **429** 上，但语义是账号级（应判为 `auth`，不能当成限流冷却）。
- Antigravity：429 里的 `MODEL_CAPACITY_EXHAUSTED` 是「当前模型没容量」（换号重试），不是「账号没额度」。

## 验证

```bash
cd backend
go test ./internal/upstream/ -count=1
go vet  ./internal/upstream/
```
