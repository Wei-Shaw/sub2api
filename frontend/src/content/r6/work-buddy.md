# Work Buddy 接入 Bestloong

> 适用对象：已开通 Bestloong（界面显示名 Bestloongai）账号、需要在 Work Buddy 客户端中调用 Bestloong API 的用户。
> 接口地址（Base URL）：`https://token.bestloongai.com/v1`（OpenAI 兼容格式）

## 一、前置准备
- 已注册 Bestloong 账号并登录
- 已安装 Work Buddy 客户端
- 准备好可用的 API 密钥（见步骤二）

## 二、获取 API 密钥
1. 登录 Bestloong 平台，进入左侧「API 密钥」。
2. 点击右上角「创建密钥」，生成以 `sk-` 开头的密钥。
3. 点击密钥旁的复制按钮，将密钥复制并妥善保存。

<div align="center">
  <img src="images/wb-01.png" alt="Bestloong 平台「API 密钥」页——点击"创建密钥"，复制 sk- 开头的密钥" style="max-width:100%; width:720px;" />
</div>

## 三、在 Work Buddy 中添加模型
1. 打开 Work Buddy，进入「设置 → 模型」，在"自定义模型"区域点击右上角「添加模型」。

<div align="center">
  <img src="images/wb-02.png" alt="Work Buddy「设置 → 模型」页——点击"添加模型"新增自定义模型" style="max-width:100%; width:720px;" />
</div>

2. 在弹出的「添加模型」表单中按如下填写：
   - **供应商**：选择「自定义」（仅支持 OpenAI 兼容协议 API）
   - **接口地址**：`https://token.bestloongai.com/v1`
   - **API Key**：粘贴步骤二复制的密钥
   - **模型名称**：填写你要使用的模型调用名（如 `gpt-5.6-sol`，可在 Bestloong「模型广场」查看）
   - **高级配置**：勾选「工具调用」「图片输入」「思考模式」「允许关闭思考」（按需）
3. 点击「保存」完成添加。

<div align="center">
  <img src="images/wb-03.png" alt="「添加模型」表单——供应商选"自定义"，填接口地址、API Key、模型名并勾选高级配置" style="max-width:100%; width:720px;" />
</div>

> 提示：接口地址需以 `/v1` 结尾；模型名必须与 Bestloong「模型广场」中的"调用名"完全一致，否则会报 404。

## 四、选择模型并测试
1. 返回对话界面，点击底部模型选择器。
2. 在"自定义模型"分组下选中刚添加的模型（如 `gpt-5.6-sol`）。
3. 发送一条测试消息（如"你好"），收到正常回复即表示接入成功。

<div align="center">
  <img src="images/wb-04.png" alt="对话界面模型选择器——选中刚添加的自定义模型" style="max-width:100%; width:720px;" />
</div>

## 五、常见问题
- **401 未授权**：密钥复制不完整或未生效，重新复制 `sk-` 开头密钥。
- **404 找不到模型**：接口地址漏写 `/v1`，或模型名与「模型广场」调用名不一致。
- **请求超时**：接口地址填写错误，或网络无法访问 `token.bestloongai.com`。
