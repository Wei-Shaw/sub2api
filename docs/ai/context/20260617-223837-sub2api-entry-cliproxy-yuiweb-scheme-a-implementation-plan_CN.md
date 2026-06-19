# Sub2API Entry Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Sub2API 成为唯一公网 API 入口，并通过内网 CLIProxyAPI 使用本地 Codex / Claude / Gemini 账号池，同时把 yui.web/shop 退为展示和跳转入口。

**Architecture:** 第一阶段只验证 Sub2API -> CLIProxyAPI -> 本地账号的最小链路，不迁移历史用户和历史账单。第二阶段改 yui.web/shop，停止新用户通过 yui.web 领取 CLIProxyAPI Key，页面改为引导到 Sub2API。

**Tech Stack:** Go / Gin / Ent / PostgreSQL / Redis / Docker Compose for Sub2API；Go for CLIProxyAPI；Node.js / Express / SQLite / static HTML for yui.web。

---

## 相关上下文

- 设计文档：`/Users/wujianxiang/CodeSpace/sub2api/docs/ai/context/20260617-223355-sub2api-entry-cliproxy-yuiweb-scheme-a_CN.md`
- Sub2API 根目录：`/Users/wujianxiang/CodeSpace/sub2api`
- CLIProxyAPI 根目录：`/Users/wujianxiang/CodeSpace/CLIProxyAPI`
- yui.web 根目录：`/Users/wujianxiang/CodeSpace/yui.web`
- 当前 CLIProxyAPI 监听：`127.0.0.1:8317`
- 当前 yui.web 监听：`*:4173`
- 当前 `8080` 被 nginx 占用，Sub2API 本机验证使用 `18080`

## 文件结构与职责

- Create: `/Users/wujianxiang/CodeSpace/sub2api/deploy/.env.scheme-a.local`
  - Sub2API 本机验证专用环境变量，不提交，不写入真实密钥到文档。
- Modify: `/Users/wujianxiang/CodeSpace/CLIProxyAPI/config.yaml`
  - 只新增一个 Sub2API 专用的上游内部 Key 到顶层 `api-keys`。
- Create: `/Users/wujianxiang/CodeSpace/CLIProxyAPI/backups/config-before-sub2api-upstream-<timestamp>.yaml`
  - 修改 CLIProxyAPI 配置前的备份。
- Modify: `/Users/wujianxiang/CodeSpace/yui.web/server.js`
  - 增加 `SHOP_LEGACY_KEY_ISSUANCE_DISABLED` 开关，阻止新的 yui.web 兑换 / 发 Key。
- Modify: `/Users/wujianxiang/CodeSpace/yui.web/shop/index.html`
  - Shop 首页改成 Sub2API 入口说明和跳转。
- Modify: `/Users/wujianxiang/CodeSpace/yui.web/shop/guide/index.html`
  - 使用指南改为 Sub2API Base URL 和 Sub2API Key 的使用方式。
- Modify: `/Users/wujianxiang/CodeSpace/yui.web/.env.example`
  - 增加 `SUB2API_PUBLIC_URL` 和 `SHOP_LEGACY_KEY_ISSUANCE_DISABLED` 示例。
- Test: `/Users/wujianxiang/CodeSpace/yui.web/test/shop-flow.test.js`
  - 验证禁用开关开启后，新兑换和新 Key 发放接口被阻止。

## Task 1: 运行前状态快照

**Files:**
- Read: `/Users/wujianxiang/CodeSpace/CLIProxyAPI/config.yaml`
- Read: `/Users/wujianxiang/CodeSpace/yui.web/.env`
- Read: `/Users/wujianxiang/CodeSpace/sub2api/deploy/docker-compose.local.yml`

- [ ] **Step 1: 记录三个服务当前监听端口**

Run:

```bash
lsof -nP -iTCP:8317 -sTCP:LISTEN || true
lsof -nP -iTCP:4173 -sTCP:LISTEN || true
lsof -nP -iTCP:18080 -sTCP:LISTEN || true
```

Expected:

```text
8317 由 cli-proxy-api 监听在 127.0.0.1
4173 由 node 监听
18080 没有进程监听
```

