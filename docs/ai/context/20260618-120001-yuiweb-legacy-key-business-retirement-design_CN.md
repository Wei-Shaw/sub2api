# yui.web 旧邀请码与旧 API Key 发放业务中长期退役设计

## 背景

当前三项目边界已经调整为：

```text
yui.web/shop：官网展示、购买说明、跳转入口
Sub2API：唯一用户登录、API Key、套餐、额度、用量事实源
CLIProxyAPI：内网上游账号池和协议转换
```

公网验证已经完成：

- `https://aaccx.pw/shop` 继续由 yui.web 提供。
- `https://aaccx.pw/dashboard` 进入 Sub2API 控制台。
- `https://aaccx.pw/v1/*` 由 Sub2API 处理。
- yui.web `.env` 当前启用 `SHOP_LEGACY_KEY_ISSUANCE_DISABLED=true`。

因此 yui.web 旧业务中的“生成邀请码、导入 API Key 池、用户兑换邀请码并领取 API Key、写入订单”已经不再是主链路。旧测试仍大量覆盖这些成功路径，所以全量 `test/shop-flow.test.js` 会出现一批期待 `201`、实际返回 `410` 的失败。

本设计解决的问题不是“把 410 改回 201”，而是规范旧业务如何退役，避免误删历史数据能力，也避免旧代码继续发放新的 Key。

## 目标

1. 明确 yui.web 旧发 Key 业务的退役边界。
2. 保留用户历史查账、历史订单、历史用量核查能力。
3. 禁止 yui.web 继续产生新的用户 API Key、邀请码、订单权益事实。
4. 更新测试，让测试表达新架构，而不是继续要求旧架构成功。
5. 给出中期和长期删除代码的前置条件，避免拍脑袋删除。

## 非目标

- 不恢复 yui.web 旧邀请码兑换能力。
- 不让 yui.web 与 Sub2API 同时管理同一用户 Key 或额度。
- 不把未发放的 yui.web unused API Key 库存迁入 Sub2API。
- 不在 yui.web 中新增 Sub2API 的完整管理后台。
- 不删除已经迁移过来的用户、订阅和 API Key。

## 当前代码事实

yui.web 中存在迁移开关：

```text
SHOP_LEGACY_KEY_ISSUANCE_DISABLED=true
```

对应代码：

```text
/Users/wujianxiang/CodeSpace/yui.web/server.js
rejectLegacyKeyIssuanceWhenDisabled()
```

该函数在开关启用时返回：

```json
{
  "code": "SHOP_LEGACY_KEY_ISSUANCE_DISABLED",
  "message": "旧 Shop API key 发放已停止，请使用 Sub2API 用户中心。"
}
```

当前被锁死的写路径包括：

```text
POST /api/account/invites/redeem
POST /api/admin/invites
POST /api/admin/api-keys
POST /api/admin/session-invites
POST /api/admin/session-api-keys
POST /api/invites/redeem
```

这些接口返回 `410 Gone` 是合理的：它们曾经存在，但现在已经被产品下线。

## 退役原则

### 1. 写路径先锁死，读路径后清理

能创建新事实的旧接口必须先关：

- 新邀请码。
- 新 API Key 库存导入。
- 新 API Key 领取。
- 新订单权益写入。
- 新 yui.web 侧套餐扣费事实。

只读和审计能力不能急着删：

- 历史订单查询。
- 历史用户查询。
- 历史用量核查。
- 旧 API Key preview/hash 状态核查。
- 迁移校验脚本依赖的数据读取。

原因：写路径会制造双事实源，读路径还能帮忙查账和处理用户迁移售后。

### 2. 旧接口返回 410，不返回 404

`404` 表示“不存在”，容易让排障误判为路由或部署问题。

`410 Gone` 表示“功能已下线”，更适合旧业务退役。响应体必须稳定包含 `code=SHOP_LEGACY_KEY_ISSUANCE_DISABLED`，方便前端、测试、日志和人工排查。

### 3. Sub2API 是唯一新事实源

迁移完成后，新用户、新 Key、新套餐和新用量都只进入 Sub2API：

```text
用户登录：Sub2API
用户 API Key：Sub2API api_keys
订阅套餐：Sub2API user_subscriptions / groups
用量记录：Sub2API usage_logs
公网调用：aaccx.pw/v1/*
```

yui.web 不再参与 Key 可用性判断，不再给用户生成 Key，不再实时扣 API 用量。

