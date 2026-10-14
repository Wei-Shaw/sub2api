# 账号库分组、首次 2FA 换绑与 OAuth 授权

这个助手连接你指定的 sub2api 后台，在可见的独立浏览器窗口中处理管理员已排队的首次 2FA 换绑或 OAuth 授权任务。账号库可多选后直接点击批量操作，按所选数量最多同时处理 4 个账号，其余账号排队。Windows 默认直接启动本机 Microsoft Edge 的独立 InPrivate 窗口，并通过仅监听本机的调试端口连接这个新窗口；其他系统默认使用 Playwright Chromium。每个账号使用独立浏览器会话，Windows 窗口标题标明账号编号，并使用新的临时资料目录。只有确认新 2FA 生效、账号库保存完成后才显示换绑成功，随后自动关闭该账号的窗口并清理临时目录。

需要邮件码、人机验证或 Passkey 的账号独立等待你操作，其他账号继续运行。主动关闭窗口会暂停该账号；若远端修改结果不确定，停止领取新账号，已运行的账号继续核对结果。降低并发数不会中断正在换绑的账号，等运行数量降下来后再领取新任务。

ChatGPT 登录时，助手在可识别的官方登录字段里填入保存的邮箱、密码和原 2FA 码，并在页面加载稳定后尝试点击普通登录按钮。邮件码、CAPTCHA、Passkey 或风控确认由你在独立浏览器窗口完成。确认登录的是账号库对应账号后，助手自动执行首次 2FA 换绑。助手不绕过验证，不直接重放密码登录或 Sentinel 接口。

## 同机自动启动

### 自动恢复凭证和查询额度

账号库的“凭证失效自动重新授权”开关会保存到数据库。启用后，服务器每分钟查询已经通过账号库授权、关联到系统的 OpenAI OAuth 账号，关闭页面后仍会工作。查询使用只读额度接口，不通过生成回答推测额度；显示接口返回的实际窗口（包括 30 天）、剩余比例、重置时间和真实查询时间。查询失败显示未知，不把历史快照当成当前余额。“立即查询额度”可手动刷新。

常规令牌刷新由系统原有刷新逻辑处理。仅在明确的 401 凭证撤销，或已确认刷新令牌失效时，自动排队一次正常 OAuth 重新授权，并更新原系统账号。网络失败、403、429 不启动重新登录。同一套失败凭证只自动尝试一次，账号恢复尝试至少间隔 30 分钟；暂停任务或恢复失败需要在页面处理，防止反复弹窗。禁用的系统账号不自动恢复。开关依赖上述同机助手配置，管理员身份或令牌版本失效后自动停用。

“一键恢复失效凭证”检查全部已关联账号，不受分页、搜索、分组或勾选影响。它复查已确认失效、自动恢复受限及访问令牌已到期的凭证，优先沿用正常刷新；仍明确失效（或过期且缺少刷新令牌）才排队 OAuth 登录。运行中或暂停中的任务会跳过，暂停任务可用原有“恢复”操作处理。完成提示分别统计排队、查询后可用、失败和跳过数量。每行默认只显示授权状态及剩余额度；授权记录、导出 JSON、查询时间和重置日期可展开详情查看。

邮箱、密码、保存的验证器 2FA 码自动填写。可识别的普通登录、账号选择、授权继续及介绍关闭按钮在可点击后随机等待 1–2 秒；此间导航发生变化时取消该次点击。CAPTCHA、邮件验证、Passkey 及额外身份检查等待真人完成。授权导入成功后关闭对应助手窗口。

账号库分组保存在服务器数据库中。点击每行的分组可设置名称，勾选多个账号后可批量设置；输入已有名称会归入同一组，输入新名称会创建新分组。名称最多 64 个字符，留空或点击“移回未分组”可取消归组。上方支持按组筛选，并与邮箱搜索、分页组合使用。账号库分组用于整理凭证，不会修改网关账号的调用分组、密码、2FA 或 OAuth 凭据。

