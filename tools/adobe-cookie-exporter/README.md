# Adobe Cookie Exporter

Chrome / Edge 浏览器扩展，用于导出 Adobe / Firefly 登录 cookie。导出文件就是 Sub2API 管理后台「导入」能直接吃的 `sub2api-data` JSON。

```json
{
  "type": "sub2api-data",
  "version": 1,
  "exported_at": "2026-09-20T06:54:00.000Z",
  "proxies": [],
  "accounts": [
    {
      "name": "adobe-jane@example.com",
      "platform": "adobe",
      "type": "oauth",
      "credentials": { "cookie": "ims_sid=...; aux_sid=...; ..." },
      "concurrency": 10,
      "priority": 1
    }
  ]
}
```

账号名在 Firefly 页面能读到邮箱或显示名时用 `adobe-{email}`，否则 `adobe-{timestamp}`。导入后分组仍需手工绑定。

也可以把同一份 JSON 贴进新建 / 重认证账号的 Adobe Cookie 输入框，后台会取出 `credentials.cookie`。

## 安装

1. 打开 `chrome://extensions` 或 `edge://extensions`
2. 开启右上角「开发者模式」
3. 点击「加载已解压的扩展程序」
4. 选择本仓库目录 `tools/adobe-cookie-exporter/`

## 使用

1. 在浏览器登录 Adobe，并打开 `https://firefly.adobe.com/generate/image`
2. 点击扩展图标
3. 选择导出范围：
   - `Adobe domains (recommended)`（推荐）
   - `Current site`
4. 点击 `Export Sub2API JSON`，保存 JSON 文件
5. 打开管理后台 → 账号 → 导入，上传该文件

## 为什么需要插件

Adobe 的关键鉴权 cookie 多为 **HttpOnly**，控制台 `document.cookie` 读不到。本扩展通过 `chrome.cookies` API 读取完整 cookie jar，包含 IMS 刷新所需的 `ims_sid` 等会话项。只从 `firefly.adobe.com` 复制 `document.cookie` 不够。

## 无痕模式

扩展从当前活动标签页所属的 cookie store 导出。若在无痕窗口使用 Adobe：

1. 在扩展详情页开启「在无痕模式下启用」
2. 在无痕窗口打开 Firefly 并登录
3. 从该无痕标签页打开扩展并导出

## 来源

移植自 [GPT2Image-Pro](https://github.com/MeowFree/GPT2Image-Pro) 的 `tools/adobe-cookie-exporter`（其本身移植自 [adobe2api](https://github.com/leik1000/adobe2api) 的 `browser-cookie-exporter`），导出格式对齐 Sub2API 账号数据导入。
