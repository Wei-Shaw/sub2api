# `.s2plugin` 包格式

`.s2plugin` 是 ZIP 文件，根目录必须包含 `manifest.json`，生产包还必须包含 `signature.json`。

## 标准布局

```text
manifest.json
signature.json
runtimes/<goos>-<goarch>/<binary>
ui/index.html
ui/assets/...
```

所有运行时和 UI 文件必须出现在 `manifest.files`，值为小写十六进制 SHA-256。清单和签名文件自身不写入 `files`。

包不允许绝对路径、父目录跳转、重复路径、符号链接、未声明文件或缺失文件。宿主还限制上传大小、解压后大小和文件数量。

## 清单

字段规范见 [`v1/manifest.schema.json`](../v1/manifest.schema.json)。版本字段含义：

- `version`：插件自身语义化版本。
- `requires.sub2api`：宿主硬兼容范围。
- `recommended_sub2api_version`：建议宿主版本。
- `tested_sub2api_versions`：发布者真实验证过的版本。
- `plugin_protocol`：进程握手协议。
- `transport_api`：请求和响应帧协议。
- `ui_bridge`：配置 UI 消息协议。

能力声明允许仅 OpenAI、仅 Anthropic，或两者同时声明，禁止重复和不匹配的平台：

```json
{
  "capabilities": [
    { "id": "anthropic.oauth.request_patch.v1", "platform": "anthropic", "account_type": "oauth" }
  ]
}
```

请求补丁是协议 v1 上的可选服务，不提高已有握手、Transport API 或 UI Bridge 版本。旧宿主不识别新能力时仍会拒绝安装；发布者必须在 `requires.sub2api` 中声明包含该能力的真实宿主版本范围，不能把本地开发检查等同于正式版本支持。

## 签名

`signature.json`：

```json
{
  "algorithm": "ed25519",
  "key_id": "publisher-key-id",
  "signature": "BASE64_SIGNATURE"
}
```

签名对象是 `manifest.json` 的精确原始字节。发布者私钥不得进入插件包、源码仓库或 Sub2API 运行环境。部署者只配置 Base64 Ed25519 公钥。

默认生产配置拒绝未签名包。官方 OpenAI Transport 使用宿主内置公钥验签，不需要配置；其他发布者仍需配置 `trusted_publishers`。`allow_unsigned` 只用于开发者自己构建的本地包。
