# Sub2API 作为公网入口的三项目串联方案 A

- 时间：2026-06-17 22:33:55 +09:00
- 范围：`sub2api`、`CLIProxyAPI`、`yui.web/shop`
- 决策：采用方案 A，Sub2API 接管公网入口、用户 Key、计费和用量事实；CLIProxyAPI 退到内网账号池；yui.web/shop 退为展示、引导或跳转入口。

## 背景

当前链路是：

```text
朋友/用户 -> 公网映射/反代 -> CLIProxyAPI -> 本地 Codex/Claude/Gemini 账号
```

`CLIProxyAPI` 当前监听 `127.0.0.1:8317`，负责 OAuth 账号、协议兼容和多账号轮询。`yui.web/shop` 当前维护自己的用户、邀请码、API Key、订阅美元额度、用量导入和扣费，并通过修改 `CLIProxyAPI/config.yaml` 的 `api-keys` 发放入口 Key。`CLIProxyAPI` 又通过 internal status 和 usage event 回调查询/回传给 `yui.web`。

接入 Sub2API 后，如果继续保留 yui.web 与 CLIProxyAPI 的双向账务链路，会出现多事实源：Sub2API、yui.web、CLIProxyAPI 同时判断 Key 状态或用量，容易造成状态冲突、重复扣费和排障困难。

## 目标架构

方案 A 的目标链路：

```text
朋友/用户
  -> Sub2API 公网域名
  -> Sub2API 用户 API Key / 余额 / 订阅 / 用量
  -> Sub2API OpenAI-compatible upstream account
  -> CLIProxyAPI 127.0.0.1:8317
  -> 本地 Codex / Claude / Gemini 账号
```

职责边界：

- Sub2API 是唯一公网 API 入口。
- Sub2API 是唯一用户 API Key 发放方。
- Sub2API 是唯一计费、额度和用量事实源。
- CLIProxyAPI 只负责本地订阅账号池、OAuth token 刷新、协议转换和多账号轮询。
- yui.web/shop 第一阶段只保留个人站入口、说明、使用指南、跳转到 Sub2API 用户中心或支付页。

## 三个项目的新职责

### Sub2API

Sub2API 接管：

- 用户注册、登录、API Key 生成。
- 分组、套餐、订阅、余额、支付和用量记录。
- 公网 API 路由，如 `/v1/messages`、`/v1/chat/completions`、`/v1/responses`、`/v1beta/models`。
- 对外暴露给朋友的唯一 Base URL 和 Bearer Key。

Sub2API 需要配置一个 OpenAI-compatible 上游账号，`base_url` 指向 CLIProxyAPI 本地端口。若 Sub2API 直接在宿主机运行，可用 `http://127.0.0.1:8317`；若跑 Docker，需使用 `host.docker.internal:8317`、host 网络，或调整网络映射。

### CLIProxyAPI

CLIProxyAPI 保留：

- Codex / Claude / Gemini / Antigravity 等本地账号池。
- OAuth 登录、刷新和模型兼容。
- 对 Sub2API 提供一个本地 OpenAI-compatible 上游服务。

CLIProxyAPI 不再承担：

- 朋友用户的直接入口 Key 管理。
- 面向公网的 API 入口。
- yui.web 的 usage 回调事实源。

当前 `host: "127.0.0.1"` 的方向正确，应继续避免直接暴露管理 API 或入口 API 到公网。

### yui.web/shop

yui.web/shop 第一阶段改为轻量入口：

- 首页说明服务用途、套餐说明、使用方式。
- 登录/购买/查看用量入口跳转到 Sub2API。
- 保留个人站品牌页面和文档入口。

yui.web/shop 不再直接：

- 修改 `CLIProxyAPI/config.yaml` 的 `api-keys`。
- 维护新的 Shop API Key 发放。
- 消费 CLIProxyAPI usage event 并进行新账务扣费。
- 判定 API Key 是否可用。

历史 SQLite 数据可保留为归档或迁移参考，不作为新请求链路的实时事实源。

## 初始落地顺序

1. 部署 Sub2API 到本机或服务器。
2. 避开当前已被 nginx 占用的 `8080`，建议先用 `18080` 或配置 nginx 新域名反代到 Sub2API。
3. 在 Sub2API 初始化管理员账号、生成 Admin API Key，并确认管理后台可访问。
4. 在 Sub2API 中创建上游账号：
   - 平台优先使用 OpenAI 兼容路径。
   - `base_url` 指向 CLIProxyAPI 的本地服务。
   - `api_key` 使用 CLIProxyAPI 中允许给 Sub2API 使用的内部 Key。
5. 在 Sub2API 中创建分组、套餐或余额策略，限定模型为当前要提供的模型集合。
6. 创建测试用户和 Sub2API 用户 API Key。
7. 用 Sub2API Key 请求 `/v1/responses`、`/v1/chat/completions` 或当前目标客户端实际使用的接口。
8. 确认 Sub2API 记录用量和扣费后，再逐步停止 yui.web -> CLIProxyAPI 的新 Key 发放链路。
9. 修改 yui.web/shop 页面，把新用户引导到 Sub2API。

## 迁移边界

第一阶段不迁移历史用户和历史账单，只切新入口。原因：

- 当前 yui.web 已有真实 SQLite 账务与历史规则，直接迁移到 Sub2API 需要字段映射和余额校验。
- 先让新流量走 Sub2API，可以降低一次性迁移风险。
- 旧用户可保留旧入口一段时间，或手工在 Sub2API 创建用户和套餐后切换。

第二阶段再评估：

- 是否将 yui.web 老用户映射到 Sub2API 用户。
- 是否把 yui.web 现有套餐余额迁移为 Sub2API 余额或订阅。
- 是否通过 Sub2API Admin API 自动化创建用户、分配订阅、充值。

## 风险和约束

- 使用订阅账号对外分发 API 可能违反上游服务条款，需按 Sub2API 合规提示自行承担风险。
- Docker 内访问宿主机 `127.0.0.1` 会指向容器自身，不会指向 CLIProxyAPI；需要显式处理网络。
- Sub2API 和 yui.web 不应同时对同一个用户 Key 扣费。
- CLIProxyAPI 管理 API 不应打开远程访问。
- 不应把完整 API Key、内部 token、HMAC secret 写入文档或提交。

## 后续实施建议

下一步应先做最小可用验证：

1. 启动 Sub2API 到非冲突端口。
2. 用一个测试内部 Key 将 Sub2API 接到 CLIProxyAPI。
3. 创建一个测试用户和测试 Sub2API Key。
4. 跑一次真实请求，验证返回、用量、扣费、错误日志。
5. 再决定是否修改 yui.web/shop 页面。
