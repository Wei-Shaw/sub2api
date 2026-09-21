# Codex 接入 Bestloong（通过 CC Switch）

> 适用对象：已开通 Bestloong（界面显示名 Bestloongai）账号、需要在 Windows 上运行 Codex（OpenAI Codex CLI）并调用 Bestloong API 的用户。
> 配置方式：使用 CC Switch 桌面应用统一管理 Codex 的供应商配置（免手动改配置文件）。
> 接口地址（Base URL）：`https://token.bestloongai.com/v1`（OpenAI 兼容格式）

## 一、前置准备
- 已注册 Bestloong 账号并登录
- 已安装 Codex CLI（OpenAI Codex）
- 已安装 CC Switch 桌面应用（Windows：从 GitHub Releases 下载 `.msi` 安装包或便携 `.zip`）
- 准备好可用的 API 密钥（见步骤二）

## 二、获取 API 密钥
1. 登录 Bestloong 平台，进入左侧「API 密钥」。
2. 点击右上角「创建密钥」，生成以 `sk-` 开头的密钥。
3. 点击密钥旁的复制按钮，将密钥复制并妥善保存。

<div align="center">
  <img src="images/wb-01.png" alt="Bestloong 平台「API 密钥」页——创建并复制 sk- 开头的密钥" style="max-width:100%; width:720px;" />
</div>

## 三、方案 A（首选）：Bestloong 一键导入到 CC Switch
Bestloong 平台支持通过 `ccswitch://` 深度链接，将 Bestloong 供应商配置**一键导入** CC Switch，自动填好接口地址、密钥与模型，**推荐优先使用本方案**。

1. 登录 Bestloong 平台，进入左侧「API 密钥」页。
2. 找到你要使用的密钥所在行，在右侧「操作」列点击「**导入到 CCS**」按钮（位于「使用说明」「禁用」「编辑」之间）。
3. 系统会自动唤起已安装的 CC Switch，并弹出「**确认导入供应商配置**」弹窗，预览将要写入的配置（应用类型 Codex、供应商名称、API 端点、API 密钥、模型、用量查询等）。
4. 核对无误后点击「**导入**」，CC Switch 即完成配置写入，无需手动填写。

<div align="center">
  <img src="images/ca-01.png" alt="Bestloong「API 密钥」页——目标密钥行「操作」列点击「导入到 CCS」" style="max-width:100%; width:720px;" />
</div>

<div align="center">
  <img src="images/ca-02.png" alt="CC Switch「确认导入供应商配置」弹窗——预览应用类型、供应商名称、API 端点、密钥、模型后点击「导入」" style="max-width:100%; width:720px;" />
</div>

> 提示：若点击后 CC Switch 未被自动唤起，请确认已安装 CC Switch 桌面应用，并允许浏览器/系统打开 `ccswitch://` 协议链接。

## 四、方案 B（备选）：手动在 CC Switch 添加 Bestloong 供应商
若一键导入操作不成功（如未安装 CC Switch、协议未关联、`ccswitch://` 无法唤起），再采用本手动方案。

1. 启动 CC Switch，在主界面顶部切换到 **Codex** 标签页。
2. 点击「添加供应商」，进入「添加新供应商」页面；在预设供应商列表左上角选择「**自定义配置**」，再点击右下角「**+ 添加**」进入表单。

<div align="center">
  <img src="images/cs-01.png" alt="CC Switch「添加新供应商」页——左上角选择「自定义配置」" style="max-width:100%; width:720px;" />
</div>

3. 在自定义供应商表单中按如下填写：
   - **供应商名称**：`Bestloong`（便于在列表里识别）
   - **官网链接**（可选）：`https://token.bestloongai.com`
   - **API Key**：粘贴步骤二复制的 `sk-` 密钥
   - **API 请求地址**：`https://token.bestloongai.com/v1`（表单下方提示"填写兼容 OpenAI Response 格式的服务端点地址"，与平台协议一致，直接填即可）

<div align="center">
  <img src="images/cs-02.png" alt="CC Switch 自定义供应商表单——填入供应商名称、官网链接、API Key、API 请求地址" style="max-width:100%; width:720px;" />
</div>

4. 展开「**高级选项**」，完成模型配置：
   - 打开「**需要本地路由映射**」开关（Codex 原生只支持 OpenAI Responses API 与 GPT 系列模型，开启后 CC Switch 会在本地自动完成协议转换）；
   - 在「模型映射」处点击「**获取模型列表**」拉取 Bestloong 可用模型，再点「**+ 添加模型**」；
   - 在下拉框中选择要使用的模型（如 `gpt-5.6-sol`）。

<div align="center">
  <img src="images/cs-03.png" alt="CC Switch 高级选项——打开本地路由映射，获取模型列表并添加模型" style="max-width:100%; width:720px;" />
</div>

5. 点击右下角「**+ 添加**」保存。CC Switch 会自动把配置写入 Codex 的配置文件，无需手动编辑；随后在供应商列表中**启用**该配置。

> 提示：接口地址需以 `/v1` 结尾；模型名必须与 Bestloong「模型广场」中的"调用名"完全一致，否则会报 404。

## 五、运行并测试
1. **重启 Codex**（关闭已打开的终端 / 重开 Codex，使新配置生效）。
2. 在目标项目目录下运行 `codex`，发送一条测试请求（如"帮我解释这段代码"）。
3. 收到正常回复、模型按预期返回结果，即表示接入成功。

## 六、常见问题
- **一键导入无反应 / CC Switch 未唤起**：确认已安装 CC Switch 桌面应用；允许系统打开 `ccswitch://` 链接；若仍不行，改用方案 B 手动配置。
- **401 未授权**：密钥复制不完整或未生效，重新复制 `sk-` 开头密钥；并确认 CC Switch 中该供应商已"启用"。
- **404 找不到模型**：接口地址漏写 `/v1`，或模型名与「模型广场」调用名不一致。
- **配置不生效**：切换供应商后需**重启 Codex / 重开终端**；CC Switch 仅改写配置文件，运行中的会话不会热加载。
- **请求超时**：接口地址填写错误，或网络无法访问 `token.bestloongai.com`。

---

> 图片状态：全部 6 张已就位——`wb-01`（密钥页，复用）、`ca-01`（导入到 CCS 入口）、`ca-02`（确认导入弹窗）、`cs-01`/`cs-02`/`cs-03`（手动配置表单及高级选项）。“运行测试”步骤无需配图。步骤描述已按实际界面校准，后续如界面变动以实际为准。
> 协议说明（研发已确认）：Codex 端固定走 Responses；Sub2API 平台自动把 Responses 转成 chat/completions 再发给 Bestloong，平台侧需开启「强制 Chat Completions」（研发/管理员配置，用户无需操作）。