### 4. 删除前必须有观测窗口

旧代码不应在切换当天立即物理删除。长期删除前至少满足：

- nginx 或应用日志中旧写接口连续 2-4 周无有效用户调用。
- Sub2API 已稳定处理新注册、新登录、新 Key、新用量。
- 已迁移用户的售后问题可通过 Sub2API 和保留的 yui.web 历史只读查询解决。
- 有可恢复备份：yui.web SQLite、Sub2API PostgreSQL、迁移脚本和迁移结果文档。

## 分阶段方案

## 阶段 1：短期，显式退役

时间窗口：当前到 1 周。

目标：不删核心代码，先让生产行为正确、可观测、可解释。

处理项：

- 保持 `SHOP_LEGACY_KEY_ISSUANCE_DISABLED=true`。
- 确认 6 个旧写接口统一返回 `410`。
- shop 页面只保留 Sub2API 入口，不再展示旧邀请码兑换入口。
- 旧入口页面如果仍可被访问，应显示“已迁移到 Sub2API”的提示，而不是继续引导用户兑换。
- 保留历史订单、账户、用量的只读接口。
- 文档中明确 yui.web 不再发 Key。

验收：

- `POST /api/invites/redeem` 返回 `410`。
- `POST /api/admin/api-keys` 返回 `410`。
- `POST /api/admin/session-api-keys` 返回 `410`。
- `GET /shop` 仍可访问并跳转 Sub2API。
- 旧用户可以使用迁移到 Sub2API 的 Key 请求 `https://aaccx.pw/v1/*`。

## 阶段 2：中期，测试和 UI 清理

时间窗口：1-2 周。

目标：让测试表达新架构，不再把已退役能力当成失败。

### 测试处理

把测试分成三类：

#### A. 删除或改写为 410 的测试

这类测试过去验证“旧发放成功”，现在应该验证“旧发放被拒绝”：

- 管理员生成邀请码。
- 管理员导入 API Key 池。
- session 管理员导入 API Key 池。
- 公开邀请码兑换。
- 登录态邀请码兑换。
- 邀请码大小写归一兑换。
- 没有 unused Key 时兑换失败。
- 重复导入 API Key。

改写后应断言：

```text
HTTP 410
body.code = SHOP_LEGACY_KEY_ISSUANCE_DISABLED
数据库没有新增 invite/order/api_key 写入
响应不包含完整 API Key
```

#### B. 保留的历史只读测试

这些能力暂时保留：

- 历史订单页面需要登录才能访问。
- Account 页面可展示历史订单和 API Key preview。
- 内部 API Key 状态接口按 hash 查询旧 Key 状态。
- 历史用量 summary 查询。
- 管理员只读统计和查账页面。

注意：如果内部状态接口仍服务于 CLIProxyAPI 或迁移核查，就不能删除。

#### C. 迁移后应移动到 Sub2API 的测试

这些测试不应该继续在 yui.web 中证明：

- 新用户创建 API Key。
- 新用户套餐生效。
- 新请求扣订阅额度。
- 新用量进入仪表盘。

这些测试应该转移到 Sub2API 的测试或端到端验收脚本中。

### UI 处理

yui.web 中应清理：

- 旧邀请码兑换入口。
- 旧 API Key 领取结果页入口。
- 旧管理员导入 Key 池入口。
- 文案中“yui.web 发 Key”的描述。

保留：

- shop 首页。
- 使用说明。
- Sub2API 跳转按钮。
- 旧订单只读说明。
- 必要的历史查账入口。

验收：

- `node --test --test-name-pattern 'Sub2API migration disables legacy invite and API key issuance endpoints' test/shop-flow.test.js` 通过。
- `node --test --test-name-pattern 'Shop 首页使用配置的 Sub2API 公网入口链接' test/shop-flow.test.js` 通过。
- 全量 shop-flow 不再因为旧发放成功路径失败。

## 阶段 3：长期，物理删除旧写代码

时间窗口：2-4 周后，满足观测条件再执行。

目标：删除 yui.web 中已经不可能再使用的写业务，降低维护成本。

可删除范围：

- 新邀请码生成逻辑。
- unused API Key 池导入逻辑。
- 邀请码兑换并发 Key 逻辑。
- 兑换时同步 CLIProxyAPI 配置的逻辑。
- 旧结果 token 绑定新订单的写逻辑。
- 与旧发放流程专用的前端页面和 JS。

