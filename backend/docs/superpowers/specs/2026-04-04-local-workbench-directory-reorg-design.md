# 本地 Workbench 目录整理设计

## 背景

这一轮与 `sub2api / codex-console / CPA / 本地运行时 / Go 工具链` 相关的工作目录目前分散在用户根目录下，主要包括：

- `C:\Users\34404\sub2api`
- `C:\Users\34404\sub2api-runtime`
- `C:\Users\34404\sub2api\.worktrees\openai-routing-observability`
- `C:\Users\34404\codex-console`
- `C:\Users\34404\.local\go`

此外，这一轮还零星用到过 `CLIProxyAPI`，但该目录与其 release 目录已经按用户要求删除，不再纳入后续整理范围。

当前问题不在于目录数量本身，而在于：

1. 源码仓库、运行时目录、工具链目录和历史残留混在同一层级。
2. 后续继续维护时，不容易快速判断“哪个目录是代码、哪个是运行态、哪个是临时遗留”。
3. 如果继续在现有结构上增量堆叠，长期会反复产生“找路径、改绝对路径、清理残留”的额外成本。

## 目标

本次整理的目标是：

1. 在 `C:\Users\34404\Documents\GitHub` 下建立一个新的统一工作根目录。
2. 采用分层结构，明确区分源码仓库、运行时目录、工具链和历史残留。
3. 对目标目录做真正搬迁，而不是只建快捷方式或入口层。
4. 同步修正已知的旧路径引用，让后续工作直接基于新根目录继续。
5. 在整理完成后，对文件存在性、git 可读性、运行链路和工具链可用性做验证。

## 非目标

本次不做以下事情：

1. 不回头整理 OpenCode 相关目录（如 `.config\opencode`、`.cache\opencode`、`.codex`、`.opencode` 等）。
2. 不把所有用户根目录下的开发项目都并入同一根目录，只处理当前这轮明确圈定的几类目录。
3. 不依赖兼容跳板（junction/symlink）长期保留旧路径；如果需要兼容，只用于短期迁移辅助，不作为最终状态。
4. 不在这一步顺手处理无关项目或再做大范围磁盘清理。

## 目标结构

新根目录放在：

- `C:\Users\34404\Documents\GitHub\workbench`

目录结构定义为：

```text
workbench/
  repos/
    sub2api/
    codex-console/
  runtime/
    sub2api-runtime/
  toolchains/
    go/
  archive/
    openai-routing-observability/
```

各目录映射关系如下：

| 旧路径 | 新路径 | 类型 |
| --- | --- | --- |
| `C:\Users\34404\sub2api` | `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api` | 主源码仓库 |
| `C:\Users\34404\codex-console` | `C:\Users\34404\Documents\GitHub\workbench\repos\codex-console` | 辅助源码仓库 |
| `C:\Users\34404\sub2api-runtime` | `C:\Users\34404\Documents\GitHub\workbench\runtime\sub2api-runtime` | 本地运行时 |
| `C:\Users\34404\.local\go` | `C:\Users\34404\Documents\GitHub\workbench\toolchains\go` | 工具链 |
| `C:\Users\34404\sub2api\.worktrees\openai-routing-observability` | `C:\Users\34404\Documents\GitHub\workbench\archive\openai-routing-observability` | 历史残留 |

## 迁移原则

### 1. 真搬迁，不保留旧路径为最终状态

用户已经明确接受“允许改旧路径引用”，因此本次整理按“整体迁移到新根目录”设计，而不是保留旧路径为主要入口。

这意味着：

1. 目录本体要真正移动到 `workbench` 下。
2. 旧位置不应在最终状态继续保留同名目录。
3. 后续系统认知统一迁移到新路径，不继续围绕旧路径做维护。

### 2. 路径引用同步迁移

这次整理不是单纯的文件操作，还必须把已知依赖旧路径的配置/脚本/运行命令一起迁移。

至少要把以下几类引用作为一等迁移对象：

1. `sub2api-runtime` 启动相关命令里对 `DATA_DIR`、二进制路径、日志目录、Redis 配置路径的引用。
2. 本地构建/交叉编译命令里对 `.local\go` 的引用。
3. 本轮仍会继续使用的本机配置中，对 `sub2api` 或 `.local\go` 的显式绝对路径引用。
4. 若 `codex-console` 有本地脚本或日志路径写死，也要同步更新。

### 3. 历史残留单独归档，不让它阻塞主迁移

`openai-routing-observability` 这个残留目录已经没有 git worktree 记录，但目录本体仍留在磁盘上，且历史上出现过“外部进程占用”现象。

本次设计把它视为历史残留：

1. 先整体搬进 `archive/`，与主工作目录隔离。
2. 搬迁后再判断是否为空壳、是否仍被外部占用。
3. 如果后续确认它已无实际用途，再单独删除。

这样可以避免一个历史壳目录拖累整个主迁移过程。

## 验证方案

整理完成后，验证分 4 层进行：

### A. 文件层

确认以下结果：

1. 5 个目标目录都存在于 `workbench` 新位置。
2. 旧位置不存在同名目录。
3. `workbench` 目录结构符合 `repos/runtime/toolchains/archive` 设计。

### B. Git / 项目层

确认：

1. `workbench\repos\sub2api` 仍能被识别为 git 仓库，`git status` / `git worktree list` 可正常读取。
2. `workbench\repos\codex-console` 如果原本是 git 仓库，则其仓库元数据仍可正常读取。

### C. 运行层

至少验证本地 `sub2api-runtime` 在新路径下仍满足：

1. 配置文件位置正确。
2. 日志目录可写。
3. 本地运行实例可按新路径启动。

### D. 工具链层

确认当前这套 Go 调用链仍可用，例如：

1. `go version` 正常。
2. 当前构建命令能从新路径的 Go 工具链正常执行。

## 风险与控制

### 风险 1：脚本/配置仍残留旧绝对路径

这是本次最大的真实风险。

控制方式：

1. 在迁移前先列出已知绝对路径引用。
2. 搬迁后按“运行时、工具链、仓库”三类逐项校验。
3. 不把“旧路径兼容跳板”当作默认解法，而是优先把引用改干净。

### 风险 2：运行时目录移动后本地实例起不来

控制方式：

1. 迁移 `sub2api-runtime` 时保留其内部相对结构不变。
2. 搬迁后优先验证本地启动链路，而不是等到整理全部做完再统一发现问题。

### 风险 3：历史 worktree 壳目录被占用，导致迁移失败

控制方式：

1. 不把它和主工作目录绑定在一起迁移。
2. 先归入 `archive/`。
3. 如果出现占用问题，只影响 archive 分支，不阻塞 `sub2api` / runtime / Go 工具链主迁移。

## 最终结论

本次整理采用：

1. `Documents\GitHub\workbench` 作为新的统一工作根目录。
2. `repos/runtime/toolchains/archive` 四层结构。
3. 真搬迁目录本体，不把旧路径作为最终状态保留。
4. 同步迁移已知绝对路径引用。
5. 对历史残留的 worktree 壳目录先归档、后判断是否清理。

这套结构的好处是：

1. 后续你可以持续把“代码、运行态、工具链、残留”分开维护。
2. `sub2api` 与 `codex-console` 作为仓库会留在同一类目录下，便于继续开发。
3. `sub2api-runtime` 与 `.local\go` 不再混在用户根目录，后续再迁别的项目时也有现成骨架可复用。
