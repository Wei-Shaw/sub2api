# TROUBLESHOOTING

> 单文件中英双语文档 / Single-file bilingual documentation (Chinese + English)

---

## 中文

### OpenAI OAuth

#### Failed to exchange OpenAI auth code

如果在添加 OpenAI 账号时出现这个错误，不一定是授权码本身有问题。

浏览器完成授权后，Sub2API 后端仍需要使用该 auth code 与 OpenAI 交换令牌。如果浏览器侧走了代理，但服务端没有走代理，或者服务端无法访问 OpenAI，就可能导致交换失败。

建议优先排查：

- 确认运行 Sub2API 的服务端机器可以访问 OpenAI。
- 必要时为服务端配置代理。关键路径是后端发起的交换请求，不只是浏览器侧能打开授权页面。
- 修改网络或代理配置后，重新生成授权链接再试。
- 浏览器与服务端不在同一地区通常不是根因，关键是服务端到 OpenAI 的网络链路可用。

---

## English

### OpenAI OAuth

#### Failed to exchange OpenAI auth code

If you see this error when adding an OpenAI account, the auth code itself may not be the problem.

After the browser completes authorization, the Sub2API backend still needs to exchange that auth code with OpenAI. If the browser uses a proxy but the server does not, or the server cannot reach OpenAI, the exchange can fail.

Recommended checks:

- Verify that the machine running Sub2API can reach OpenAI.
- Configure a server-side proxy if needed. The critical path is the backend exchange request, not just the browser-side authorization page.
- After changing network or proxy settings, generate a new authorization link and try again.
- The browser and server do not need to be in the same region. The key requirement is that the server-side network path to OpenAI works.