如果后台和有桌面的浏览器运行在同一台机器，可以在后台进程环境中设置：

```text
ACCOUNT_VAULT_AUTO_WORKER=1
ACCOUNT_VAULT_AUTO_WORKER_NODE=<Node 24 可执行文件的绝对路径>
ACCOUNT_VAULT_AUTO_WORKER_SCRIPT=<本目录 src/cli.mjs 的绝对路径>
ACCOUNT_VAULT_AUTO_WORKER_ORIGIN=http://127.0.0.1:8080
```

Linux 使用 Playwright Chromium，无需安装 Edge。若服务器访问 ChatGPT 必须经过本机代理，再设置 `ACCOUNT_VAULT_BROWSER_PROXY=http://127.0.0.1:7890`；也支持本机 SOCKS5 代理。代理地址只接受回环地址和无账号密码的 URL。可见浏览器需要管理员能访问的 Linux 桌面会话，邮件验证码、CAPTCHA、Passkey 仍需本人在窗口中完成。

先在本目录安装依赖和配套 Chromium。启用后，在后台点击“首次换绑”、导入并勾选换绑，或恢复任务时，服务会自动创建只属于当前管理员的助手连接码，并通过子进程标准输入发送；浏览器页面无需生成或复制连接码。已有排队任务在管理员打开账号库时也会启动。后台地址必须指向本机回环地址，助手子进程只继承运行浏览器所需的少量环境变量，不继承数据库密码或账号库密钥。

页面每两秒读取每个账号的任务进度，显示阶段、步骤和大致剩余时间。助手自动填写邮箱、密码、已有 2FA 并点击普通登录按钮；需要额外验证时，由你在对应窗口完成，助手约每 1.5 秒重新检查并自动继续。人工验证耗时无法准确预估；远端换绑步骤显示预计时间。换绑成功后自动关闭对应窗口。

## Windows 启动

