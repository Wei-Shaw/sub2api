# Sub2API 插件开发教程

本文面向希望为 Sub2API 开发、打包和发布插件的团队。插件是独立进程和静态 UI 组成的 `.s2plugin` 包，宿主通过稳定的 gRPC 协议调用它。本文以当前宿主已经定义的 `openai.oauth.outbound_transport.v1` 能力作为协议示例，说明开发者需要准备什么、哪些职责属于插件、哪些职责仍由 Sub2API 负责。

本文不是一个可直接安装的完整插件，也不代表 Sub2API 已经发布对应的官方插件包。当前文档主要描述公开协议、宿主边界和开发流程。后续是否发布可安装包、支持哪些 Provider，以及如何提供示例仓库，都需要另行公告。

## 1. 准备开发环境

建议使用以下环境：

- Go 1.21 或更高版本；
- Node.js（仅在插件 UI 使用 JavaScript 时需要）；
- Git；
- 与目标部署环境一致的构建工具链。

协议定义和通用说明位于：

- `backend/pkg/pluginapi/v1/plugin.proto`：进程间消息和流式请求定义；
- `backend/pkg/pluginapi/v1/runtime.go`：插件进程启动入口；
- `backend/pkg/pluginapi/v1/manifest.schema.json`：包清单 JSON Schema；
- `backend/pkg/pluginapi/docs/`：开发、UI Bridge、包格式和安全边界说明。

目前暂未提供可直接复制的官方示例源码。开发者可以按照本文的目录和协议说明创建自己的插件工程；示例仓库发布后，会在本文补充正式的获取地址、目录说明和版本要求。公开协议始终以 `backend/pkg/pluginapi/` 为准。

## 2. 创建插件工程

在示例仓库发布前，可以先创建一个独立的 Go 工程，目录建议如下：

```text
my-plugin/
├── cmd/<plugin>/main.go
├── internal/pluginconfig/
├── internal/transport/
├── ui/index.html
├── ui/assets/
├── tools/
├── manifest.source.json
└── build.sh
```

开发时至少准备以下部分：

1. `manifest.source.json`：插件 ID、名称、版本、作者、能力和兼容的 Sub2API 版本；
2. `cmd/<plugin>/main.go`：启动入口和运行时版本注入，并同步打包器中的构建目标和二进制名称；
3. `internal/pluginconfig/`：配置结构、默认值、严格校验和规范化；
4. `internal/transport/`：HTTP 客户端、代理、请求头、请求体、网络连接参数、响应流和资源回收；
5. `ui/index.html` 与 `ui/assets/`：插件自己的配置界面；
6. 单元测试、进程集成测试和目标平台构建配置。

入口文件应保持很小，只负责调用 `pluginv1.Serve`。实际逻辑放在可独立测试的包中，避免把配置解析、网络请求和协议组装全部写在 `main.go`。

## 3. 编写运行时

运行时实现 `TransportPlugin` 服务，必须满足以下约定：

| 方法 | 要求 |
| --- | --- |
| `GetInfo` | 返回的插件 ID、版本、协议版本、传输 API 版本和能力必须与清单一致。 |
| `Health` | 快速返回进程是否可以接收新请求，不执行长时间网络探测。 |
| `ValidateConfig` | 严格解析 JSON，拒绝未知字段和非法范围，并返回完整的规范化配置。 |
| `ApplyConfig` | 成功后原子切换配置；失败时保留旧配置和旧连接。 |
| `TestConfig` | 针对已保存配置进行快速诊断，返回简短、可展示的结果。 |
| `Forward` | 按协议接收请求流，发出上游请求，再按顺序返回响应流。 |

请求帧顺序为 `start`、零到多个 `body_chunk`、`body_end`；响应帧顺序为 `start`、零到多个 `body_chunk`、`end`。不能继续处理时发送 `error` 帧。

`ForwardResponseError.request_sent` 必须准确：只有在能够确认尚未调用上游 HTTP Transport 时才返回 `false`；一旦已经调用，或无法确认上游是否收到请求，就返回 `true`。宿主会据此决定是否允许切换账号重试，避免重复执行同一个请求。

资源管理也属于运行时契约：复用 HTTP Transport 和连接池，配置切换时关闭旧空闲连接，沿用 gRPC stream 的 context 取消 DNS、连接、上传和响应读取，并始终关闭上游响应体。日志和错误消息不能包含 Token、代理凭据、完整请求体或敏感响应头。

