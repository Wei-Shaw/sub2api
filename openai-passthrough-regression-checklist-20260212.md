# OpenAI 自动透传回归测试清单（2026-02-12）

## 目标
- 验证 OpenAI 账号（OAuth/API Key）“自动透传”开关在创建页与编辑页可正确开关。
- 验证开启后请求透传（仅替换认证），并保留计费/并发/审计等网关能力。
- 验证 `User-Agent` 头透传到上游，且 Usage 页面展示原始 UA（不映射、不截断）。

## 自动化测试
在仓库根目录执行：

```bash
(cd backend && go test ./internal/service -run 'OpenAIGatewayService_.*Passthrough|TestAccount_IsOpenAIPassthroughEnabled|TestAccount_IsOpenAIOAuthPassthroughEnabled' -count=1)
(cd backend && go test ./internal/handler -run OpenAI -count=1)
pnpm --dir frontend run typecheck
pnpm --dir frontend run lint:check
```

预期：
- 所有命令退出码为 `0`。

## 手工回归场景

### 场景1：创建 OpenAI API Key 账号并开启自动透传
1. 进入管理端账号创建弹窗，平台选择 OpenAI，类型选择 API Key。
2. 打开“自动透传（仅替换认证）”开关并保存。
3. 检查创建后的账号详情。

预期：
- `extra.openai_passthrough = true`。
- 模型白名单/映射区域显示“不会生效”的提示。

### 场景2：编辑 OpenAI OAuth 账号开关可开可关
1. 打开已有 OpenAI OAuth 账号编辑弹窗。
2. 将“自动透传（仅替换认证）”从关切到开并保存。
3. 再次进入编辑页，将开关从开切到关并保存。

预期：
- 开启后：`extra.openai_passthrough = true`。
- 关闭后：`extra.openai_passthrough` 与 `extra.openai_oauth_passthrough` 均被清理。

### 场景3：请求链路透传（含 User-Agent）
1. 使用设置为“自动透传=开启”的 OpenAI 账号发起 `/v1/responses` 请求。
2. 请求头设置 `User-Agent: codex_cli_rs/0.1.0`（或任意自定义 UA）。

预期：
- 上游收到与下游一致的 `User-Agent`。
- 请求体保持原样透传，仅认证头被替换为目标账号令牌。

### 场景4：Usage 页面原样显示 User-Agent
1. 进入管理端用量表（Admin Usage）与用户侧用量页（User Usage）。
2. 查找包含长 UA 的记录。

预期：
- 显示原始 UA 文本（不再映射为 VS Code/Cursor 等）。
- 文本可换行完整展示，不被 `...` 截断。