1. 从 [Node.js 官网](https://nodejs.org/) 安装 **Node.js 24 LTS**，并确认本机安装 Microsoft Edge。
2. 在后台以管理员身份登录，按后台已配置的敏感操作策略完成验证，进入账号库并排队首次换绑任务，生成 **8 小时本地助手连接码**。连接码以 `avw1_` 开头；不要使用管理员 JWT、全局 API Key 或其他账号 token。
3. 双击本目录的 `start.cmd`。首次运行会按 `package-lock.json` 安装依赖，并使用 Playwright 官方安装器安装本版本 Chromium。
4. 输入后台 **origin**，例如 `https://panel.example.com`。不要加 `/api/v1`、路径、查询参数或连接码。
5. 在随后出现的隐藏输入提示中粘贴连接码并按回车。输入不会回显，也不会写入脚本、文件或命令历史。
6. 保持终端与可见浏览器开启。额外验证应在标题对应的账号窗口内完成，并发模式自动检查并继续；单窗口手动启动模式可在终端按回车继续。

也可以只在命令行传入不含凭据的地址：

```bat
start.cmd --origin https://panel.example.com
```

手动启动时可设置助手最多打开的窗口数（后台设置仍是实际并发上限）：

```bat
start.cmd --origin https://panel.example.com --concurrency 4
```

处理至多一个账号后退出：

```bat
start.cmd --origin https://panel.example.com --once
```

命令行不接受连接码参数。后台地址必须为 HTTPS；只有 `localhost`、`127.0.0.0/8` 和 `::1` 允许 HTTP。后台 API 请求拒绝重定向，不允许通过跳转把连接码发到另一地址。

Windows 助手默认启动独立 Edge InPrivate 窗口，不接管你已打开的 Edge 无痕会话；如需诊断性地改回 Chromium，可给手动启动的助手设置 `ACCOUNT_VAULT_BROWSER_CHANNEL=chromium`。Playwright 1.64.0 官方支持的 Windows 范围是 **Windows 11+ / Windows Server 2019+**；不要把本目录当作已验证的 Windows 10 兼容包。[官方系统要求](https://playwright.dev/docs/intro)

## macOS / Linux

同样先安装 Node.js 24。在有桌面显示环境的终端运行：

```sh
sh start.sh --origin https://panel.example.com
```

也可以手动准备并启动：

```sh
npm ci --ignore-scripts
npm run install:browser
npm start -- --origin https://panel.example.com
```

Linux 如缺少浏览器系统依赖，请按 [Playwright 官方浏览器安装说明](https://playwright.dev/docs/browsers)准备依赖。生产入口始终使用可见浏览器和正常沙箱；没有无头登录、关闭 Web 安全、stealth 或接管你现有浏览器的开关。

## 运行与暂停

- 成功任务自动串行继续；修改远端 2FA 之前暂停的账号不会阻塞后续队列。修改阶段出现不确定状态时，本轮助手停止领取后续账号。
- 等待人工登录时，后台进度为 `awaiting_user`，当前租约仍每 30 秒续约。它不同于释放租约的最终“暂停”。
- 如果初次打开 ChatGPT 超时，后台进度为 `awaiting_navigation`；助手保留 Edge 窗口，用户可在该窗口地址栏手动打开 `https://chatgpt.com`，登录后助手继续。
- `Ctrl+C` 会立即停止后续动作、关闭本助手的账号上下文，并尽力向后台报告暂停。它不会关闭你原来打开的浏览器。
- 连接码过期、撤销权限、账号被取消或租约冲突会中止当前 UI 等待和后续写操作。需按后台状态处理后再启动。
- 不要同时在官方账号设置中更改同一账号的 2FA。助手会重复核对上游账号身份和固定 factor ID，发现不一致即暂停。
- 如果窗口已被你关闭，当前任务会暂停。无需通过反复点击“重新导入”恢复任务。

## 换绑顺序与恢复边界

后台是唯一的状态与密钥存储来源。助手先获得当前账号的短期租约，再登录、查询 `/backend-api/me`，核对邮箱及已绑定的上游账号 ID。旧因素必须是唯一的 TOTP 且与默认 factor 一致。

每一个上游写动作都必须先收到后台已经持久化的独立 permit：

1. 保存关闭 intent，取得固定旧 factor ID，再关闭旧因素。
2. 查询上游，确认旧因素消失、TOTP 列表为空且两个 enabled 标记均为 false。
3. 保存 enroll intent，再申请新 Secret。
4. **先把新 Secret、session 和 factor ID 发送后台加密保存，收到保存确认后，才可以申请 activate permit。**
5. 向后台读取新验证码，再激活对应 enrollment。
6. 查询上游，只有两个 enabled 标记均为 true、TOTP 恰好只有新 factor、默认 factor 也是新 ID、旧 ID 已消失，才请求后台原子完成更新。

网络失败不等于操作失败。恢复时遵循以下边界：

| 保存的阶段 | 助手如何处理 |
| --- | --- |
| `disable_intent` | 只先查询；确认旧因素已消失后继续，不再次关闭 |
| `enroll_intent` | 未知是否已取得新 Secret，停止自动恢复，不再次 enroll |
| `enrolled` | 使用后台已经保存的候选数据，不重新申请 Secret |
| `activate_intent` | 只核对新因素；已激活才完成，未能确认时不重发激活 |
| `completed` | 保持完成，不重新执行首次换绑 |

若 checkpoint 已提交但响应丢失，助手可通过同一有效租约的 heartbeat 读取最新 revision；这只用于观察和暂停，**不视为新的远端写许可**。最终提交成功会清除租约，因此丢失完成响应后，heartbeat 也可能返回租约失效。终端会提示以后台实际进度为准；不能仅凭终端停止推断数据库未完成。

上游成功 enroll 但响应丢失、enrollment 过期、上游结构改变等情况可能需要管理员在官方页面处理。助手不会猜测 Secret、把 `enabled=true` 单独当作新 Secret 生效，或伪造完成标记。原 Secret 是否仍可用应以后台阶段及官方状态为准。

## 凭据与会话

### OAuth 授权添加与重新授权

账号库支持逐个或多选后批量授权，使用现有 1–4 并发队列。每个任务在独立无痕窗口登录，自动填写邮箱、密码、已有 TOTP 并点击普通按钮；人机验证、邮件验证码和 Passkey 交给用户完成，页面显示操作提示。

此流程不读取 `/api/auth/session`、不导出网页 Session 或 cookies、不修改 2FA。登录界面准备好后，在同一窗口打开系统 `OpenAIOAuthService.GenerateAuthURL` 生成的 Codex OAuth 链接。自动选择对应邮箱，已知 consent 页只有一个明确选中工作区且邮箱匹配时自动继续。后台独立校验签发令牌中的邮箱、用户及工作区标识。

普通登录的 `/auth/login_with` 中转页若持续空白，等待 20 秒后会改走系统生成的 OAuth 登录流程，继续使用同一个窗口。人机验证、邮件验证码和 Passkey 优先交给用户处理。主页同时显示顶部和侧栏两个头像时也能识别登录界面已准备好；真正授权成功仍必须完成回调校验、凭据交换和系统导入。

PKCE 会话标识加密绑定到账号、任务和租约；回调仅提交 code 和 state。助手按每个页面的 main-frame 请求和预期 state 提取 localhost 回调，即使本地页面拒绝连接也有效；不会记录回调内容。后台复用系统 `ExchangeCode` 接口，按令牌实际有效期保存 refresh_token 与 access_token，再通过身份匹配创建或更新 OAuth 系统账号，保留已有分组及配置。

页面显示登录、确认授权、交换凭据、导入及完成状态，并显示当前系统凭据的到期时间。完成后自动关闭自己的窗口，可下载系统导入 JSON。再次授权会更新同一身份的账号；旧凭据导致的 error 状态在更新后恢复，管理员设置的账号有效期和禁用状态仍保留。access_token 到期时间保存在 credentials.expires_at，不用它设置账号的 expires_at；refresh_token 有效时系统自动刷新。签发时长以上游实际返回为准，refresh_token 失效则重新授权。系统生成的待授权 PKCE 会话有效 30 分钟，授权 code 一次使用并立即交换。

内部数据库 kind 和 API 的 session 名称保留用于兼容旧作业；新作业不保存网页 Session。
## 固定版本和验证范围

- Node.js：24.x；实现环境为 24.19.0。
- Playwright：精确固定 **1.64.0**，带 npm lockfile；npm 元数据的 Node 要求为 `>=20`，官方文档明确支持 24.x。
- `start.cmd` / `start.sh` 使用 Playwright 官方 CLI 下载与该版本匹配的 Chromium，不下载第三方浏览器打包文件。
- 已验证的是 Linux 环境的纯 Node 故障测试，以及真实 Chromium 上的**本地合成页面与合成 MFA API**。本地浏览器验证使用已有 Chromium 153，并不等同于对 1.64.0 配套 Chromium、Windows 实机或真实 ChatGPT 账号的验收。
- 未读取真实 HAR，尚未验证真实平台完整换绑成功。自动识别不到的页面会交给人工；不得把合成测试通过写成真实平台换绑成功。

测试命令：

```sh
npm test
npm run test:browser
```

浏览器测试默认使用本版本安装的 Chromium。仅用于本地合成测试时，可以通过 `VAULT_TEST_BROWSER` 指向已有浏览器；生产 CLI 不读取这个变量。测试创建的内容全部为公开合成值，不产生凭据图片或浏览器记录文件。

官方参考：[Playwright 安装与系统要求](https://playwright.dev/docs/intro)、[浏览器安装](https://playwright.dev/docs/browsers)、[隔离 BrowserContext](https://playwright.dev/docs/api/class-browser#browser-new-context)、[npm Playwright](https://www.npmjs.com/package/playwright)。