## 4. 设计插件配置

插件配置由插件定义，由 Sub2API 加密保存。推荐流程是：

1. 在 `internal/pluginconfig.Config` 中定义字段和默认值；
2. 使用 `json.Decoder.DisallowUnknownFields` 等严格方式解析；
3. 将空对象规范化为完整默认配置；
4. 在 `ValidateConfig` 和 `ApplyConfig` 中复用同一套校验；
5. 配置应用成功后再让宿主保存，保存失败时允许恢复旧配置。

JSON 字段统一使用 `snake_case`。敏感配置不要放入 URL、UI 通知、诊断结果或日志。插件不应从 UI 读取、刷新或持久化 OAuth Token；宿主只在运行时调用需要的网络转发接口。

## 5. 实现插件自己的配置 UI

UI 是插件包内的静态页面，不需要修改 Sub2API 前端源码。宿主会在受限 iframe 中加载 `ui/index.html`，并通过 UI Bridge 提供配置读写和测试能力。

页面初始化流程：

1. 加载包内 HTML、CSS 和 JavaScript；
2. 创建 Bridge 并注册 `message` 监听；
3. 发送 `sub2api.plugin.ready`；
4. 调用 `config.load` 渲染表单；
5. 编辑后调用 `config.save`；
6. 测试前先保存，再调用 `config.test`；
7. 页面卸载时调用 `dispose()`。

当前 Bridge 支持：

| 消息 | 用途 |
| --- | --- |
| `config.load` | 读取当前配置。 |
| `config.save` | 提交配置，由运行时校验、应用并加密保存。 |
| `config.test` | 运行已保存配置的诊断。 |
| `ui.resize` | 调整配置 iframe 高度。 |
| `ui.notify` | 显示成功、错误或提示消息。 |

每条消息都必须带 `request_id`，并校验 `event.source`、消息来源标识和 Bridge Token。不要依赖 CDN、远程脚本、Cookie 或本地存储。页面需要兼容窄屏和明暗主题，并正确处理加载、保存、测试、超时和未保存状态。

详细信封格式见 `backend/pkg/pluginapi/docs/ui-bridge.md`。如果后续示例仓库提供可复用的 Bridge SDK，本文会在示例仓库章节补充对应路径和使用方式。

## 6. 编写包清单

只维护 `manifest.source.json`，不要手工编辑构建目录中的 `manifest.json`。至少需要填写：

```json
{
  "schema_version": 1,
  "id": "example.openai.transport",
  "name": "Example OpenAI Transport",
  "version": "0.1.0",
  "requires": {
    "sub2api": ">=0.1.179 <0.2.0",
    "recommended_sub2api_version": "0.1.179",
    "tested_sub2api_versions": ["0.1.179"],
    "plugin_protocol": 1,
    "transport_api": 1,
    "ui_bridge": 1
  },
  "capabilities": [
    {
      "id": "openai.oauth.outbound_transport.v1",
      "platform": "openai",
      "account_type": "oauth"
    }
  ],
  "runtimes": {},
  "ui": { "entrypoint": "ui/index.html" },
  "files": {}
}
```

打包器会自动填充目标平台运行时、UI 和运行时文件的 SHA-256。清单中的 `requires.sub2api` 是硬兼容范围；`tested_sub2api_versions` 应只填写真实验证过的版本；`recommended_sub2api_version` 用于管理页面展示。当前宿主仅处理 `openai.oauth.outbound_transport.v1`，声明其他能力不会自动产生新路由。后续增加 Provider 支持时，会在协议、能力清单和宿主路由完成适配后，再补充对应的清单示例。

## 7. 生成密钥并签名

生产包应始终签名，宿主默认拒绝未签名包。可以使用插件工程中的密钥生成工具生成一对 Ed25519 密钥；示例仓库发布后会提供标准工具和完整命令：

```bash
go run ./tools/keygen -out build/keys/my-publisher
```

生成的 `my-publisher.private` 只保存在受控的开发机或 CI Secret 中，不能提交到源码仓库、插件包或部署服务器。公钥是 Base64 文本，可以提供给部署者。

插件工程的 `build.sh` 应调用标准打包器。自定义发布者密钥时必须同时提供 `-signing-key` 和 `-key-id`：

```bash
./build.sh \
  -signing-key /安全目录/my-publisher.private \
  -key-id my-publisher-v1 \
  -output dist/my-openai-plugin.s2plugin
```

