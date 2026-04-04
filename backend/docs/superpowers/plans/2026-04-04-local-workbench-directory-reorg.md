# Local Workbench Directory Reorg Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把这一轮核心工作目录真正迁移到 `C:\Users\34404\Documents\GitHub\workbench` 下的分层结构中，并同步修正已知旧路径引用，保证后续工作可以在新根目录下持续进行。

**Architecture:** 迁移以目录分层为主线：先建立 `repos/`、`runtime/`、`toolchains/`、`archive/` 四层新根，再按“工具链与运行时优先、源码仓库随后、历史残留最后”的顺序搬迁。每完成一层，就立刻验证文件存在性、git 元数据、运行链路与工具链可用性，避免把多个风险叠到最后一起爆发。

**Tech Stack:** Windows PowerShell 5.1, git, Go toolchain, local filesystem operations, local runtime verification.

---

## 文件结构

- `C:\Users\34404\Documents\GitHub\workbench\`
  新的统一工作根目录。
- `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api`
  `sub2api` 新源码位置。
- `C:\Users\34404\Documents\GitHub\workbench\repos\codex-console`
  `codex-console` 新源码位置。
- `C:\Users\34404\Documents\GitHub\workbench\runtime\sub2api-runtime`
  本地 `sub2api` 运行时目录新位置。
- `C:\Users\34404\Documents\GitHub\workbench\toolchains\go`
  本地 Go 工具链新位置（原 `.local\go`）。
- `C:\Users\34404\Documents\GitHub\workbench\archive\openai-routing-observability`
  残留 worktree 壳目录的新归档位置。
- `C:\Users\34404\sub2api\backend\docs\superpowers\specs\2026-04-04-local-workbench-directory-reorg-design.md`
  本次整理设计依据。

## 任务拆分

### Task 1: 建立新根目录并做迁移前快照

**Files:**
- Create: `C:\Users\34404\Documents\GitHub\workbench\`
- Create: `C:\Users\34404\Documents\GitHub\workbench\repos\`
- Create: `C:\Users\34404\Documents\GitHub\workbench\runtime\`
- Create: `C:\Users\34404\Documents\GitHub\workbench\toolchains\`
- Create: `C:\Users\34404\Documents\GitHub\workbench\archive\`

- [ ] **Step 1: 先记录旧路径状态，作为迁移前快照**

Run:

```powershell
$targets = @(
  'C:\Users\34404\sub2api',
  'C:\Users\34404\sub2api-runtime',
  'C:\Users\34404\sub2api\.worktrees\openai-routing-observability',
  'C:\Users\34404\codex-console',
  'C:\Users\34404\.local\go'
)

foreach ($path in $targets) {
  Write-Host "=== $path ==="
  if (Test-Path $path) {
    Get-Item $path | Format-List FullName, PSIsContainer, LastWriteTime
  } else {
    Write-Host 'MISSING'
  }
}
```

Expected: 5 个目标路径在迁移前的存在性和基础元数据可读。

- [ ] **Step 2: 创建新的 workbench 四层目录**

Run:

```powershell
$root = 'C:\Users\34404\Documents\GitHub\workbench'
$dirs = @(
  $root,
  "$root\repos",
  "$root\runtime",
  "$root\toolchains",
  "$root\archive"
)

foreach ($dir in $dirs) {
  New-Item -ItemType Directory -Path $dir -Force | Out-Null
}
```

Expected: `workbench` 及其 4 个一级子目录存在。

- [ ] **Step 3: 校验新根目录结构正确**

Run:

```powershell
Get-ChildItem 'C:\Users\34404\Documents\GitHub\workbench' -Force
```

Expected: 至少能看到 `repos/`、`runtime/`、`toolchains/`、`archive/`。

### Task 2: 先迁移工具链和运行时，再验证本地运行链路

**Files:**
- Modify: `C:\Users\34404\.local\go` -> `C:\Users\34404\Documents\GitHub\workbench\toolchains\go`
- Modify: `C:\Users\34404\sub2api-runtime` -> `C:\Users\34404\Documents\GitHub\workbench\runtime\sub2api-runtime`

- [ ] **Step 1: 停掉仍依赖旧运行时路径的本地相关进程**

Run:

```powershell
$targets = Get-CimInstance Win32_Process | Where-Object {
  ($_.CommandLine -like '*sub2api-runtime*') -or
  ($_.ExecutablePath -like 'C:\Users\34404\sub2api-runtime*')
}

$targets | Select-Object ProcessId, Name, ExecutablePath, CommandLine | Format-List
```

Expected: 列出与 `sub2api-runtime` 相关的本地进程，便于后续手动/脚本停止。

- [ ] **Step 2: 真正移动 Go 工具链和 runtime 目录**

Run:

```powershell
Move-Item 'C:\Users\34404\.local\go' 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go'
Move-Item 'C:\Users\34404\sub2api-runtime' 'C:\Users\34404\Documents\GitHub\workbench\runtime\sub2api-runtime'
```

Expected: 两个旧路径消失，新路径存在。

- [ ] **Step 3: 立即验证 Go 工具链新路径仍能工作**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' version
```

Expected: 输出 `go version ...`。

- [ ] **Step 4: 验证 runtime 目录结构保持原样**

Run:

```powershell
Get-ChildItem 'C:\Users\34404\Documents\GitHub\workbench\runtime\sub2api-runtime' -Force
```

