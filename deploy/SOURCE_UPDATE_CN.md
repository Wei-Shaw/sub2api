# Sub2API 上游源码合并指南

本文档适用于当前仓库的二次开发分支结构：

| 名称 | 用途 |
|---|---|
| `upstream/main` | 官方仓库 `Wei-Shaw/sub2api` 的主分支 |
| `origin` | 个人仓库 `afushui/sub2api` |
| `custom/main` | 当前二次开发分支 |

所有 Git 命令均在仓库根目录执行：

```bash
cd /Users/fushui/sub2api
```

## 1. 检查当前状态

```bash
git status --short --branch
git remote -v
```

更新前必须先处理未提交修改。不要在存在重要未提交修改时直接执行 `git pull` 或 `git merge`。

## 2. 提交二次开发修改

先检查实际改动：

```bash
git diff
git diff --check
```

只暂存本次需要提交的文件：

```bash
git add <文件路径>
git commit -m "custom: describe the local changes"
```

不建议直接使用 `git add .`，避免把 `.env`、构建产物或无关文件加入提交。

如果暂时不准备提交，可以使用 Stash：

```bash
git stash push --include-untracked -m "before upstream merge"
```

合并完成后恢复：

```bash
git stash pop
```

对于正式二开内容，优先提交到 Git，Stash 只适合短期保存。

## 3. 获取官方更新

```bash
git fetch upstream
```

查看官方新增提交：

```bash
git log --oneline custom/main..upstream/main
```

查看双方提交数量：

```bash
git rev-list --left-right --count custom/main...upstream/main
```

输出的两个数字分别表示 `custom/main` 独有提交数和 `upstream/main` 独有提交数。

## 4. 合并到二次开发分支

```bash
git switch custom/main
git merge upstream/main
```

本项目推荐使用 `merge` 保留官方更新和二开提交的边界，不建议对已经推送的 `custom/main` 执行 Rebase。

如果没有冲突，直接进入验证步骤。

## 5. 解决合并冲突

查看冲突文件：

```bash
git status
git diff --name-only --diff-filter=U
```

冲突文件中会出现：

```text
[当前分支开始标记] HEAD
当前二开代码
[分隔标记]
官方更新代码
[合并分支结束标记] upstream/main
```

根据最终需求整合两侧代码，并删除所有冲突标记。完成后执行：

```bash
git add <已解决的文件>
git diff --check
git commit
```

确认没有残留冲突标记：

```bash
git status
git grep -n -e '^<<<<<<< ' -e '^=======$' -e '^>>>>>>> '
```

如果尚未提交合并，而且决定放弃本次合并：

```bash
git merge --abort
```

`git merge --abort` 只用于正在进行的合并，不要使用 `git reset --hard` 清理冲突。

## 6. 验证合并结果

检查提交历史和工作区：

```bash
git log --oneline --graph --decorate -20
git status --short --branch
git diff --check
```

使用 Docker 完成完整生产构建：

```bash
cd /Users/fushui/sub2api/deploy
./build_image.sh
```

Docker 构建会执行前端类型检查、Vite 生产构建和 Go 后端编译。

## 7. 推送个人仓库

验证通过后推送二开分支：

```bash
git push origin custom/main
```

不要把二开分支直接推送到 `upstream/main`。

## 8. 部署更新

当前 PostgreSQL 和 Redis 已独立运行，只更新应用容器：

```bash
cd /Users/fushui/sub2api/deploy
./deploy_local_image.sh
```

也可以连续完成构建和部署：

```bash
./build_image.sh && ./deploy_local_image.sh
```

部署脚本不会重建 PostgreSQL、Redis，也不会删除现有数据卷。

## 日常更新速查

当工作区已经提交干净时，完整流程为：

```bash
cd /Users/fushui/sub2api
git fetch upstream
git switch custom/main
git merge upstream/main

cd /Users/fushui/sub2api/deploy
./build_image.sh
./deploy_local_image.sh

cd /Users/fushui/sub2api
git push origin custom/main
```