- [ ] **Step 2: 备份 CLIProxyAPI 配置**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/CLIProxyAPI
ts="$(date '+%Y%m%d-%H%M%S')"
mkdir -p backups
cp config.yaml "backups/config-before-sub2api-upstream-${ts}.yaml"
chmod 600 "backups/config-before-sub2api-upstream-${ts}.yaml"
ls -l "backups/config-before-sub2api-upstream-${ts}.yaml"
```

Expected:

```text
备份文件存在，权限不高于 600
```

- [ ] **Step 3: 记录 dirty worktree**

Run:

```bash
git -C /Users/wujianxiang/CodeSpace/sub2api status --short --untracked-files=all
git -C /Users/wujianxiang/CodeSpace/CLIProxyAPI status --short --untracked-files=all
git -C /Users/wujianxiang/CodeSpace/yui.web status --short --untracked-files=all
```

Expected:

```text
输出被保存到实施记录；后续不回滚用户已有改动
```

## Task 2: 准备 Sub2API 本机验证环境

**Files:**
- Create: `/Users/wujianxiang/CodeSpace/sub2api/deploy/.env.scheme-a.local`
- Read: `/Users/wujianxiang/CodeSpace/sub2api/deploy/docker-compose.local.yml`

- [ ] **Step 1: 生成本机验证环境变量文件**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/sub2api/deploy
umask 077
cat > .env.scheme-a.local <<EOF
BIND_HOST=127.0.0.1
SERVER_PORT=18080
SERVER_MODE=release
RUN_MODE=standard
POSTGRES_USER=sub2api
POSTGRES_DB=sub2api
POSTGRES_PASSWORD=$(openssl rand -hex 24)
REDIS_PASSWORD=
ADMIN_EMAIL=admin@sub2api.local
ADMIN_PASSWORD=$(openssl rand -base64 24 | tr -d '\n')
JWT_SECRET=$(openssl rand -hex 32)
TOTP_ENCRYPTION_KEY=$(openssl rand -hex 32)
TZ=Asia/Shanghai
SECURITY_URL_ALLOWLIST_ENABLED=false
SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=true
SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=true
GATEWAY_OPENAI_RESPONSE_HEADER_TIMEOUT=0
EOF
chmod 600 .env.scheme-a.local
```

Expected:

```text
deploy/.env.scheme-a.local 存在，且没有提交到 git
```

- [ ] **Step 2: 创建 Docker 本地数据目录**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/sub2api/deploy
mkdir -p data postgres_data redis_data
```

Expected:

```text
data、postgres_data、redis_data 三个目录存在
```

- [ ] **Step 3: 启动 Sub2API**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/sub2api/deploy
docker compose --env-file .env.scheme-a.local -f docker-compose.local.yml up -d
```

Expected:

```text
sub2api、sub2api-postgres、sub2api-redis 三个容器启动成功
```

- [ ] **Step 4: 验证健康检查**

Run:

```bash
curl -fsS http://127.0.0.1:18080/health
```

Expected:

```json
{"status":"ok"}
```

- [ ] **Step 5: 保存管理员临时密码**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/sub2api/deploy
grep '^ADMIN_PASSWORD=' .env.scheme-a.local
```

Expected:

```text
只在本机终端查看，不写入文档，不提交
```

## Task 3: 给 CLIProxyAPI 增加 Sub2API 专用内部 Key

**Files:**
- Modify: `/Users/wujianxiang/CodeSpace/CLIProxyAPI/config.yaml`
- Create: `/Users/wujianxiang/CodeSpace/CLIProxyAPI/backups/config-before-sub2api-upstream-<timestamp>.yaml`

- [ ] **Step 1: 生成 Sub2API 上游内部 Key**

Run:

```bash
SUB2API_UPSTREAM_KEY="sk-sub2api-upstream-$(openssl rand -hex 24)"
printf '%s\n' "$SUB2API_UPSTREAM_KEY"
```

Expected:

```text
终端输出一个以 sk-sub2api-upstream- 开头的内部 Key；只保存到本机密码管理器或本机临时 shell
```

- [ ] **Step 2: 把内部 Key 加入 CLIProxyAPI 顶层 api-keys**

Edit `/Users/wujianxiang/CodeSpace/CLIProxyAPI/config.yaml`:

```yaml
api-keys:
  - "原有 key 保持不变"
  - "sk-sub2api-upstream-<本机生成值>"
