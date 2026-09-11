# Codex 上游身份与 User-Agent 画像

本文记录 Sub2API 向 Codex 相关上游发送请求时，`User-Agent`、`originator` 和
`version` 的来源与处理规则。`/backend-api/codex/*` 是内部协议，本文不是 OpenAI
公开 API 的稳定合同。

## 1. 结论与证据边界

实现依据分为三层，不混用“官方”“实测”“参考方案”：

| 结论 | 证据 | 可信边界 |
|---|---|---|
| App Server 初始化包含 `clientInfo` | OpenAI App Server 文档 | 官方公开协议 |
| `clientInfo.name/version` 被写入 UA 尾部 | `openai/codex` 源码 | 官方开源实现 |
| TUI 传入 `codex-tui` 和自身构建版本 | `openai/codex` 的 `rust-v0.147.0` TUI 源码 | embedded 同构建时才与 Core 同版 |
| remote TUI 传入自身构建版本，远端 Core 可不同 | 官方 App Server 文档、TUI remote 连接和 initialize 映射源码 | 2026-09-11 核验；不代表任意配置元组均经过制品验真 |
| OpenAI provider 推理请求带 `version` | `openai/codex` provider 源码 | 官方 0.147.0 实现 |
| Desktop 使用独立的 Core/Desktop 版本 | 官方 appcast/发布包、`initialize` 返回值和回环抓包 | 2026-09-11 对一个 macOS 发布制品的直接验证 |
| Desktop 制品原子更新方案 | `zyycn/codex-proxy-rs` | 第三方参考方案，不是 OpenAI 协议合同 |
| 默认 Desktop 完整画像 | 本机官方发布制品、`initialize` 返回值和回环抓包 | 用户指定的 2026-09-11 本机基准，不代表所有机器或最新版本 |

来源：

