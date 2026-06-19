# 15776812883 公网 API Key 联通性验证结果

## 背景

- 用户：`15776812883@phone.com`
- Key 掩码：`sk-58bef07...1b805f`
- 验证时间：2026-06-18 11:59 JST
- 参考最新公网结果文档：`docs/ai/context/20260618-114829-aaccx-pw-sub2api-public-route-result_CN.md`

## 验证目标

确认该用户迁移后的旧 API Key 通过公网 Sub2API 入口可用。

## 验证入口

- 主入口：`https://aaccx.pw/v1/*`
- 兼容入口：`https://api.aaccx.pw/v1/*`

## 验证结果

### 主入口 `https://aaccx.pw`

- `GET /v1/models`
  - HTTP 200
  - `model_count=10`
  - `first_model=gpt-5.5`
- `POST /v1/chat/completions`
  - HTTP 200
  - `model=gpt-5.5`
  - `choices=1`
  - `finish_reason=stop`
  - 返回内容预览：`pong`

### 兼容入口 `https://api.aaccx.pw`

- `GET /v1/models`
  - HTTP 200
  - `model_count=10`
  - `first_model=gpt-5.5`

## 结论

`15776812883@phone.com` 对应的旧 Key 公网联通性正常。当前推荐把用户侧文档和入口统一指向 `https://aaccx.pw/v1`，`https://api.aaccx.pw/v1` 可作为兼容入口保留观察。

## 安全边界

- 本次记录不写入完整 API Key。
- 命令输出只保留 Key 掩码、HTTP 状态和响应摘要。
