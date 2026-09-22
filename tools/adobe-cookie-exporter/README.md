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
      "credentials": {
        "cookie": "ims_sid=...; aux_sid=...; ...",
        "access_token": "eyJhbGciOiJ...（可选，约 24h，在 Firefly 成功生图后才会带上）",
        "arp_session_id": "eyJzaWQiOiI...（可选，在 Firefly 成功生图后才会带上）"
      },
      "concurrency": 10,
      "priority": 1
    }
  ]
}
```

`access_token` 来自 Firefly 提交 `generate-async` 时的 `Authorization: Bearer`（页面正在用的 IMS token）。未捕获时该字段省略；导入后若 JWT 仍未过期，后台会跳过第一次 cookie 换 token。token 大约 24 小时，**不能替代 cookie**——过期后仍靠 cookie 刷新。

`arp_session_id` 来自同一请求的 `x-arp-session-id` 头（Sherlock ARP token），**不是 cookie**。未捕获时该字段省略，导入仍然成功；收费号可以不填。

账号名在 Firefly 页面能读到邮箱或显示名时用 `adobe-{email}`，否则 `adobe-{timestamp}`。导入后分组仍需手工绑定。

也可以把同一份 JSON 贴进新建 / 重认证账号的 Adobe Cookie 输入框，后台会取出 `credentials.cookie`；若 JSON 里有 ARP 或 access_token，界面会自动填入对应输入框。

## 安装

1. 打开 `chrome://extensions` 或 `edge://extensions`
2. 开启右上角「开发者模式」
3. 点击「加载已解压的扩展程序」
4. 选择本仓库目录 `tools/adobe-cookie-exporter/`

升级后若扩展已经加载过：打开 `chrome://extensions`，在本扩展卡片上点「重新加载」。只保存仓库文件不够，Chrome 不会自动换上新脚本。Popup 顶部应显示 `Extension v1.4.0`；若仍是更旧版本，说明还没重载成功。

重载之后，已经打开的 Firefly 标签页也不必先关掉：1.4.0 会用 background `webRequest` 抓 `Authorization` 和 `x-arp-session-id`。但**重载之前**生过的图不会留下这些头，需要再成功生一次。

## 使用

1. 在浏览器登录 Adobe，并打开 `https://firefly.adobe.com/generate/image`
2. **在该页成功生一次图**（Sherlock 只在这次提交里带 ARP / IMS token；popup 会显示 Token / ARP captured 或 not captured）
3. 点击扩展图标
4. 选择导出范围：
   - `Adobe domains (recommended)`（推荐）
   - `Current site`
5. 点击 `Export Sub2API JSON`，保存 JSON 文件
6. 打开管理后台 → 账号 → 导入，上传该文件

未捕获 token / ARP 时仍可导出 cookie。缺 token 时后台会在首次使用时用 cookie 换；收费号可以不带 ARP。

## 为什么需要插件

Adobe 的关键鉴权 cookie 多为 **HttpOnly**，控制台 `document.cookie` 读不到。本扩展通过 `chrome.cookies` API 读取完整 cookie jar，包含 IMS 刷新所需的 `ims_sid` 等会话项。只从 `firefly.adobe.com` 复制 `document.cookie` 不够。IMS token 和 ARP 也读不到 cookie：扩展监听发往 `*.adobe.io` 的 `Authorization` / `x-arp-session-id`（并在页面里备份 hook `fetch` / XHR）。

## 无痕模式

扩展从当前活动标签页所属的 cookie store 导出。若在无痕窗口使用 Adobe：

1. 在扩展详情页开启「在无痕模式下启用」
2. 在无痕窗口打开 Firefly 并登录
3. 从该无痕标签页打开扩展并导出

## 来源

移植自 [GPT2Image-Pro](https://github.com/MeowFree/GPT2Image-Pro) 的 `tools/adobe-cookie-exporter`（其本身移植自 [adobe2api](https://github.com/leik1000/adobe2api) 的 `browser-cookie-exporter`），导出格式对齐 Sub2API 账号数据导入。