- [Codex App Server 初始化文档](https://developers.openai.com/codex/app-server#initialization)
- [Codex TUI 远程连接文档](https://learn.chatgpt.com/docs/app-server#connect-the-cli-terminal-ui)
- [TUI remote 连接的 client_name/client_version](https://github.com/openai/codex/blob/main/codex-rs/tui/src/lib.rs)
- [RemoteAppServerConnectArgs 到 initialize.clientInfo 的映射](https://github.com/openai/codex/blob/main/codex-rs/app-server-client/src/remote.rs)
- [TUI 0.147.0 传入 `client_name=codex-tui` 和构建版本](https://github.com/openai/codex/blob/rust-v0.147.0/codex-rs/tui/src/lib.rs)
- [App Server 0.147.0 处理 `clientInfo` 和 UA suffix](https://github.com/openai/codex/blob/rust-v0.147.0/codex-rs/app-server/src/request_processors/initialize_processor.rs)
- [Codex 0.147.0 UA 与 default headers 的生成](https://github.com/openai/codex/blob/rust-v0.147.0/codex-rs/login/src/auth/default_client.rs)
- [authorization-code exchange 使用 raw auth client](https://github.com/openai/codex/blob/rust-v0.147.0/codex-rs/login/src/server.rs)
- [OAuth refresh 使用 default auth client](https://github.com/openai/codex/blob/rust-v0.147.0/codex-rs/login/src/auth/manager.rs)
- [OpenAI provider 0.147.0 的 `version` 头](https://github.com/openai/codex/blob/rust-v0.147.0/codex-rs/model-provider-info/src/lib.rs)
- [Codex 0.153.4 终端检测与 `unknown` 回退](https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/terminal-detection/src/lib.rs)
- [Codex Desktop 官方 appcast](https://persistent.oaistatic.com/codex-app-prod/appcast.xml)
- [参考项目 `codex-proxy-rs`](https://github.com/zyycn/codex-proxy-rs)

### 1.1 本机实证（2026-09-11）

验证对象是 `/Applications/ChatGPT.app`（bundle id `com.openai.codex`），并用官方
appcast 中同版本的发布 ZIP 交叉验证，不是根据第三方示例反推：

| 证据 | 实测值 |
|---|---|
| `CFBundleShortVersionString` / `app.getVersion()` | `26.901.41600` |
| App 内置 `Contents/Resources/codex --version` | `0.153.4` |
| 打包代码传给 app-server 的 `clientInfo` | `name="Codex Desktop"`，`version=app.getVersion()` |
| 本机 `/Applications/iTerm.app` | `CFBundleShortVersionString=3.6.10` |
| 正在运行的 Desktop app-server 进程 | 官方终端检测器读取的变量全部不存在 |
| `rust-v0.153.4` 官方终端检测逻辑 | 未命中任何终端变量时输出 `unknown` |
| 真实 Desktop 终端 token | `unknown` |

appcast 中存在 `26.901.41600` / build `7982` 发布项。下载该项的 ZIP 后，以 App 内
`SUPublicEDKey` 验证 appcast 的 Ed25519 `sparkle:edSignature` 结果为 `true`；解包后的
`Info.plist`、`app.asar` 和内置 `codex` 与当前安装副本逐字节一致。macOS
`codesign --verify --deep --strict` 对发布 ZIP 和当前安装副本都报告 invalid，因此本文不把
Apple code-signing 校验作为阳性证据，采用 Sparkle 发布签名与关键文件一致性作为制品来源证据。

同机独立 CLI `0.147.0` 在终端测试环境中以 TUI `clientInfo` 初始化后返回：

```text
codex-tui/0.147.0 (Mac OS 15.6.0; arm64) xterm-256color (codex-tui; 0.147.0)
```

同机内置 Core 在带 `TERM=xterm-256color` 的测试环境中以 `codex exec` 发请求时，
回环端点捕获到：

```text
User-Agent: codex_exec/0.153.4 (Mac OS 15.6.0; arm64) xterm-256color (codex_exec; 0.153.4)
originator: codex_exec
version: 0.153.4
```

上述两次手工测试证明了字段来源和请求头版本关系，但其中的 `xterm-256color` 来自测试
进程环境，不能作为 Desktop 证据。真实 Desktop app-server 由 GUI 启动，没有官方检测器
读取的任何终端变量，按 `rust-v0.153.4` 源码确定输出 `unknown`。本机安装的 iTerm2
`3.6.10` 只影响从 iTerm2 启动的 CLI/TUI，不属于 Desktop 画像。

## 2. `ClientInfo` 从哪里来

`clientInfo` 不是 Sub2API 发明的 HTTP 头，而是 App Server JSON-RPC 初始化参数：

```json
{
  "method": "initialize",
  "params": {
    "clientInfo": {
      "name": "codex-tui",
      "version": "0.147.0"
    }
  }
}
```

官方实现的处理链是两个独立输入：

```text
process/default originator（可由 CODEX_INTERNAL_ORIGINATOR_OVERRIDE 覆盖）
  -> User-Agent 前缀
  -> HTTP originator header

initialize.params.clientInfo.name/version
  -> USER_AGENT_SUFFIX = "{name}; {version}"
  -> User-Agent 尾部 "({name}; {version})"
```

本机官方 Desktop Core 的参数矩阵也直接证明二者不要求同名：以
`clientInfo={name:"codex_vscode", version:"9.8.7"}` 初始化同一二进制时，它返回：

```text
Codex Desktop/0.153.4 (Mac OS 15.6.0; arm64) unknown (codex_vscode; 9.8.7)
```

前缀仍是 Desktop 制品的 originator，只有尾部来自 `clientInfo`。

因此代码中的 `CodexClientInfo` 只是从已配置 UA 尾部解析出的内部结构：

```go
type CodexWireProfile struct {
    Originator        string
    CoreVersion       string
    RuntimeDescriptor string
    ClientInfo        *CodexClientInfo
}
```

它用于把 UA 前缀身份与尾部 frontend 信息分别建模，不会作为第四个请求头发送。

## 3. UA 结构

官方 `get_codex_user_agent()` 的基础结构是：

```text
{originator}/{core_version} ({os_type} {os_version}; {arch}) {terminal}
```

App Server 初始化过真实 frontend 后追加：

```text
({clientInfo.name}; {clientInfo.version})
```

### 3.1 TUI

TUI 的 `clientInfo.version` 来自 **TUI 自己的构建版本**。2026-09-11 核验的官方
`connect_remote_app_server` 实现传入：

```text
client_name    = codex-tui
client_version = CARGO_PKG_VERSION
```

只有明确是 embedded/同一构建时，才能确认 TUI 与 Core 同版：

```text
codex-tui/{core} ({os} {os_version}; {arch}) {terminal} (codex-tui; {core})
```

remote 路径则把这些参数经 `RemoteAppServerConnectArgs::initialize_params` 发给远端。
远端 `get_codex_user_agent()` 使用服务端自身的 `CARGO_PKG_VERSION` 作为前缀版本，
`initialize` 将 TUI 传来的 `clientInfo` 写入尾部。因此协议允许：

```text
codex-tui/{remote_core} ({remote_os} {remote_os_version}; {remote_arch}) {remote_terminal} (codex-tui; {local_tui})
```

`remote_core` 与 `local_tui` 可以不同。之前本文从“传入 CARGO_PKG_VERSION”推导
“所有 TUI 必须同版”是错误的，忽略了该常量属于哪个进程。客户端名、版本是否相等、
OS/终端字符串都不能证明连接是 embedded 或二者来自同一制品。

### 3.2 Desktop 双版本

App Server 的 UA 前缀版本来自被嵌入的 Codex Core；尾部版本来自 frontend 传入的
`clientInfo.version`。两者在协议上本来就是两个来源，因此 frontend 独立发布时可以不同。

本机真实 Desktop 画像为：

```text
Codex Desktop/0.153.4 (Mac OS 15.6.0; arm64) unknown (Codex Desktop; 26.901.41600)
```

`codex-proxy-rs` 当前固定画像的另一个 Desktop 制品示例是：

```text
Codex Desktop/0.153.4 (Mac OS 15.7.1; arm64) unknown (Codex Desktop; 26.901.51231)
```

验证当日官方 appcast 的最新项已经是 `26.903.71938`，说明第三方项目里的固定值只能作为
已审计历史画像，不能直接当作“当前真实版本”抄入 Sub2API。

这两份画像的含义是：

- `0.153.4`：两份 Desktop 制品内嵌的 Codex Core；
- `26.901.41600`：本机 Desktop frontend 版本；
- `26.901.51231`：`codex-proxy-rs` 示例的 Desktop frontend 版本；
- HTTP `version`：应取 Core 版本 `0.153.4`。

Sub2API 保留这种双版本元组的理由是避免把“最新独立 CLI 版本”塞进某个 Desktop
制品，拼出不存在的组合。该关系已经用本机官方制品和回环请求直接验证。默认值现固定为
上述本机完整画像；管理员也可显式配置另一份完整画像。Sub2API 尚未实现 Desktop 制品
下载、验签和原子解析，因此默认画像不会随独立 CLI 版本同步自动更新。

## 4. Sub2API 默认数据从哪里来

默认不是从部署机器动态采集，也不是拿 GitHub 最新 CLI 版本临时拼接，而是固定使用
2026-09-11 已验证的本机 Desktop 完整画像：

| 字段 | 来源 |
|---|---|
| `originator` | 本机抓包：`Codex Desktop` |
| Core 版本 | 本机 Desktop 内置 Core：`0.153.4` |
| OS / OS 版本 | 本机 `initialize`：`Mac OS 15.6.0` |
| 架构 | 本机 `initialize`：`arm64` |
| 终端 | Desktop app-server 无终端变量，官方检测回退：`unknown` |
| Desktop `clientInfo.name` | 本机打包代码和抓包：`Codex Desktop` |
| Desktop `clientInfo.version` | 本机 `app.getVersion()`：`26.901.41600` |

手工 app-server/exec 测试曾出现 `xterm-256color`，原因是测试进程继承了该终端环境，
因此不能沿用。若从本机 iTerm2 `3.6.10` 启动 Codex CLI，终端 token 才会是
`iTerm.app/3.6.10`，但对应客户端应是 `codex-tui` 或 `codex_exec`，不能与
`Codex Desktop` 混拼。管理员配置完整 UA 时以配置为准。

当前默认画像是：

```text
User-Agent: Codex Desktop/0.153.4 (Mac OS 15.6.0; arm64) unknown (Codex Desktop; 26.901.41600)
originator: Codex Desktop
version: 0.153.4
```

### 4.1 网站运行特性如何体现

Sub2API 是多用户 Web 网关，不是某位管理员本机上的 Codex 进程。因此网站侧采用一套
站点级上游画像：所有 OpenAI OAuth 推理路径在服务端统一收口 `User-Agent`、`originator`
和 `version`，不会从访问者浏览器、服务器启动终端或入站请求中拼接身份。

管理站点展示“当前生效的上游画像”，用于区分三类数据：

- 输入框为空：使用代码内已核验的完整 Desktop 制品画像；
- 单版本 CLI/TUI 模板：沿用同时重建首尾版本的配置策略，不据此宣称制品来源已验证；
- 管理员双版本画像：包括 Desktop、VS Code、remote TUI，整体保留，不接受 CLI 版本局部覆盖。

站点名称或 Sub2API 版本不写入官方 `originator`。OpenAI 官方文档说明自有 app-server
集成应通过 `clientInfo.name` 标识并申请加入已知客户端列表；Sub2API 当前实现是上游 HTTP
兼容网关，并没有完成该注册流程，不能一边声明官方兼容身份、一边擅自拼入第三方 clientInfo。

CLI 稳定版自动同步仍用于管理员明确配置的单版本 TUI/CLI UA；默认 Desktop 不读取该值，
否则会把 Desktop 的 Core `0.153.4` 局部替换成独立 CLI 版本，而尾部仍保留
`26.901.41600`，形成未经任何真实制品验证的组合。

## 5. 画像处理规则

| 输入 | 处理 | `version` 头 |
|---|---|---|
| 未配置 UA | 使用固定的本机 Desktop 完整画像 | `0.153.4` |
| 无尾部的单版本官方 UA | 保留 client/runtime，更新 Core | 当前生效 Core 版本 |
| 首尾同版本 UA | 同时更新前缀和尾部 | 当前生效 Core 版本 |
| 管理员显式配置的双版本完整 UA（含 remote TUI） | 格式、最低 Core 版本校验后整体保留 | UA 前缀 Core 版本 |
| 非官方或非法画像 | 整体回退默认 Desktop | `0.153.4` |
| 双版本画像 Core 低于最低门槛 | 整体回退默认 Desktop | `0.153.4` |

OAuth 模型清单的 `client_version` 查询参数也由最终画像的 Core 版本生成；下游传入值
不会继续穿透到 ChatGPT 上游。这样 `User-Agent` 前缀、HTTP `version` 和
`client_version` 不会形成来自三个客户端的混合画像。API-key 自定义上游仍保留显式
版本协商能力。

对于 ChatGPT Codex OAuth，`User-Agent`、`originator` 和 `version` 是网关拥有的
上游身份字段。默认强制统一开启时，入站值可以用于准入、兼容路由或诊断，但请求构造器不会把它们复制到
上游；Anthropic/Chat Completions 协议转换路径同样如此。会话与 turn 字段属于另一类：
它们可以作为连续性种子被接收，但必须先经过账号作用域化、隔离、校验或重建才能出站。

显式设置 `gateway.disable_codex_identity_enforcement=true` 时，HTTP、透传、compact、WS
恢复旧的白名单透传与最终 UA 配对逻辑：合法客户端 UA 可保留，`originator` 根据最终 UA
配对；非官方 UA 仍回退规范身份。账号 UA 覆写和 `ForceCodexCLI` 的原有优先级不变。
这不是任意头的原样透传，不会新增 `version` 的入站白名单权限；API-key 路径不受此开关影响。

首尾版本不同只是双版本元组，不证明发布方式或制品真实性。管理员显式配置走以下校验，
不得仅因 `clientInfo.name=codex-tui` 而拒绝：

```text
clientInfo.version != core_version
Core/frontend 版本格式合法，Core 不低于最低门槛
```

`HasDistinctClientVersion` 仅比较两个版本值，不推断 embedded/remote，不要求
originator 与 clientInfo 同名。版本格式非法或 Core 低于门槛时，账号级候选回退当前规范
身份，全局 UA 回退内置 Desktop。仅有 UA 的配置入口没有可信连接模式元数据，不能施加
“同制品必须同版”的断言；内置 embedded 模板则在构造时从同一版本值生成首尾。

管理员配置通过校验不等于已完成制品验证。目前没有新增“已验证组合”白名单；如需此类
强保证，应另行提供可追溯的制品/连接证据。默认仍只使用本机已核验的固定 Desktop 画像，
强制统一开启时不会采纳下游自行提交的双版本 UA。

单版本画像可随稳定版同步；双版本画像不能局部更新。`User-Agent`、`originator`、
`version` 必须从同一次解析结果生成，避免出现例如：

```text
originator: codex-tui
User-Agent: Codex Desktop/...
```

## 6. 真实 Codex 各请求路径的头

不能笼统分成“OAuth 面”和“推理面”；Codex 在 auth 内部还区分 raw/default client。

| 请求路径 | 官方 Codex 行为 | Sub2API 行为 |
|---|---|---|
| authorization-code token exchange | raw auth client；不注入 Codex UA/originator/version | 不注入，并显式压掉 req/v3 默认 UA |
| device-code 请求/轮询、API-key token exchange | raw auth client；不注入三项 | 当前无对应通用转发路径 |
| OAuth refresh | default auth client；UA + originator，无 version | UA + originator，无 version |
| token revoke、PAT whoami | default auth client；UA + originator，无 version | PAT whoami 使用相同身份对；revoke 暂无对应路径 |
| OpenAI/Codex provider 推理请求 | UA + originator + version | UA + originator + version |

`version` 并非由 App Server `clientInfo` 直接变成请求头；它由 OpenAI model provider
的静态 `http_headers` 注入，值是 Codex Core 的 `CARGO_PKG_VERSION`。UA/originator
则来自 default HTTP client。两条代码路径最后在推理请求上合并。

## 7. 代码位置

| 文件 | 责任 |
|---|---|
| `backend/internal/pkg/openai/request.go` | UA 配对、画像解析/渲染、安全校验 |
| `backend/internal/service/openai_gateway_service.go` | 本机验证的默认 Desktop 完整画像常量 |
| `backend/internal/service/openai_codex_identity.go` | 请求级画像选择及身份头生成 |
| `backend/internal/service/setting_gateway_runtime.go` | 配置优先级、单/双版本处理 |
| `backend/internal/service/openai_codex_version_sync_service.go` | CLI/Core 稳定版本同步 |
| `backend/internal/repository/openai_oauth_service.go` | token exchange 与 refresh 的不同 auth-client 语义 |

## 8. 验证

测试覆盖：

- 官方 TUI 首尾版本同步；
- 默认值精确等于本机验证的 Desktop 三元组；
- 独立 CLI 版本同步不会拆开默认 Desktop 元组；
- Desktop 双版本只做完整元组保留；
- 非法、过旧画像回退；
- 管理员双版本 TUI 元组在账号和全局配置中保留；仅格式错误或 Core 过旧时回退；
- 开关启闭覆盖 HTTP、透传、compact、WS，保留 API-key 与覆写优先级；
- originator 与已知 `clientInfo` 不同名时仍按两个独立字段解析；
- 推理请求三头同源；
- auth code exchange 不注入 Codex 身份；
- refresh/PAT 路径只注入 UA + originator；
- HTTP、WebSocket、compact 和探测路径回归。

```bash
cd backend
go test ./...
```

## 9. 尚未解决

当前实现不声称对齐以下内容：

- Desktop appcast、签名制品和内嵌 Core 的原子更新；
- 除本机已验证制品以外的 Desktop 版本元组；
- TLS ClientHello、HTTP/2 SETTINGS、Header 顺序；
- 出口 IP、ASN、代理信誉；
- attestation；
- OpenAI 风控结果或账号状态。

若后续启用 Desktop 自动画像，应参考 `codex-proxy-rs` 的原则，从同一个已验证官方
Desktop 制品原子取得 `Desktop version + build + bundled Core version`；任一步失败时
保留上一份完整画像，而不是只更新其中一个版本。

在此之前，默认画像是有意固定的本机快照。验证当日 appcast 已存在更高版本
`26.903.71938`，所以“按本机为标准”保证的是可验证和内部一致，不等于始终追踪最新版本。