签名覆盖最终 `manifest.json` 的精确字节；清单中的文件哈希再覆盖运行时和 UI 文件。签名完成后不要重新格式化 `manifest.json`。

部署者在 Sub2API 配置文件中追加公钥：

```yaml
plugins:
  allow_unsigned: false
  trusted_publishers:
    my-publisher-v1: "BASE64_ED25519_PUBLIC_KEY"
```

`trusted_publishers` 是在宿主内置官方公钥之外追加的信任来源，不能覆盖内置公钥。`signature.json` 中的 `key_id` 必须与配置键完全一致。密钥轮换时先发布包含新公钥的宿主配置或版本，再发布新签名包，最后再停用旧密钥。

开发阶段如需使用未签名包，只应在隔离的本地环境临时设置 `plugins.allow_unsigned: true`，测试完成后立即恢复为 `false`。

## 8. 构建、测试和安装

在插件目录执行：

```bash
go test ./... -count=1
node --check ui/assets/bridge-v1.js
node --check ui/assets/app.js
./build.sh
unzip -t dist/*.s2plugin
```

回到 Sub2API 仓库根目录后，再使用真实构建包运行宿主集成测试：

```bash
cd ../..
SUB2API_TEST_PLUGIN_PACKAGE=plugins/my-openai-plugin/dist/my-openai-plugin.s2plugin \
  go test ./backend/internal/service -run '^TestPluginRuntimeIntegration$' -count=1
```

最低测试集应覆盖配置默认值和边界值、插件身份、请求和响应分块、流式响应、上下文取消、插件退出、代理开关、包哈希、签名、路径安全、目标平台运行时以及 UI Bridge 的加载、保存、测试、错误和超时。

安装后先保持停用，确认清单兼容性、签名和诊断结果，再按账号灰度启用。API Key 账号和未命中灰度的 OAuth 账号继续走 Sub2API 原有路径。

## 9. 发布前检查清单

- 插件版本与 `GetInfo` 返回值一致；
- `requires.sub2api` 覆盖范围经过验证，没有未经测试的破坏性版本；
- `tested_sub2api_versions` 与实际测试记录一致；
- 每个支持的平台和架构都有运行时文件；
- 生产包存在有效 `signature.json`，公钥已交付部署者；
- 包中没有私钥、源映射、测试数据、日志和临时文件；
- UI 不依赖外部资源，也不保存宿主会话信息；
- 配置切换、请求取消、响应关闭和错误重试语义经过测试；
- 发布说明包含升级、停用、回滚和兼容版本信息。

## 10. 常见问题

| 现象 | 排查方向 |
| --- | --- |
| 安装提示签名不受信任 | 检查 `signature.json.key_id`、Base64 公钥和配置键是否完全一致。 |
| 插件显示不兼容 | 检查 `requires.sub2api`、`plugin_protocol`、`transport_api` 和 `ui_bridge`。 |
| 插件进程无法启动 | 检查目标系统和架构对应的运行时路径、可执行权限和运行用户权限。 |
| 配置页无法加载 | 检查 `ui.entrypoint`、UI 文件哈希、Bridge Token 校验和 iframe 消息来源。 |
| 保存后配置未生效 | 查看 `ValidateConfig`、`ApplyConfig` 返回的规范化配置和诊断信息。 |
| 请求失败后重复执行 | 检查 `ForwardResponseError.request_sent` 是否准确反映请求是否可能已发出。 |

## 11. 需要扩展能力时

如果新插件需要支持其他 Provider、其他账号类型或新的消息字段，应先扩展并版本化公开协议，再由宿主增加能力匹配和生命周期处理。不要仅通过清单声明一个宿主尚未实现的能力。这样可以让旧插件继续运行，也能让新宿主明确拒绝不兼容的插件。

Sub2API 后续会持续补充更多 Provider 的插件适配说明，包括能力标识、请求和响应契约、配置字段、UI Bridge 使用方式、版本兼容要求以及测试清单。本文会随着这些能力的落地继续更新，Provider 专属章节会放在本节之后。

## 12. 示例仓库预留

后续计划提供独立的插件示例仓库，用于存放可复用的运行时骨架、UI 组件、打包工具和各 Provider 的最小实现。目前示例仓库尚未准备完成，因此暂不提供地址；正式发布后会在这里补充仓库地址、适用的 Sub2API 版本、示例插件版本和构建说明。