谨慎保留范围：

- 历史订单表结构和读取逻辑。
- 历史用户表读取逻辑。
- API Key hash / preview 的只读读取逻辑。
- 已迁移核查脚本依赖的字段。
- 备份和迁移文档。

长期删除后，旧写接口可以有两种处理：

### 推荐：保留极薄 410 路由

即使删除内部业务，也保留 route stub：

```text
POST /api/invites/redeem -> 410
POST /api/admin/api-keys -> 410
```

优点：

- 旧客户端访问时得到清楚解释。
- 日志仍可统计旧访问量。
- 不会让 404 和 nginx 路由问题混在一起。

### 不推荐：直接删除路由变 404

只有在确认旧入口完全没有访问量、且不再需要兼容提示时才考虑。

## 数据处理策略

### 不删除历史数据

至少保留：

- `users`
- `orders`
- `api_keys` 中已使用记录的 preview/hash/ciphertext
- `account_subscriptions`
- `usage_events`
- `charge_records`
- `account_balance_transactions`

这些数据用于：

- 老用户售后。
- 对账。
- 迁移结果复核。
- 争议处理。

### unused API Key 库存

yui.web 中未发放的 unused API Key 不再给用户使用。

处理方式：

- 中期保留在 SQLite 中，只读归档。
- 长期可以导出到离线加密归档后从生产库删除。
- 不迁入 Sub2API，避免把旧库存变成新的事实源。

### 明文和密文安全

任何退役和导出脚本都必须遵守：

- 不打印完整 API Key。
- 日志只输出 preview/hash。
- 不在文档记录加密 secret。
- 导出文件如果包含密文和 nonce，也按敏感数据处理。

## 监控与日志

中期应增加或确认以下观测：

- 每个旧写接口的 `410` 访问次数。
- 来源 IP、User-Agent、Referer。
- 是否带登录态。
- 是否是真实用户点击旧入口，还是脚本误调用。

如果 2-4 周内仍有真实用户访问旧写接口，应先修 UI 或用户指引，不急着删 route。

## 风险与对应策略

### 风险：误删历史查账能力

策略：

- 先锁写，不删读。
- 删除前列出接口清单和表字段依赖。
- 保留 SQLite 备份和迁移文档。

### 风险：旧测试被大面积删除后失去覆盖

策略：

- 不简单删除测试。
- 将旧成功路径改为退役路径测试。
- 把新 Key / 套餐 / 用量测试迁移到 Sub2API。

### 风险：旧客户端拿到 404 后误判服务故障

策略：

- 长期也保留薄 410 route。
- 响应体保持稳定 code 和中文说明。

### 风险：yui.web 和 Sub2API 再次产生双事实源

策略：

- `SHOP_LEGACY_KEY_ISSUANCE_DISABLED=true` 作为生产默认。
- yui.web 不再暴露任何创建 Key 的 UI。
- 后续如果需要发 Key，只从 Sub2API 控制台创建。

## 推荐实施顺序

1. 新建中期清理计划文档，列出具体测试改写任务。
2. 先改测试，把旧成功路径改为 410 退役路径。
3. 清理 shop 页面和管理员页面中的旧入口。
4. 确认 yui.web 全量测试不再因为旧发放路径失败。
5. 加旧接口 410 访问日志或确认现有日志足够。
6. 观察 2-4 周。
7. 写长期删除计划，按 route、函数、前端文件、测试分批删除旧写代码。

## 验收标准

中期完成标准：

- shop 页面只有 Sub2API 入口，不再引导旧兑换。
- 旧写接口统一返回 `410`。
- yui.web 测试不再要求旧发放成功。
- 历史订单和历史查账仍可用。
- Sub2API 新用户、新 Key、新套餐、新用量链路通过。

长期完成标准：

- yui.web 中旧写业务主体代码删除。
- 旧写接口薄 410 route 保留。
- 历史只读能力保留。
- 生产日志连续观察无真实用户依赖旧写路径。
- 删除前后的备份、回滚说明和验证记录齐全。

## 结论

旧业务不应立即一刀切删除。正确方式是：

```text
短期：锁死写路径，明确返回 410
中期：清理 UI 和测试，把旧成功路径改为退役路径
长期：观察稳定后删除旧写代码，保留历史只读和薄 410 路由
```

这能避免 yui.web 和 Sub2API 形成双事实源，同时保留老用户售后和查账所需的历史能力。
