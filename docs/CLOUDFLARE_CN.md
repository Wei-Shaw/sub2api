# Cloudflare CDN 接入说明

下面这套流程适用于把 `sub2api` 放到 Cloudflare 前面做代理加速，并保持 SSE 流式响应可用。

## 1. 域名接入 Cloudflare

1. 登录 [dash.cloudflare.com](https://dash.cloudflare.com)
2. 添加你的域名
3. 按 Cloudflare 提供的两个 NS 地址，去域名注册商处修改 NS
4. 等待 NS 生效，通常几分钟到十几分钟

## 2. 添加 DNS 记录

在 Cloudflare 控制台进入 `DNS`，添加一条 `A` 记录：

- 类型：`A`
- 名称：`sub2api.yourdomain.com` 或 `@`
- IPv4：你的香港 VPS 公网 IP
- 代理状态：橙色云朵，开启代理

说明：

- 橙色云朵表示走 Cloudflare 代理
- 灰色云朵只是 DNS 解析，不会走 Cloudflare 节点

## 3. 配置 SSL

在 `SSL/TLS` 里选择加密模式：

- VPS 没有证书：`Flexible`
- VPS 已有证书：`Full`

如果你已经在源站部署了可信证书，优先用 `Full` 或 `Full (strict)`。

## 4. 关闭响应缓冲

Sub2API 会返回流式响应，Cloudflare 默认可能做响应体缓冲。

在 Cloudflare 控制台进入：

`Rules` -> `Configuration Rules` -> 新建规则

设置：

- 匹配条件：你的域名
- Response Body Buffering：`None` / `OFF`

这一步很关键，否则 SSE 可能不会边生成边返回。

## 5. 源站侧建议

如果你是直接把 Cloudflare 指到源站，或者前面还有一层 Caddy / Nginx，建议同步处理真实 IP：

- `server.trusted_proxies`：加入你的反代 IP
- 如果是本机反代：加入 `127.0.0.1/32` 和 `::1/128`
- 如果是 Cloudflare 直连源站：加入 Cloudflare 官方 IP 段
- 如需 API Key 白名单 / 黑名单按真实访客 IP 生效，开启 `security.trust_forwarded_ip_for_api_key_acl=true`

这个项目已经优先读取 `CF-Connecting-IP`，所以一般不需要改代码。

## 6. 验证

```bash
curl -I https://sub2api.yourdomain.com
```

期望看到：

- `server: cloudflare`
- 返回正常的 `200` / `301` / `302`

如果你想确认 SSE 是否被正确透传，可以再用实际的流式接口做一次请求。

## 参考

- [Cloudflare DNS proxy status](https://developers.cloudflare.com/dns/proxy-status/use-cases/)
- [Cloudflare DNS records](https://developers.cloudflare.com/dns/manage-dns-records/how-to/create-dns-records/)
- [Cloudflare SSL/TLS encryption modes](https://developers.cloudflare.com/learning-paths/get-started/security/ssl-tls)
- [Cloudflare Response Body Buffering](https://developers.cloudflare.com/rules/configuration-rules/settings/)
