# API 联通性与模型访问验收结果

## 背景

用户要求测试当前 API 是否联通，以及是否能访问到模型。

## 验收对象

- 本地入口：`http://127.0.0.1:18080`
- 公网入口：`https://aaccx.pw`
- 公网入口：`https://api.aaccx.pw`
- 测试 Key：`sk-LOCAL-4...e28804`
- Key 所属用户：`15951875192@phone.com`
- 订阅分组：`codex-pool-local-unlimited`

## 验收计划

1. 验证 Sub2API 容器和本地 `/health`。
2. 使用本机自用 Key 请求三个入口的 `/v1/models`。
3. 从模型列表选择 `gpt-5.4` 做最小 `/v1/chat/completions` 请求。
4. 仅记录脱敏 Key 和结果，不记录完整 API Key。

## 验收结果

### 本地入口

- `GET http://127.0.0.1:18080/health`：HTTP 200。
- `GET http://127.0.0.1:18080/v1/models`：HTTP 200。
- 模型数量：10。
- 样例模型：`gpt-5.5`、`gpt-5.4`、`gpt-5.4-mini`、`gpt-5.3-codex`、`gpt-5.3-codex-spark`、`codex-auto-review`、`gpt-5.2`、`gpt-image-1`。
- `POST http://127.0.0.1:18080/v1/chat/completions`，模型 `gpt-5.4`：HTTP 200，返回内容 `pong`。

### `aaccx.pw` 公网入口

- `GET https://aaccx.pw/v1/models`：HTTP 200。
- 模型数量：10。
- 样例模型：`gpt-5.5`、`gpt-5.4`、`gpt-5.4-mini`、`gpt-5.3-codex`、`gpt-5.3-codex-spark`、`codex-auto-review`、`gpt-5.2`、`gpt-image-1`。
- `POST https://aaccx.pw/v1/chat/completions`，模型 `gpt-5.4`：HTTP 200，返回内容 `pong`。

### `api.aaccx.pw` 公网入口

- `GET https://api.aaccx.pw/v1/models`：HTTP 200。
- 模型数量：10。
- 样例模型：`gpt-5.5`、`gpt-5.4`、`gpt-5.4-mini`、`gpt-5.3-codex`、`gpt-5.3-codex-spark`、`codex-auto-review`、`gpt-5.2`、`gpt-image-1`。
- `POST https://api.aaccx.pw/v1/chat/completions`，模型 `gpt-5.4`：HTTP 200，返回内容 `pong`。

## 结论

当前本地入口和两个公网入口都能访问模型列表，并能通过 `gpt-5.4` 完成最小 chat 请求。

## 注意

- 本次只验证联通性和模型可访问性，没有做并发、长流式、扣费边界或所有用户 Key 的批量回归。
- 完整 API Key 没有写入文档。
