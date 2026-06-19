# 新生成 API Key 联通性测试结果

## 结论

- 通过 `https://aaccx.pw/keys` 页面连续创建 3 个新 API Key。
- 修复前：3 个新 Key 的 `GET /v1/models` 均返回 200，能看到模型列表；`POST /v1/chat/completions` 均返回 503。
- 对照原有 Key `ceshi` 后确认：原有 Key 也同样 `/v1/models` 200、chat 503，因此不是“新生成 Key 不能认证”的问题。
- 根因：`codex-pool-49-usd` 分组没有绑定任何上游账号，chat 调度日志为 `no available accounts`。
- 已修复运行态配置：给 `codex-pool-29-usd` 和 `codex-pool-49-usd` 补上 `cliproxy-local-openai` 上游账号绑定，优先级为 1。
- 修复后：3 个新 Key 的 `POST /v1/chat/completions` 均返回 200，内容为 `pong`，并写入 `usage_logs`。

## 测试 Key

只记录掩码，不记录完整 Key。

| 名称 | Key 掩码 | 分组 | 状态 |
| --- | --- | --- | --- |
| `new-key-test-20260619-0933-01` | `sk-dcacd...284ed0` | `codex-pool-49-usd` | active |
| `new-key-test-20260619-0933-02` | `sk-ab32e...04be68` | `codex-pool-49-usd` | active |
| `new-key-test-20260619-0933-03` | `sk-ce3bb...5aaf57` | `codex-pool-49-usd` | active |

## 修复前验证

`GET https://aaccx.pw/v1/models`：

| Key | HTTP | 模型数 | 样例模型 |
| --- | --- | --- | --- |
| 01 | 200 | 10 | `gpt-5.5`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.3-codex`, `gpt-5.3-codex-spark` |
| 02 | 200 | 10 | `gpt-5.5`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.3-codex`, `gpt-5.3-codex-spark` |
| 03 | 200 | 10 | `gpt-5.5`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.3-codex`, `gpt-5.3-codex-spark` |

`POST https://aaccx.pw/v1/chat/completions`：

| Key | 模型 | HTTP | 错误 |
| --- | --- | --- | --- |
| 01 | `gpt-5.4-mini` | 503 | `Service temporarily unavailable` |
| 02 | `gpt-5.4` | 503 | `Service temporarily unavailable` |
| 03 | `gpt-5.5` | 503 | `Service temporarily unavailable` |
| 原有 `ceshi` | `gpt-5.4` | 503 | `Service temporarily unavailable` |

服务日志对应 `openai_chat_completions.account_select_failed`，错误为 `no available accounts`，`group_id=4`。

## 运行态修复

执行的配置修复：

```sql
INSERT INTO account_groups (account_id, group_id, priority)
VALUES (1, 3, 1), (1, 4, 1)
ON CONFLICT (account_id, group_id) DO NOTHING;
```

修复后分组绑定：

| group_id | 分组 | 绑定数 |
| --- | --- | --- |
| 2 | `codex-pool-19-usd` | 1 |
| 3 | `codex-pool-29-usd` | 1 |
| 4 | `codex-pool-49-usd` | 1 |
| 5 | `codex-pool-local-unlimited` | 1 |

## 修复后验证

`POST https://aaccx.pw/v1/chat/completions`，模型 `gpt-5.5`：

| Key | HTTP | 内容 |
| --- | --- | --- |
| 01 | 200 | `pong` |
| 02 | 200 | `pong` |
| 03 | 200 | `pong` |

`usage_logs` 新增 3 条记录：

| Key | account_id | group_id | subscription_id | actual_cost |
| --- | --- | --- | --- | --- |
| 01 | 1 | 4 | 46 | `0.0016750000` |
| 02 | 1 | 4 | 46 | `0.0016750000` |
| 03 | 1 | 4 | 46 | `0.0016750000` |

## 后续注意

- `/v1/models` 能 200 只能说明 Key 认证和模型列表可见，不代表分组一定有可调度上游账号。
- 新增或重命名订阅分组后，必须同步检查 `account_groups` 绑定，否则 chat/responses 真实请求会在账号选择阶段失败。
- 不要在文档中记录完整 API Key。
