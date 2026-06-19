# tongji_lishouqi 用户 59 元套餐添加结果

## 结论

- 已将 `tongji_lishouqi@163.com` 添加到 Sub2API。
- 用户状态：`active`。
- 用户角色：`user`。
- 已按用户要求重置登录密码；文档不记录明文密码。
- 已绑定 59 元套餐对应分组 `codex-pool-49-usd`，`group_id=4`。
- 已创建默认 API Key `tongji-lishouqi-59-default`，只记录掩码：`sk-fcebb...f42bb0`。
- 公网 `/v1/models` 和 `/v1/chat/completions` 均验证通过。

## 数据库结果

| 项 | 值 |
| --- | --- |
| user_id | `27` |
| email | `tongji_lishouqi@163.com` |
| subscription_id | `47` |
| group_id | `4` |
| group_name | `codex-pool-49-usd` |
| api_key_id | `34` |
| key_name | `tongji-lishouqi-59-default` |
| key_masked | `sk-fcebb...f42bb0` |

订阅有效期：

- `expires_at=2026-07-19 10:42:01 +08`

## 验证结果

登录验证：

- `POST http://127.0.0.1:18080/api/v1/auth/login`
- HTTP `200`
- 返回 access token 和 refresh token
- 用户状态 `active`

公网 API 验证：

| 接口 | HTTP | 结果 |
| --- | --- | --- |
| `GET https://aaccx.pw/v1/models` | `200` | 返回 10 个模型 |
| `POST https://aaccx.pw/v1/chat/completions` | `200` | 返回 `pong` |

样例模型：

- `gpt-5.5`
- `gpt-5.4`
- `gpt-5.4-mini`
- `gpt-5.3-codex`
- `gpt-5.3-codex-spark`

用量记录：

| model | account_id | group_id | subscription_id | input_tokens | output_tokens | actual_cost |
| --- | --- | --- | --- | --- | --- | --- |
| `gpt-5.5` | `1` | `4` | `47` | `305` | `5` | `0.0016750000` |

## 注意

- 不要在文档、提交或回复中记录完整 API Key。
- 该账号之后可登录控制台自行管理或新建 API Key。
