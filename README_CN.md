# AstroGate 使用说明

当前入口：`https://www.astrogate.sbs` 与 `https://dicksuck.astrogate.sbs`。两个入口都把账号库任务送到同一台受信任的 Linux 后端；浏览器授权窗口在那台机器的桌面打开。中转服务器只负责转发，账号库、PostgreSQL、Redis 和加密数据保存在 Linux 后端。

## 本仓库改了什么

- **OpenAI 默认代理**：配置 `OPENAI_DEFAULT_PROXY_ID` 后，新建 OpenAI 账号及 OpenAI OAuth 授权在未手动指定代理时使用该代理。已有账号可在“账号管理”中编辑代理。
- **批量加入分组**：账号管理页新增“批量加入分组”。它给选中的账号添加目标分组，并保留账号原有的分组；原有“批量编辑账号”中的分组设置仍用于替换分组。
- **账号库自动授权**：Linux 桌面使用系统安装的 Microsoft Edge InPrivate 窗口；Windows 本机助手也使用 Edge InPrivate。普通登录和 OAuth 按钮由助手操作，邮件验证码、人机验证和 Passkey 仍需在弹出的窗口人工完成。

## 1. 登录后台

在浏览器打开 `https://www.astrogate.sbs`，使用管理员账号登录。

## 2. 配置 OpenAI 代理

1. 进入 **代理管理**，添加后端能够访问的 HTTP 或 SOCKS5 代理，并确认代理处于启用状态。代理运行在另一台机器时，填写那台机器可从后端访问的地址。
2. 将该代理的 ID 写入服务的 `.env`：

   ```dotenv
   OPENAI_DEFAULT_PROXY_ID=<代理ID>
   ```

3. 重启后端服务，让环境变量生效。已有 OpenAI 账号需要进入 **账号管理 → 编辑**，给账号选择代理。

如果使用 Clash 的规则模式，应让 OpenAI 和 ChatGPT 域名走所需节点，并确认该节点的出口可访问对应服务。`SUB2API_HTTP_PROXY` 和 `SUB2API_HTTPS_PROXY` 可用于设置后端的通用 HTTP 代理；按账号选择的代理会用于该账号的上游请求。

## 3. 添加及重新授权 OpenAI 账号

在 **账号库** 中录入账号后，可选择账号并点击 **授权添加账号 / 更新**；暂停的账号点击 **恢复任务**。任务由 Linux 后端接收，在它的桌面打开独立 Edge InPrivate 窗口。访问管理页面的电脑不会弹出授权浏览器。助手核对 OAuth 身份后导入或更新系统账号，任务完成才显示成功；关闭授权窗口会暂停该账号。

自动助手需要 Linux 图形桌面会话保持登录。当前部署使用 [账号库助手的 Linux 启动脚本](tools/account-vault-worker/deploy/linux/node-with-display.sh)；它通过用户级 systemd 服务启动 Edge，保留后端服务的 `NoNewPrivileges` 限制。安装和恢复说明见 [账号库助手文档](tools/account-vault-worker/README.md)。

以下是无法使用自动助手时的手动授权方式：

1. 进入 **账号管理 → 添加账号**，选择 **OpenAI** 和 **OAuth**。
2. 生成授权链接，在浏览器中打开并完成登录。
3. 授权后浏览器可能跳到 `localhost:1455/auth/callback?...` 并显示“无法连接”。这是手动授权流程中的回调地址；复制浏览器地址栏中的完整回调 URL，粘贴回添加账号窗口，点击 **完成授权**。
4. 在账号列表中确认状态正常，并按需检查账号的代理和可用模型。

如果授权时报 `unsupported_country_region_territory`，检查浏览器和服务器端代理的出口地区；如果调用时报连接超时，检查该账号实际绑定的代理及其到 `chatgpt.com` 的连通性。

## 4. 批量加入分组

1. 在 **分组管理** 中创建目标分组。
2. 进入 **账号管理**，勾选需要加入的账号。可以先用顶部筛选器缩小范围，再全选筛选结果。
3. 点击 **批量加入分组**，选目标分组并确认。

操作完成后，账号原有分组仍会保留。如果要用一组新分组替换原有分组，使用 **批量编辑账号**。

## 5. 创建 API 密钥并调用

进入 **我的账户 → API 密钥** 创建密钥。密钥所属用户应有目标分组的使用权限。客户端的 API Base URL 填 `https://www.astrogate.sbs/v1`，API Key 填刚创建的密钥。

例如调用 Responses 接口：

```bash
curl https://www.astrogate.sbs/v1/responses \
  -H "Authorization: Bearer <你的 Sub2API API Key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.6","input":"hi"}'
```

模型名以已添加账号和分组实际开放的模型为准。

## 6. 在 Codex CLI 中使用

在用户配置目录创建 `~/.codex/sub2api.config.toml`；Windows 路径为 `%USERPROFILE%\.codex\sub2api.config.toml`：

```toml
model = "gpt-5.6"
model_provider = "sub2api"

[model_providers.sub2api]
name = "Sub2API"
base_url = "https://www.astrogate.sbs/v1"
env_key = "SUB2API_API_KEY"
wire_api = "responses"
```

在 PowerShell 中启动：

```powershell
$env:SUB2API_API_KEY = "<你的 Sub2API API Key>"
codex --profile sub2api
```

如果 PowerShell 提示找不到 `codex`，先安装 Codex CLI，或使用本机 `codex.exe` 的完整路径运行。配置文件中的密钥环境变量名必须与 PowerShell 中设置的名称一致。Codex 的自定义提供商与配置方案格式见 [官方配置文档](https://learn.chatgpt.com/docs/config-file/config-reference)。