Expected: 仍能看到至少 `app/`、`data/`、`logs/`、`redis-data/`、`redis-sub2api.conf`。

- [ ] **Step 5: 修正我们当前已知的 Go / runtime 旧路径引用清单**

迁移后至少同步更新以下绝对路径使用点：

```text
C:\Users\34404\.local\go\bin\go.exe
C:\Users\34404\sub2api-runtime\app\sub2api.exe
C:\Users\34404\sub2api-runtime\data
C:\Users\34404\sub2api-runtime\logs
C:\Users\34404\sub2api-runtime\redis-sub2api.conf
```

Expected: 当前后续工作会继续使用的命令/脚本/配置，不再引用旧路径。

### Task 3: 迁移源码仓库和历史残留目录

**Files:**
- Modify: `C:\Users\34404\sub2api` -> `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api`
- Modify: `C:\Users\34404\codex-console` -> `C:\Users\34404\Documents\GitHub\workbench\repos\codex-console`
- Modify: `C:\Users\34404\sub2api\.worktrees\openai-routing-observability` -> `C:\Users\34404\Documents\GitHub\workbench\archive\openai-routing-observability`

- [ ] **Step 1: 先确认 sub2api 当前无活跃 git worktree 记录**

Run:

```powershell
git -C 'C:\Users\34404\sub2api' worktree list
```

Expected: 只剩主工作区；残留目录不是活跃 git worktree。

- [ ] **Step 2: 迁移 `sub2api` 和 `codex-console` 到 repos 层**

Run:

```powershell
Move-Item 'C:\Users\34404\sub2api' 'C:\Users\34404\Documents\GitHub\workbench\repos\sub2api'
Move-Item 'C:\Users\34404\codex-console' 'C:\Users\34404\Documents\GitHub\workbench\repos\codex-console'
```

Expected: 两个仓库都移入 `repos/`，旧路径不存在。

- [ ] **Step 3: 单独归档残留的 worktree 壳目录**

Run:

```powershell
Move-Item 'C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\openai-routing-observability' 'C:\Users\34404\Documents\GitHub\workbench\archive\openai-routing-observability'
```

如果 `Move-Item` 因占用失败，则记录失败对象并不要回滚前两步主迁移。

Expected: 残留目录要么成功进入 `archive/`，要么被单独识别为仍被占用的历史残留。

- [ ] **Step 4: 验证两个源码仓库在新路径仍可读**

Run:

```powershell
git -C 'C:\Users\34404\Documents\GitHub\workbench\repos\sub2api' status --short
git -C 'C:\Users\34404\Documents\GitHub\workbench\repos\codex-console' status --short
```

Expected: 两个 git 仓库都还能正常响应状态查询。

### Task 4: 在新路径下恢复本地工作链路并做收尾验证

**Files:**
- Modify: 所有当前仍引用旧绝对路径的本地配置/脚本/命令

- [ ] **Step 1: 在新路径下验证 `sub2api` 本地构建命令**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run TestNonExistent -count=1
```

Workdir: `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\backend`

Expected: 返回 `ok` 或 `[no tests to run]` 的编译级通过结果。

- [ ] **Step 2: 在新路径下验证 `codex-console` 项目结构可读**

Run:

```powershell
Get-ChildItem 'C:\Users\34404\Documents\GitHub\workbench\repos\codex-console' -Force
```

Expected: 至少还能看到 `.git/`、`src/`、`webui.py`、`requirements.txt` 等主结构。

- [ ] **Step 3: 在新路径下验证 `sub2api-runtime` 的配置和日志路径可读**

Run:

```powershell
Get-ChildItem 'C:\Users\34404\Documents\GitHub\workbench\runtime\sub2api-runtime\data' -Force
Get-ChildItem 'C:\Users\34404\Documents\GitHub\workbench\runtime\sub2api-runtime\logs' -Force
```

Expected: `data/`、`logs/` 可访问，无权限或路径错误。

- [ ] **Step 4: 做最终文件层验证**

Run:

```powershell
$old = @(
  'C:\Users\34404\sub2api',
  'C:\Users\34404\sub2api-runtime',
  'C:\Users\34404\codex-console',
  'C:\Users\34404\.local\go'
)

foreach ($path in $old) {
  Write-Host "$path => $(Test-Path $path)"
}

Get-ChildItem 'C:\Users\34404\Documents\GitHub\workbench' -Recurse -Depth 2 -Force
```

Expected: 旧主路径全部返回 `False`，新 workbench 树在 2 层深度内结构清晰。

- [ ] **Step 5: 输出迁移结果与剩余人工项**

结果总结必须明确包含：

1. 新根目录位置
2. 各旧路径 -> 新路径映射
3. 哪些旧路径引用已同步修改
4. `archive/openai-routing-observability` 是否已成功归档，还是仍有外部占用

## 自查清单

- Spec 覆盖：
  - `workbench` 四层结构：Task 1
  - 真搬迁而非兼容跳板：Task 2 + Task 3
  - 路径引用同步修改：Task 2 + Task 4
  - `archive` 处理残留目录：Task 3 + Task 4
  - 四层验证：Task 2/3/4
- 占位词检查：没有 `TODO/TBD/implement later` 等占位描述。
- 类型/路径一致性：新根统一为 `C:\Users\34404\Documents\GitHub\workbench`，子层统一为 `repos/runtime/toolchains/archive`，不混用其他命名。
