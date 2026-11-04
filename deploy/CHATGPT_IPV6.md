# ChatGPT IPv6 页面开关

在 **系统设置 → 网关转发 → ChatGPT 仅使用 IPv6** 打开并保存。默认关闭，保存后当前实例对新请求立即生效；其他实例每 5 秒后台同步（数据库读取超时 2 秒），空闲实例也会更新，请求不等待数据库读取，不需要重启。数据库读取失败时保留上次策略并重试。值保存在数据库，重启时加载；正在执行的请求不被中断。普通连接池与 IPv6 连接池独立，切换不会复用另一地址族的已有连接。

页面保存值优先于 `SUB2API_CHATGPT_IPV6_ONLY`；该环境变量仅保留作为尚未设置页面值时的兼容默认值。无需用户编辑 Compose。

启用后，对 `chatgpt.com` 及其子域的直接连接强制 `tcp6`，不会回退 IPv4。配置代理时，应用解析目标域名的 AAAA 地址，再通过 HTTP/HTTPS CONNECT 或 SOCKS5/SOCKS5H 隧道向代理提交明确的 IPv6 目标地址；TLS SNI、证书校验与 HTTP Host 仍使用原域名。应用到代理的连接可以使用 IPv4 或 IPv6，要求代理到 ChatGPT 使用 IPv6。覆盖 OAuth 普通 HTTP/SSE Responses、WebSocket，以及 OAuth 生图、web-image、Native-Image 生图和编辑的连接入口。TLS 指纹、请求内容、模型和用量处理保持不变。共享 HTTP 客户端访问 ChatGPT 的辅助请求也采用同一规则。

限制与前提：

- **直连时，运行应用的网络命名空间必须有可用 IPv6。** 宿主机 IPv6 通不代表 Docker bridge 容器通。使用代理时，本地只需能连接代理；代理出口必须有可用 IPv6。本开关不自动修改 Docker、宿主机路由、防火墙或业务网络。
- 支持 HTTP、HTTPS、SOCKS5 和 SOCKS5H 代理及账号密码认证。HTTP/HTTPS 代理必须接受 IPv6 字面量 CONNECT，SOCKS5 必须接受 IPv6 地址类型。此模式下目标 AAAA 由应用本地 DNS 解析，而非交由代理解析域名；没有 AAAA 或代理拒绝 IPv6 时返回错误，不绕过代理、不回退 IPv4。多个 AAAA 地址会在代理建连超时范围内依次尝试。
- 仅匹配 ChatGPT 域名，不改变 `api.openai.com`、Claude、Gemini 或其他平台域名。自定义中转域名和外部图片 CDN 不在此域名策略内。
- IPv6 连接不可用会返回连接错误；无 IPv4 回退。
- 此开关不保证降低挑战频率或加速。前轮各三次对照未发现 IPv6 优势。