```

Rules:

- 保留所有已有 `api-keys`。
- 只新增一个 Sub2API 内部 Key。
- 不把内部 Key 写入任何 Markdown 文档。

- [ ] **Step 3: 重启 CLIProxyAPI**

Run:

```bash
pkill -f cli-proxy-api || true
cd /Users/wujianxiang/CodeSpace/CLIProxyAPI
nohup ./cli-proxy-api --config config.yaml > logs/sub2api-upstream-restart.log 2>&1 &
sleep 2
lsof -nP -iTCP:8317 -sTCP:LISTEN
```

Expected:

```text
cli-proxy-api 重新监听 127.0.0.1:8317
```

- [ ] **Step 4: 用内部 Key 验证 CLIProxyAPI 可用**

Run:

```bash
curl -fsS http://127.0.0.1:8317/v1/models \
  -H "Authorization: Bearer ${SUB2API_UPSTREAM_KEY}" \
  | head -c 500
```

Expected:

```text
返回模型列表 JSON，且不是 401 / 403
```

## Task 4: 在 Sub2API 中创建上游账号、分组和套餐

**Files:**
- Configure via Sub2API Admin UI at `http://127.0.0.1:18080`
- Reference: `/Users/wujianxiang/CodeSpace/sub2api/backend/internal/server/routes/admin.go`
- Reference: `/Users/wujianxiang/CodeSpace/sub2api/backend/internal/handler/admin/account_handler.go`

- [ ] **Step 1: 登录 Sub2API 管理后台**

Open:

```text
http://127.0.0.1:18080
```

Use:

```text
Email: admin@sub2api.local
Password: deploy/.env.scheme-a.local 中的 ADMIN_PASSWORD
```

Expected:

```text
可以进入 Sub2API 管理后台
```

- [ ] **Step 2: 创建 OpenAI 平台分组**

Admin UI fields:

```text
Name: codex-pool
Platform: openai
Subscription Type: subscription
Daily Limit USD: 19
Weekly Limit USD: 0
Monthly Limit USD: 0
Default Validity Days: 30
Status: active
Allow Messages Dispatch: enabled
Default Mapped Model: gpt-5.4
```

Expected:

```text
分组 codex-pool 创建成功，后续 API Key 可绑定到该分组
```

- [ ] **Step 3: 创建 Sub2API 上游账号**

Admin UI fields:

```text
Name: cliproxy-local-openai
Platform: openai
Type: apikey
Concurrency: 3
Priority: 50
Group: codex-pool
Credentials:
  api_key: 使用 Task 3 生成的 SUB2API_UPSTREAM_KEY
  base_url: http://host.docker.internal:8317/v1
```

Expected:

```text
账号创建成功，账号测试能连通 CLIProxyAPI
```

- [ ] **Step 4: 创建在售套餐**

Admin UI fields:

```text
Name: 29 元订阅池
Group: codex-pool
Price: 29
Validity Days: 30
For Sale: enabled
Description: 每日 19 USD 用量额度
```

Expected:

```text
套餐创建成功，用户可购买或管理员可分配订阅
```

## Task 5: 创建测试用户和测试 Sub2API Key

**Files:**
- Configure via Sub2API Admin UI and User UI
- Reference: `/Users/wujianxiang/CodeSpace/sub2api/backend/internal/handler/api_key_handler.go`

- [ ] **Step 1: 创建测试用户**

Admin UI fields:

```text
Email: sub2api-test-local@example.com
Password: 使用本机生成的临时强密码
Username: sub2api-test-local
Balance: 0
Allowed Groups: codex-pool
Status: active
```

Expected:

```text
测试用户创建成功
```

- [ ] **Step 2: 给测试用户分配订阅**

Admin UI fields:

```text
User: sub2api-test-local@example.com
Group: codex-pool
Validity Days: 30
Notes: scheme-a local chain validation
```

Expected:

```text
测试用户拥有 codex-pool 的 active subscription
```

- [ ] **Step 3: 登录测试用户并创建 API Key**

User UI fields:

```text
Name: scheme-a-local-test
Group: codex-pool
Quota: 1
Rate Limit 5h: 1
Rate Limit 1d: 1
Rate Limit 7d: 1
Expires In Days: 1
```

Expected:

```text
生成一个 Sub2API 用户 API Key；只在本机终端保存为 SUB2API_USER_KEY
```

## Task 6: 验证 Sub2API -> CLIProxyAPI -> 本地账号链路

**Files:**
- Read: `/Users/wujianxiang/CodeSpace/sub2api/backend/internal/server/routes/gateway.go`
- Read: `/Users/wujianxiang/CodeSpace/CLIProxyAPI/logs/sub2api-upstream-restart.log`

