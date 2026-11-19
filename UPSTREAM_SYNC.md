# 上游同步操作手册

自己的仓库：`git@github.com:wklong/sub2api.git`（origin）。
源仓库：`git@github.com:Wei-Shaw/sub2api.git`（upstream）。
自己的 main 包含已完成的定制功能和部署配置，按上游正式版本创建同步 PR。
本地远程配置不随 Git 提交分发，新克隆仓库需自行执行：

```sh
git remote add upstream git@github.com:Wei-Shaw/sub2api.git
git remote -v
```

如 upstream 已存在，先检查地址，无需重复添加。

## 按正式版本同步

以下命令在仓库根目录执行，PowerShell 和常见 Shell 均可使用。
先提交或暂存现有工作，并确保工作区干净。

```sh
git switch main
git pull --ff-only origin main
git fetch upstream --no-tags 'refs/heads/main:refs/remotes/upstream/main' 'refs/tags/*:refs/tags/upstream/*'
git tag --list 'upstream/*'
```

上游标签保存到本地 `upstream/` 命名空间，避免与自己的发布标签混淆。
下面的 `vX.Y.Z` 是占位符，需从上游 Release 选择真实的稳定版本；列表不代表版本已验证。

```sh
git switch -c sync/upstream-vX.Y.Z
git merge --no-ff refs/tags/upstream/vX.Y.Z
```

若有冲突，逐文件理解双方意图，特别检查配置、支付逻辑、数据库迁移和工作流；
不要批量采用一侧内容。解决后只暂存确认过的文件并提交。
需要放弃尚未提交的本次合并时可使用 `git merge --abort`。

执行相关测试和构建后推送同步分支：

```sh
git push -u origin sync/upstream-vX.Y.Z
```

创建 PR 合并到自己的 main，PR 说明上游版本、冲突处理、自定义功能回归和迁移影响。
**同步 PR 必须使用普通 merge commit，不能使用 Squash 或 Rebase 合并**，以保留上游祖先关系。
仓库需启用 “Allow merge commits”；main 的规则不能强制线性历史。
不要 force push main，不要用上游文件整目录覆盖自己的仓库。

## 每次同步必查

- 自有充值入口、权限、支付回调、余额计算和历史数据兼容性。
- `deploy/custom/compose.yml` 的继承配置是否仍兼容基础 Compose，新增必填变量是否已记录。
- Dockerfile 的构建参数、Go/Node/pnpm 版本、镜像架构、数据库主版本及存储路径变化。
- `.github/workflows/release.yml` 的上游仓库限制是否保留，自有工作流是否仍只发布自有标签。
- `.github/workflows/` 中其他新增工作流、权限、外部脚本和发布行为。
- 新迁移的顺序和兼容性。不要修改已在生产执行的迁移文件。

验证和 PR 合并完成后，按 `deploy/custom/README.md` 创建新的自有版本并部署。
只推送自己的标签，不执行 `git push --tags`；后者可能把本地缓存的上游标签一起推送并触发意外工作流。

同步说明中记录基线，例如“上游 vX.Y.Z，定制 vX.Y.Z-custom.1”。
不要仅为区分 Fork 批量修改 Go module/import 路径，这会扩大后续同步冲突。
