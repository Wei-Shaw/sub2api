# pprof 诊断与线上图片内存排查

## 开关与访问范围

需要先部署包含此功能的新版本；旧镜像不支持这些开关。

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `SUB2API_PPROF_ENABLED` | `false` | 设为 `true` 启用；修改后需重建/重启服务进程 |
| `SUB2API_PPROF_PORT` | `6060` | 仅在启用时读取，范围 1–65535 |

诊断服务独立于业务 HTTP 路由，启用后固定监听 `0.0.0.0`，不提供监听地址配置。
没有启用时不创建诊断监听器。端口冲突或启用配置非法会使启动失败并输出明确错误。
开启诊断不调整 GC、内存限制、图片请求或账号配置，也不默认开启 CPU、block、mutex 采样。

诊断入口没有认证。绑定所有 IPv4 接口后，容器网络中的其他进程可以访问；发布端口后外部也可访问。通过防火墙/安全组限制到管理员 IP，诊断结束后关闭开关并收回防火墙放行规则。堆、goroutine、trace 文件可能包含内部运行信息。
仍可通过 `docker exec` 在容器内抓取；远程访问使用 Compose 已内置的端口映射。

## 1. 开启诊断

仓库内四份 Compose（标准、local、standalone、dev）已内置开关传递和端口映射，不需要手动补 YAML。
默认 `SUB2API_PPROF_ENABLED=false`，程序不创建诊断监听器。Compose 仍预留宿主机 6060 端口；如被其他服务占用，设置 `SUB2API_PPROF_PORT` 为其他端口，宿主机和容器端口会同步变化。

先部署包含此功能的新镜像，然后在原部署目录执行：

```bash
SUB2API_PPROF_ENABLED=true docker compose up -d --no-deps sub2api

docker logs --tail 100 sub2api 2>&1 | grep 'pprof diagnostics enabled'
docker exec sub2api wget -q -O - http://127.0.0.1:6060/debug/pprof/
```

单次命令的变量只用于本次 Compose 操作；若需后续重建也保持开启，在现有 `.env` 中设置 `SUB2API_PPROF_ENABLED=true`。`.env.example` 已包含默认关闭的配置。
若原部署使用 `-f`、项目名或 env-file，请保留这些选项。例如 local 配置：

```bash
SUB2API_PPROF_ENABLED=true docker compose -f docker-compose.local.yml up -d --no-deps sub2api
```

无需手动添加端口映射；但服务器防火墙/云安全组仍需允许管理员 IP 访问诊断端口，仓库配置不会自动修改云安全组。
此步骤会重建服务，安排在无在途图片请求时操作。不要在重建前忘记记录原来的内存现场。

## 2. 保存基线、峰值和结束后的快照

以下在**线上宿主机**执行，使用镜像已有的 `wget`，无需在服务器安装 Go 或其他软件：

```bash
umask 077
profile_dir="$PWD/sub2api-pprof-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$profile_dir"

# 发请求前：GC 后仍存活的堆，以及累计分配基线。
docker exec sub2api wget -q -O - \
  'http://127.0.0.1:6060/debug/pprof/heap?gc=1' > "$profile_dir/heap-before.pb.gz"
docker exec sub2api wget -q -O - \
  'http://127.0.0.1:6060/debug/pprof/allocs' > "$profile_dir/allocs-before.pb.gz"
```

随后在后台用同一提示词、同一模型测试一个账号，记录通道、请求开始/完成时间、图片大小及其他并发流量。
另一个终端观察进程 RSS，看到高占用时，在定义了 `profile_dir` 的终端抓取：

```bash
# 峰值时先不主动触发 GC，避免先改变内存现场。
docker exec sub2api wget -q -O - \
  'http://127.0.0.1:6060/debug/pprof/heap' > "$profile_dir/heap-high.pb.gz"
docker exec sub2api wget -q -O - \
  'http://127.0.0.1:6060/debug/pprof/goroutine?debug=2' > "$profile_dir/goroutines-high.txt"

# 图片完成后：先记录自然状态，再抓 GC 后存活对象。
docker exec sub2api wget -q -O - \
  'http://127.0.0.1:6060/debug/pprof/heap' > "$profile_dir/heap-after.pb.gz"
docker exec sub2api wget -q -O - \
  'http://127.0.0.1:6060/debug/pprof/heap?gc=1' > "$profile_dir/heap-after-gc.pb.gz"
docker exec sub2api wget -q -O - \
  'http://127.0.0.1:6060/debug/pprof/allocs' > "$profile_dir/allocs-after.pb.gz"
```

确认各命令退出成功、输出文件非空；失败时不要把空文件当作快照。默认业务端口上的 `/debug/pprof/heap` 可能返回前端 HTML，不能替代上述独立端口。
普通 heap profile 主要反映最近已完成 GC 的统计，不保证与当时 RSS 一致；`gc=1` 会主动触发 GC，可能短暂停顿，并改变后续内存曲线。
不要用完成后的强制 GC 结果声称服务会自然回落。各通道使用独立目录，不覆盖快照；一次只测一条链路。

也可以在管理员电脑直接抓取（将地址替换为服务器 IP/域名）：

```bash
umask 077
curl --fail --show-error \
  'http://服务器IP:6060/debug/pprof/heap' -o heap-high.pb.gz
```

不要把 `0.0.0.0` 当作远程访问地址；它只是监听地址。其他快照可使用相同路径和参数。

## 3. 本地分析

通过现有 SSH/SCP 下载目录，在装有 Go 的本地电脑执行：

```bash
go tool pprof -top -sample_index=inuse_space \
  -base heap-before.pb.gz heap-after-gc.pb.gz

go tool pprof -top -sample_index=alloc_space \
  -base allocs-before.pb.gz allocs-after.pb.gz

go tool pprof -top -sample_index=inuse_space heap-high.pb.gz
```

- `inuse_space`：仍存活对象占用。重点看请求结束并 GC 后，哪些调用栈仍保留大对象。
- `alloc_space`：累计分配；高值代表分配频繁，不等于同一时刻占用，更不能直接称为泄漏。
- 对比 goroutine 文件，检查是否有未完成的图片读取、写回或后台任务。
- 这些是采样 profile，不是包含所有对象内容的完整进程转储；RSS 还包含堆空闲页、栈、代码等。

通常 profile 自带函数名即可查看 `top`。如需反汇编或源码定位，保留对应版本的二进制及源码，不能混用其他版本。

## 4. 关闭

使用原 Compose 文件组合关闭诊断：

```bash
SUB2API_PPROF_ENABLED=false docker compose up -d --no-deps sub2api
```

如果 `.env` 中曾设为 `true`，也改回 `false`，避免之后重建再次开启。映射保留，但程序不再监听；收回防火墙/安全组放行规则。
以下命令应连接失败；若设置过其他诊断端口，替换 6060：

```bash
docker exec sub2api wget -q -O - http://127.0.0.1:6060/debug/pprof/
```

无需给业务路由添加或删除任何接口。
