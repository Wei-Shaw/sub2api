# LESSONS

- 当 Codex/Responses 请求失败时，如果 `https://api.codemax.store/cdn-cgi/trace`、无鉴权 `/v1/models`、`/health` 这类不依赖模型上游的轻请求也随机出现 TLS handshake reset/timeout，优先排查 Cloudflare zone、DNS 解析、anycast 线路、WAF/安全规则、Tunnel/源站连接；不要先把问题归因到 Responses SSE/tool_calls 兼容层。

- 多节点部署 Sub2API 时，新增应用节点不能只复制数据库和 Redis 地址；`JWT_SECRET`、`TOTP_ENCRYPTION_KEY` 以及 OAuth 相关密钥也必须和主节点完全一致，否则会出现登录态漂移、2FA 不可读或 OAuth 行为不一致。

- 验证生产只读数据库账号时，不要用 `CREATE TEMP TABLE` 作为只读证明；只读事务仍可能允许临时表。应使用真实业务表的 no-op `UPDATE`/`INSERT` 权限探针，确认数据库拒绝写入权限。

- Cloudflare Tunnel 远端 origin 如果配置为 `http://localhost:8080`，新增 connector 用 Docker 容器部署时要加 `--network host`。否则容器内的 `localhost` 指向 cloudflared 自己，不是宿主机 Sub2API，会导致线上 502；回滚后用 host network 重新启动即可。
