# hello-world plugin

Sub2API 插件系统最小可运行的烟测样板。
配套 SDK 文档：[`plugin-sdk/README.md`](../../plugin-sdk/README.md)。

## 它演示了什么

| Capability | 实现位置 | 行为 |
|------------|----------|------|
| `http.register.plugin` | `RegisterHTTP` + `Manifest.PluginEndpoints` | 暴露 3 个 admin/无认证路由：`/hello`、`/db-test`、`/redis-test` |
| `db.own.read` | `handleDBTest` | 通过 `ctx.DB().QueryRowContext(...)` 跑 `SELECT 1` 检查 SQL 代理 |
| `redis.own` | `handleRedisTest` | 通过 `ctx.Redis().SetEx + Get` 检查 Redis 代理（自动加前缀 `plugin:hello-world:`） |
| `settings.own.read`（隐式） | `Manifest.SettingsSchema` + `handleHello` | ship `greeting` 设置项；`/hello` 用 `ctx.Settings().GetTyped` 读当前值 |
| `events.subscribe.lowfreq` | `handleAuthUserRegistered` | 订阅 `auth.user.registered` 事件，每条新用户注册打一行 log |
| 前端 bundle | `OpenFrontendFile` + `frontend/dist/entry.js` | 注册一个 admin 菜单项 + Vue 路由 |

> 没有演示 `secrets.encrypt` / `outbound.http` / `redis.raw` / `db.core.*` / `migrations.apply` / `jobs.register` —— 真实例子去看 [`plugins/channel-management`](../channel-management/)。

## 编译运行

```bash
# 在 worktree 根目录
go build -o ./bin/hello-world ./plugins/hello-world

# 本地手测（host 没启动；只能确认握手 + Manifest 不会 panic）
./bin/hello-world --core-sdk-addr=127.0.0.1:0 --log-level=debug
```

正常会先打印一行 handshake JSON 到 stdout：

```json
{"protocol":1,"grpc_addr":"127.0.0.1:54321","http_addr":"127.0.0.1:54322","pid":12345}
```

随后 SDK 等 host 调 `Init`；host 没起的话进程会停在 `select`，按 Ctrl-C 退出。

## 让 host 加载它

host（backend）启动时根据 `plugins/<name>/` 下的二进制 + Manifest 自动 spawn。

最简流程：

1. 在 worktree 根目录 `make plugins`（或手工 `go build -o plugins/hello-world/hello-world ./plugins/hello-world`）。
2. 启动 host：`cd backend && go run ./cmd/server`。
3. host 在启动日志里打印 `plugin loaded plugin=hello-world`。
4. 浏览器打开 `http://localhost:8080/admin`，sidebar 末尾应该出现 **Hello World** 菜单项。
5. 调用 admin API：

   ```bash
   curl http://localhost:8080/api/v1/plugin/hello-world/hello
   # {"message":"Hello from plugin!","version":"0.1.0"}

   # 下面两个需要 admin 鉴权（x-api-key）
   curl -H "x-api-key: $ADMIN_API_KEY" http://localhost:8080/api/v1/plugin/hello-world/db-test
   curl -H "x-api-key: $ADMIN_API_KEY" http://localhost:8080/api/v1/plugin/hello-world/redis-test
   ```

完整部署细节见 worktree 根目录的 `CLAUDE.md`（"服务器部署"一节）。

## 文件布局

```
plugins/hello-world/
├── README.md          # 本文件
├── go.mod / go.sum    # 独立 Go module，replace 指向 ../../plugin-sdk
├── main.go            # 单文件实现（Manifest / Init / Shutdown / 三个 HTTP handler）
└── frontend/
    └── dist/
        └── entry.js   # 预先打包好的前端，main.go 用 //go:embed 嵌入二进制
```

`main.go` 每段都加了详细注释，初次接触 SDK 时建议从 `Manifest()` 往下读。

## 修改建议

- 改 manifest 字段后必须重启 host 才能看到（host 在 spawn 时缓存 Manifest）。
- 想加新 capability 时先看 [`plugin-sdk/capabilities.go`](../../plugin-sdk/capabilities.go) 里的 canonical 列表，然后追加到 `Manifest.Capabilities`；落 wire 后 host 会回推批准列表。
- `Settings().GetTyped` 在 schema 漂移时返回 `pluginsdk.ErrSchemaVersionMismatch`，按 [`plugin-sdk/SETTINGS_API.md`](../../plugin-sdk/SETTINGS_API.md) 处理即可。