- [ ] **Step 1: 验证 Sub2API 模型列表**

Run:

```bash
curl -fsS http://127.0.0.1:18080/v1/models \
  -H "Authorization: Bearer ${SUB2API_USER_KEY}" \
  | head -c 1000
```

Expected:

```text
返回模型列表 JSON，且不是 401 / 403 / 404
```

- [ ] **Step 2: 验证 Responses API**

Run:

```bash
curl -fsS http://127.0.0.1:18080/v1/responses \
  -H "Authorization: Bearer ${SUB2API_USER_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.4","input":"只回复 pong","stream":false}'
```

Expected:

```text
返回成功响应，内容中包含 pong 或上游模型的简短文本回答
```

- [ ] **Step 3: 验证 Chat Completions API**

Run:

```bash
curl -fsS http://127.0.0.1:18080/v1/chat/completions \
  -H "Authorization: Bearer ${SUB2API_USER_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.4","messages":[{"role":"user","content":"只回复 pong"}],"stream":false}'
```

Expected:

```text
返回成功响应，choices 中有 assistant 消息
```

- [ ] **Step 4: 验证错误 Key 被拒绝**

Run:

```bash
curl -sS -o /tmp/sub2api-invalid-key.json -w '%{http_code}\n' \
  http://127.0.0.1:18080/v1/models \
  -H "Authorization: Bearer sk-invalid"
cat /tmp/sub2api-invalid-key.json
```

Expected:

```text
HTTP 状态码是 401 或 403，响应为认证失败
```

- [ ] **Step 5: 验证 Sub2API 用量记录**

Admin UI:

```text
进入 Usage / Usage Records
筛选用户 sub2api-test-local@example.com
检查最近请求的 model、api key、account、cost、endpoint
```

Expected:

```text
能看到 Task 6 的成功请求记录和成本记录
```

## Task 7: 停止 yui.web 新 Key 发放路径

**Files:**
- Modify: `/Users/wujianxiang/CodeSpace/yui.web/server.js`
- Modify: `/Users/wujianxiang/CodeSpace/yui.web/.env.example`
- Test: `/Users/wujianxiang/CodeSpace/yui.web/test/shop-flow.test.js`

- [ ] **Step 1: 写失败测试**

Add to `/Users/wujianxiang/CodeSpace/yui.web/test/shop-flow.test.js`:

```js
test('Sub2API migration disables legacy invite and API key issuance endpoints', async () => {
    await withServer(async ({ baseUrl, db }) => {
        seedAdminUserForTest(db);
        const adminLogin = await jsonFetch(`${baseUrl}/api/auth/login`, {
            method: 'POST',
            body: JSON.stringify({ phone: '15951875192', password: 'Abcdefg1' })
        });
        assert.equal(adminLogin.response.status, 200);
        const adminCookie = adminLogin.response.headers.get('set-cookie') || '';
        const accountCookie = await registerUserAndGetCookie(baseUrl, '13800138777');

        const cases = [
            {
                name: 'account invite redeem',
                path: '/api/account/invites/redeem',
                headers: { cookie: accountCookie },
                body: { code: 'YUI-111111-222222' }
            },
            {
                name: 'token invite create',
                path: '/api/admin/invites',
                headers: { 'x-admin-token': 'test-token' },
                body: { count: 1 }
            },
            {
                name: 'token api key import',
                path: '/api/admin/api-keys',
                headers: { 'x-admin-token': 'test-token' },
                body: { apiKeys: ['sk-disabled-token-import'] }
            },
            {
                name: 'session invite create',
                path: '/api/admin/session-invites',
                headers: { cookie: adminCookie },
                body: { count: 1 }
            },
            {
                name: 'session api key import',
                path: '/api/admin/session-api-keys',
                headers: { cookie: adminCookie },
                body: { apiKeysText: 'sk-disabled-session-import' }
            },
            {
                name: 'legacy public invite redeem',
                path: '/api/invites/redeem',
                headers: {},
                body: { phone: '13800138778', code: 'YUI-111111-222222' }
            }
        ];

        for (const item of cases) {
            const result = await jsonFetch(`${baseUrl}${item.path}`, {
                method: 'POST',
                headers: item.headers,
                body: JSON.stringify(item.body)
            });
            assert.equal(result.response.status, 410, item.name);
            assert.equal(result.body.code, 'SHOP_LEGACY_KEY_ISSUANCE_DISABLED', item.name);
        }
    }, {
        legacyKeyIssuanceDisabled: true
    });
});
```

