# Sub2API 本地插件协议

本目录是插件开发者可以依赖的公开契约。`v1/plugin.proto` 和 `v1/runtime.go` 定义进程协议，`v1/manifest.schema.json` 定义包清单，`docs/` 记录开发和发布规范。Provider 私有实现不应放入本目录。

## 开发文档

- [开发指南](docs/development.md)：从运行时、配置到集成测试的完整流程。
- [UI Bridge](docs/ui-bridge.md)：沙箱配置 UI 的消息结构和安全要求。
- [包格式](docs/package-format.md)：清单、文件哈希、签名和版本规则。
- [安全边界](docs/security.md)：进程权限、敏感数据和故障策略。

## 实体与运行方式

插件的交付实体是一个 `.s2plugin` 文件，本质上是带清单、签名、独立可执行文件和静态 UI 的 ZIP 包。管理员在独立的插件管理页手动上传，Sub2API 不从网络自动下载插件，也不要求 Docker。

启用后，Sub2API 以子进程方式拉起当前操作系统和 CPU 架构对应的二进制，通过本机 gRPC 流传递请求与响应。插件进程退出时会随 Sub2API 清理；停用时先停止接收新请求，再等待正在处理的请求结束。

多实例部署不要求共享插件目录。宿主会在数据库保存已验签的原始插件包，各实例缺少本地文件时会重新验签和解包，并周期性对齐启用状态、灰度比例和加密配置。所有实例必须连接同一数据库并使用相同的加密密钥。

独立进程是代码和发布边界，不是操作系统安全沙箱。插件拥有 Sub2API 服务用户所拥有的文件和网络权限，因此只应安装可信发布者的签名包。闭源二进制可提高源码分发门槛，但不能承诺无法反编译。

## 初期能力边界

当前只接受 `openai.oauth.outbound_transport.v1`：

- 仅匹配 `platform=openai` 且 `account_type=oauth` 的上游 HTTP 请求。
- API Key 账号、其他 provider、OAuth 登录与 Token 刷新流程不进入插件。
- 插件建立真实的上游 HTTP/TLS 连接并返回原始 HTTP 响应。
- 命中插件的 OAuth WebSocket 账号会使用 Sub2API 现有 HTTP Bridge，不直接建立上游 WebSocket，避免绕过 v1 HTTP 插件协议。
- Sub2API 继续负责响应状态处理、SSE 解析、错误映射、用量统计、计费和下游输出。
- 灰度比例以账号 ID 稳定分桶，未命中的 OAuth 账号继续使用原有内置路径。

## 包结构

```text
manifest.json
signature.json                 # 生产包必需
runtimes/linux-amd64/plugin
runtimes/linux-arm64/plugin
runtimes/windows-amd64/plugin.exe
ui/index.html
ui/assets/...
```

`manifest.json` 必须声明所有运行时和 UI 文件的 SHA-256。`signature.json` 使用受信任发布者的 Ed25519 私钥对 `manifest.json` 原始字节签名。官方 OpenAI Transport 公钥由宿主内置，第三方发布者公钥由部署者追加到 `plugins.trusted_publishers`。文件哈希由已签名清单保护。

插件默认保持停用。未签名包默认拒绝安装；`plugins.allow_unsigned` 只应用于开发者自己构建的本地调试包。

## 兼容性

清单必须同时声明：

- `requires.sub2api`：允许的 Sub2API 语义化版本范围。
- `requires.recommended_sub2api_version`：建议使用的宿主版本。
- `requires.tested_sub2api_versions`：发布者实际验证过的宿主版本。
- `plugin_protocol`、`transport_api`、`ui_bridge`：三个独立协议版本。

宿主版本超出范围时，插件可以安装并查看，但保持“不兼容”状态且不能启用。版本在范围内但未列入已测试版本时，管理员必须再次确认才能启用。

## UI 隔离与 Bridge

插件 UI 由包内静态文件实现，宿主使用只有 `allow-scripts` 权限的 sandbox iframe 加载。iframe 没有管理员 Token，也不能直接访问管理 API。宿主为每次打开配置页生成短时资源 URL 和独立 Bridge Token，并且同时校验消息来源窗口与 Token。

UI 可以发送以下消息：

- `config.load`
- `config.save`
- `config.test`
- `ui.resize`
- `ui.notify`

每个请求消息带 `request_id`，宿主以 `<type>.result` 返回结果。配置整体使用 Sub2API 的密钥加密后存入数据库；运行中插件会先验证并应用新配置，数据库写入失败时恢复旧配置。

## 协议源码

- `v1/plugin.proto`：稳定的进程间消息定义。
- `v1/runtime.go`：Go 插件进程启动入口和宿主客户端声明。
- `v1/manifest.schema.json`：`manifest.json` 的 JSON Schema。

插件通过进程协议协作，不使用 Go 动态链接，也不要求插件与 Sub2API 使用相同编译器或共享内存 ABI。
