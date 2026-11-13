# 额度矩阵与跨账号总览：界面对比及模拟验证

2026-11-14，PR #8425。对照基线为 upstream/main `530c5e3a4f9c73ca2b0881191d0ef8e0d82c5484`（v0.2.21 发布后的主线）。

保留矩阵与跨账号总览，并接回 #8590 的 Antigravity 强制查询按钮、#8581 的刷新期间保留账号行和额度显示。列表与总览共享五分钟内存缓存，不再将额度响应持久化到 localStorage；刷新浏览器后重新查询。国产供应商、Ollama Cloud、OpenCode Go 与无限额度标签保持主线组件，AI Credits 和 Grok 预付余额保持普通文字。

## 界面对比

以下是生产构建的浏览器原始截图，使用相同的 6 个模拟账号。官方列表显示已用比例，矩阵显示剩余比例：7% 已用对应 93% 剩余。

| 场景 | upstream/main | 本 PR |
| --- | --- | --- |
| 桌面账号列表 | ![上游桌面列表](screenshots/quota-overview/main-desktop-list.png) | ![矩阵桌面列表](screenshots/quota-overview/pr-desktop-list.png) |
| 手机账号列表 | ![上游手机列表](screenshots/quota-overview/main-mobile-list.png) | ![矩阵手机列表](screenshots/quota-overview/pr-mobile-list.png) |

| 桌面总览 | 手机总览 |
| --- | --- |
| ![桌面总览](screenshots/quota-overview/pr-desktop-overview.png) | ![手机总览](screenshots/quota-overview/pr-mobile-overview.png) |

1000 账号时汇总覆盖全部账号，明细每页 25 条：

![1000 账号总览](screenshots/quota-overview/pr-large-overview.png)

缺少分组摘要时，列表保留明确标注为已用比例的模型快照，不将其推断为五小时/周窗口或计入周期总览。

![模型快照回退](screenshots/quota-overview/model-snapshot-fallback.png)

## 刷新行为与回归

- 普通查询已发出后收到强制刷新：排队一次强制查询；重复的强制请求合并。
- 普通查询与其他账号的强制查询分批发送，避免把整批账号都升级为上游强制查询。
- 单元格主动查询返回后，旧批量响应不能覆盖新值。
- 登录用户改变时丢弃旧队列和旧结果；额度响应只存在当前页面会话内存中。
- 关闭重开总览复用缓存，翻页和筛选不重新查询；浏览器重载重新采集，每个账号每轮查询一次。
- 总览打开时暂停账号页自动刷新，关闭后恢复。
- 官方 Antigravity 查询按钮在实际浏览器中保留，点击发送 `source=active&force=true`。

## 模拟规模验证

macOS，Playwright 1.60.0、Edge 157.0.4341.46；桌面 1440×1000，手机 390×844。所有账号、接口和错误均模拟；静态服务只绑定本机，浏览器拦截所有非本地请求，不连接实际部署或提供商账号。

账号列表接口延迟 25 ms、额度批量接口延迟 50 ms。完整加载从打开总览到全部采集完成并等待 100 ms 计时；以下为本机功能验证中的单次采样，真实上游延迟另计。

| 场景 | 完整加载 | 弹窗元素 | 明细行 |
| --- | --- | --- | --- |
| 桌面 / 100 账号 | 352 ms | 1327 | 25 |
| 桌面 / 500 账号 | 938 ms | 1728 | 25 |
| 桌面 / 1000 账号 | 1988 ms | 2228 | 25 |
| 手机 / 1000 账号 | 1970 ms | 2257 | 25 |

- 批次串行执行，每批最多 50 个账号，列表与总览合并重叠请求。
- 100 账号中模拟 10% 失败，错误可见，手动刷新全部恢复。
- 混合渠道/套餐验证覆盖 PRO、ULTRA、OpenAI、Anthropic、模型快照、过期数据、不可调度账号和未知套餐；分别汇总并排除无效周期容量。
- 浏览器运行时错误及未模拟 API 均为 0。所有场景验证主动查询按钮与浏览器重载后的重新采集。
- 完整前端测试：387 个文件、3179 项通过；类型检查、ESLint、生产构建通过。

精简测量记录见 [metrics.json](screenshots/quota-overview/metrics.json)。

## 复现

[tools/quota-review.mjs](../tools/quota-review.mjs) 仅操作传入的静态构建。浏览器依赖可安装在临时目录。

```sh
review_dir=$(mktemp -d)
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend exec vite build --outDir "$review_dir/pr-dist"
npm install --prefix "$review_dir/browser" playwright@1.60.0
"$review_dir/browser/node_modules/.bin/playwright" install chromium
export PLAYWRIGHT_MODULE="$review_dir/browser/node_modules/playwright/index.mjs"
node tools/quota-review.mjs --after="$review_dir/pr-dist" --out="$review_dir/full" --verify=true
node tools/quota-review.mjs --after="$review_dir/pr-dist" --out="$review_dir/mixed" --mode=mixed --verify=true
```

可设置 `BROWSER_EXECUTABLE` 使用已安装的 Chromium 系浏览器，本次使用 Edge。截图对照需在独立 checkout 构建上述主线，额外传入 `--before=/absolute/path/to/main-dist --mode=shots`。每次运行输出包含请求记录、断言结果和计时的 `results.json`。