Expected:

```text
当前测试失败，因为 createShopApp 还没有读取 legacyKeyIssuanceDisabled 选项
```

- [ ] **Step 2: 运行失败测试**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/yui.web
npm test -- --test-name-pattern 'Sub2API migration disables legacy invite and API key issuance endpoints'
```

Expected:

```text
FAIL，响应不是 410 或测试辅助函数需要按现有 test harness 调整
```

- [ ] **Step 3: 增加禁用开关**

Modify `/Users/wujianxiang/CodeSpace/yui.web/server.js` near server option parsing:

```js
const legacyKeyIssuanceDisabled = String(
    options.legacyKeyIssuanceDisabled ?? process.env.SHOP_LEGACY_KEY_ISSUANCE_DISABLED ?? ''
).trim().toLowerCase() === 'true';
```

Add helper inside `createApp` before route registration:

```js
function rejectLegacyKeyIssuanceWhenDisabled(req, res, next) {
    if (!legacyKeyIssuanceDisabled) {
        return next();
    }
    return res.status(410).json({
        code: 'SHOP_LEGACY_KEY_ISSUANCE_DISABLED',
        message: '旧 Shop API key 发放已停止，请使用 Sub2API 用户中心。'
    });
}
```

Apply the middleware to these routes:

```js
app.post('/api/account/invites/redeem', limitRedeemApi, requireAccount, rejectLegacyKeyIssuanceWhenDisabled, requireSameOrigin, requireAccountCsrf, (req, res) => {
```

```js
app.post('/api/admin/invites', limitAdminApi, requireAdminToken, rejectLegacyKeyIssuanceWhenDisabled, (req, res) => {
```

```js
app.post('/api/admin/api-keys', limitAdminApi, requireAdminToken, rejectLegacyKeyIssuanceWhenDisabled, (req, res) => {
```

```js
app.post('/api/admin/session-invites', limitAdminApi, requireSameOrigin, requireAdminAccount, rejectLegacyKeyIssuanceWhenDisabled, requireAccountCsrf, (req, res) => {
```

```js
app.post('/api/admin/session-api-keys', limitAdminApi, requireSameOrigin, requireAdminAccount, rejectLegacyKeyIssuanceWhenDisabled, requireAccountCsrf, (req, res) => {
```

```js
app.post('/api/invites/redeem', limitRedeemApi, rejectLegacyKeyIssuanceWhenDisabled, (req, res) => {
```

- [ ] **Step 4: 更新 yui.web 环境变量示例**

Add to `/Users/wujianxiang/CodeSpace/yui.web/.env.example`:

```dotenv
SUB2API_PUBLIC_URL=http://localhost:18080
SHOP_LEGACY_KEY_ISSUANCE_DISABLED=false
```

- [ ] **Step 5: 运行 yui.web 测试**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/yui.web
npm test
```

Expected:

```text
所有 node --test 测试通过
```

- [ ] **Step 6: 提交 yui.web 后端切换开关**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/yui.web
git diff -- server.js .env.example test/shop-flow.test.js
git add server.js .env.example test/shop-flow.test.js
git commit -m "feat(shop): disable legacy key issuance for sub2api migration"
```

Expected:

```text
提交成功，diff 只包含禁用开关和测试
```

## Task 8: 把 yui.web/shop 改为 Sub2API 入口页

**Files:**
- Modify: `/Users/wujianxiang/CodeSpace/yui.web/shop/index.html`
- Modify: `/Users/wujianxiang/CodeSpace/yui.web/shop/guide/index.html`
- Modify: `/Users/wujianxiang/CodeSpace/yui.web/styles/site.css` after CSS build
- Test: `/Users/wujianxiang/CodeSpace/yui.web/test/shop-frontend.test.js`

- [ ] **Step 1: 写前端测试**

Add to `/Users/wujianxiang/CodeSpace/yui.web/test/shop-frontend.test.js`:

```js
test('shop home points new users to Sub2API instead of legacy redeem', async () => {
  const html = await fs.promises.readFile(path.join(rootDir, 'shop', 'index.html'), 'utf8');
  assert.match(html, /Sub2API/);
  assert.match(html, /\/shop\/guide\//);
  assert.doesNotMatch(html, /href="\/shop\/redeem\/"/);
});
```

Expected:

```text
当前测试失败，因为首页仍然包含旧兑换入口
```

- [ ] **Step 2: 运行失败测试**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/yui.web
npm test -- --test-name-pattern 'shop home points new users to Sub2API'
```

Expected:

```text
FAIL，旧首页还没有改成 Sub2API 入口
```

- [ ] **Step 3: 修改 Shop 首页主文案和按钮**

In `/Users/wujianxiang/CodeSpace/yui.web/shop/index.html`, replace the hero copy with:

```html
<p class="text-xs uppercase tracking-[0.28em] text-text-muted dark:text-dark-text-muted">Sub2API gateway</p>
<h1 class="mt-5 font-display text-5xl md:text-7xl leading-[1.05] text-primary dark:text-dark-text">Codex<br/><span class="italic text-text-muted dark:text-dark-text-muted">统一入口</span></h1>
<p class="mt-6 max-w-xl text-lg font-light leading-relaxed text-text-muted dark:text-dark-text-muted">新的 API key、套餐、余额和用量记录都在 Sub2API 中管理。这里保留使用说明和入口说明，正式调用请使用 Sub2API 分配给你的 Base URL 和 API key。</p>
<div class="mt-10 grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-[520px]">
    <a class="btn-primary h-14 px-4 text-center justify-center whitespace-nowrap" href="/shop/guide/">查看使用方法</a>
    <a class="btn-secondary h-14 px-4 text-center justify-center whitespace-nowrap dark:bg-dark-card dark:border-dark-border dark:text-dark-text" href="http://localhost:18080" data-sub2api-link>打开 Sub2API</a>
</div>
```

Replace the three right-side cards with:

```html
<div class="border border-border-subtle dark:border-dark-border rounded-lg bg-white dark:bg-dark-card p-5">
    <div class="flex items-start gap-4">
        <span class="material-symbols-outlined text-3xl text-primary dark:text-dark-text">key</span>
        <div>
            <h2 class="font-display text-2xl text-primary dark:text-dark-text">Sub2API 发 Key</h2>
            <p class="mt-2 text-sm leading-relaxed text-text-muted dark:text-dark-text-muted">朋友使用 Sub2API 中生成的 API key，不再直接使用 CLIProxyAPI key。</p>
        </div>
    </div>
</div>
<div class="border border-border-subtle dark:border-dark-border rounded-lg bg-white dark:bg-dark-card p-5">
    <div class="flex items-start gap-4">
        <span class="material-symbols-outlined text-3xl text-primary dark:text-dark-text">route</span>
        <div>
            <h2 class="font-display text-2xl text-primary dark:text-dark-text">本地账号池</h2>
            <p class="mt-2 text-sm leading-relaxed text-text-muted dark:text-dark-text-muted">Sub2API 会把请求转发到本机 CLIProxyAPI，再由 CLIProxyAPI 使用本地账号池处理。</p>
        </div>
    </div>
</div>
<div class="border border-border-subtle dark:border-dark-border rounded-lg bg-white dark:bg-dark-card p-5">
    <div class="flex items-start gap-4">
        <span class="material-symbols-outlined text-3xl text-primary dark:text-dark-text">monitoring</span>
        <div>
            <h2 class="font-display text-2xl text-primary dark:text-dark-text">用量归 Sub2API</h2>
            <p class="mt-2 text-sm leading-relaxed text-text-muted dark:text-dark-text-muted">新入口的余额、订阅和用量记录以 Sub2API 为准。</p>
        </div>
    </div>
</div>
```

- [ ] **Step 4: 修改使用指南**

In `/Users/wujianxiang/CodeSpace/yui.web/shop/guide/index.html`, ensure the page includes this command block:

```html
<pre><code>export OPENAI_BASE_URL="https://你的-sub2api-域名/v1"
export OPENAI_API_KEY="Sub2API 分配给你的 API key"</code></pre>
```

Ensure the guide states:

```text
不要使用 CLIProxyAPI 的内部 key；朋友只使用 Sub2API 用户 key。
```

- [ ] **Step 5: 构建 CSS**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/yui.web
npm run build:css
```

Expected:

```text
styles/site.css 更新成功
```

- [ ] **Step 6: 运行前端测试**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/yui.web
npm test
```

Expected:

```text
所有测试通过
```

- [ ] **Step 7: 提交 yui.web 页面切换**

Run:

```bash
cd /Users/wujianxiang/CodeSpace/yui.web
git diff -- shop/index.html shop/guide/index.html styles/site.css test/shop-frontend.test.js
git add shop/index.html shop/guide/index.html styles/site.css test/shop-frontend.test.js
git commit -m "feat(shop): point users to sub2api gateway"
```

Expected:

```text
提交成功，Shop 首页不再引导新用户兑换 legacy API key
```

## Task 9: 配置公网入口

**Files:**
- Read or modify local nginx config after locating it with `nginx -T`
- No repository file is modified unless nginx config lives in a tracked project file

- [ ] **Step 1: 设置本次公网变量**

Run:

```bash
: "${SUB2API_PUBLIC_URL:?必须设置实际 Sub2API 公网地址，格式为 https://域名}"
SUB2API_PUBLIC_HOST="$(node -e 'console.log(new URL(process.env.SUB2API_PUBLIC_URL).host)')"
printf 'SUB2API_PUBLIC_URL=%s\nSUB2API_PUBLIC_HOST=%s\n' "$SUB2API_PUBLIC_URL" "$SUB2API_PUBLIC_HOST"
```

Expected:

```text
输出实际公网地址和域名；不使用示例域名作为执行值
```

- [ ] **Step 2: 找到 nginx 配置**

Run:

```bash
nginx -T 2>/tmp/nginx-dump.err | sed -n '1,240p'
cat /tmp/nginx-dump.err
```

Expected:

```text
能看到当前 8080 server block 或 include 路径
```

- [ ] **Step 3: 增加 Sub2API 反代 server**

Run:

```bash
: "${NGINX_SUB2API_CONF:?必须设置 nginx include 文件路径，例如 /opt/homebrew/etc/nginx/servers/sub2api.conf}"
cat > /tmp/sub2api-nginx-server.conf <<NGINX
server {
    listen 80;
    server_name ${SUB2API_PUBLIC_HOST};

    underscores_in_headers on;

    location / {
        proxy_pass http://127.0.0.1:18080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_buffering off;
        proxy_read_timeout 900s;
    }
}
NGINX
sudo install -m 0644 /tmp/sub2api-nginx-server.conf "$NGINX_SUB2API_CONF"
sudo sed -n '1,120p' "$NGINX_SUB2API_CONF"
```

Expected:

```text
server_name 是 SUB2API_PUBLIC_HOST 的真实值，proxy_pass 指向 127.0.0.1:18080
```

- [ ] **Step 4: 验证 nginx 配置并重载**

Run:

```bash
sudo nginx -t
sudo nginx -s reload
```

Expected:

```text
syntax is ok
test is successful
```

- [ ] **Step 5: 验证公网健康检查**

Run:

```bash
curl -fsS "${SUB2API_PUBLIC_URL%/}/health"
```

Expected:

```json
{"status":"ok"}
```

## Task 10: 切换运行策略并记录验收

**Files:**
- Create: `/Users/wujianxiang/CodeSpace/sub2api/docs/ai/context/YYYYMMDD-HHMMSS-sub2api-entry-chain-acceptance_CN.md`
- Update: `/Users/wujianxiang/CodeSpace/sub2api/AGENTS.md`
- Update: `/Users/wujianxiang/CodeSpace/yui.web/docs/ai/context/YYYYMMDD-HHMMSS-shop-sub2api-entry-switch_CN.md`

- [ ] **Step 1: 开启 yui.web legacy 发 Key 禁用开关**

Run:

```bash
: "${SUB2API_PUBLIC_URL:?必须沿用 Task 9 的实际 Sub2API 公网地址}"
cd /Users/wujianxiang/CodeSpace/yui.web
touch .env
if grep -q '^SHOP_LEGACY_KEY_ISSUANCE_DISABLED=' .env; then
  perl -0pi -e 's/^SHOP_LEGACY_KEY_ISSUANCE_DISABLED=.*$/SHOP_LEGACY_KEY_ISSUANCE_DISABLED=true/m' .env
else
  printf '\nSHOP_LEGACY_KEY_ISSUANCE_DISABLED=true\n' >> .env
fi
if grep -q '^SUB2API_PUBLIC_URL=' .env; then
  SUB2API_PUBLIC_URL="$SUB2API_PUBLIC_URL" perl -0pi -e 's#^SUB2API_PUBLIC_URL=.*$#"SUB2API_PUBLIC_URL=$ENV{SUB2API_PUBLIC_URL}"#me' .env
else
  printf 'SUB2API_PUBLIC_URL=%s\n' "$SUB2API_PUBLIC_URL" >> .env
fi
grep -E '^(SHOP_LEGACY_KEY_ISSUANCE_DISABLED|SUB2API_PUBLIC_URL)=' .env
```

Restart yui.web using the existing local service method.

Expected:

```text
/api/account/invites/redeem 返回 410，/shop/ 首页仍可访问
```

- [ ] **Step 2: 运行端到端验收**

Run:

```bash
curl -fsS "${SUB2API_PUBLIC_URL%/}/v1/models" \
  -H "Authorization: Bearer ${SUB2API_USER_KEY}" \
  | head -c 1000

curl -fsS "${SUB2API_PUBLIC_URL%/}/v1/responses" \
  -H "Authorization: Bearer ${SUB2API_USER_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.4","input":"只回复 pong","stream":false}'
```

Expected:

```text
公网域名可返回模型列表和 Responses 成功响应
```

- [ ] **Step 3: 写验收文档**

Create `/Users/wujianxiang/CodeSpace/sub2api/docs/ai/context/YYYYMMDD-HHMMSS-sub2api-entry-chain-acceptance_CN.md` with:

```markdown
# Sub2API 入口链路验收记录

- 时间：
- Sub2API 访问地址：
- CLIProxyAPI 上游地址：127.0.0.1:8317
- yui.web legacy 发 Key：已禁用
- 验收命令：
  - GET /health
  - GET /v1/models
  - POST /v1/responses
- 结果：
  - 健康检查：
  - 模型列表：
  - Responses：
- 用量记录：
  - Sub2API Usage 页面能看到测试请求：
- 风险：
  - 不记录完整 API Key 或内部 token。
```

- [ ] **Step 4: 更新 AGENTS 记忆**

Append to `/Users/wujianxiang/CodeSpace/sub2api/AGENTS.md`:

```markdown
## Sub2API 入口链路验收

- YYYY-MM-DD：Sub2API 已作为公网入口完成最小链路验收：用户 -> Sub2API -> CLIProxyAPI -> 本地账号池。
- 验收记录见 `docs/ai/context/YYYYMMDD-HHMMSS-sub2api-entry-chain-acceptance_CN.md`。
- yui.web/shop 新 Key 发放已禁用，新用户入口改为 Sub2API。
```

- [ ] **Step 5: 最终检查**

Run:

```bash
git -C /Users/wujianxiang/CodeSpace/sub2api status --short --untracked-files=all
git -C /Users/wujianxiang/CodeSpace/CLIProxyAPI status --short --untracked-files=all
git -C /Users/wujianxiang/CodeSpace/yui.web status --short --untracked-files=all
```

Expected:

```text
只包含本计划范围内的配置、文档和 yui.web 页面 / 测试改动
```

## Rollback Plan

- Sub2API 入口失败：停止 Sub2API 容器，保留 CLIProxyAPI 和 yui.web 原运行状态。

```bash
cd /Users/wujianxiang/CodeSpace/sub2api/deploy
docker compose --env-file .env.scheme-a.local -f docker-compose.local.yml down
```

- CLIProxyAPI 内部 Key 配置错误：用 Task 1 的备份恢复 `config.yaml`，重启 CLIProxyAPI。

```bash
cd /Users/wujianxiang/CodeSpace/CLIProxyAPI
latest_backup="$(ls -t backups/config-before-sub2api-upstream-*.yaml | head -n 1)"
cp "$latest_backup" config.yaml
pkill -f cli-proxy-api || true
nohup ./cli-proxy-api --config config.yaml > logs/rollback-restart.log 2>&1 &
```

- yui.web 页面切换失败：回滚 yui.web 提交，或把 `.env` 中 `SHOP_LEGACY_KEY_ISSUANCE_DISABLED` 改回 `false` 后重启 yui.web。

```bash
cd /Users/wujianxiang/CodeSpace/yui.web
shop_commit="$(git log --format=%H --grep='feat(shop): point users to sub2api gateway' -n 1)"
git revert "$shop_commit"
```

## Plan Self-Review

- Spec coverage：方案 A 的三项职责边界、最小链路验证、yui.web 退场、端口冲突和 Docker 网络约束均有任务覆盖。
- Placeholder scan：计划未发现禁止占位语句。
- Type consistency：环境变量、文件路径、接口路径和任务引用一致。
- Scope check：本计划只完成方案 A 的第一阶段和入口页面切换；历史用户 / 历史账单迁移不在本计划内。
