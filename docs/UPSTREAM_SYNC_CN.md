# 同步上游仓库说明

这个仓库当前建议按下面的分支模型维护：

- `upstream/main`：原始开源项目 `Wei-Shaw/sub2api` 的主线
- `origin/custom/main`：你自己的定制主分支
- 日常同步方向：`upstream/main -> custom/main`

## 已提供脚本

仓库内新增了一个 PowerShell 脚本：

`tools/sync-upstream.ps1`

默认行为：

- 检查工作区是否干净
- 拉取最新 `upstream`
- 切换到 `custom/main`
- 分析 `custom/main` 和 `upstream/main` 的差异
- 如果上游有新提交，则执行合并
- 可选推送到 `origin/custom/main`

## 常用命令

只检查状态，不真正合并：

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\sync-upstream.ps1 -DryRun
```

执行同步，但不自动推送：

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\sync-upstream.ps1
```

执行同步并推送到你的 fork：

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\sync-upstream.ps1 -Push
```

## 可选参数

- `-TargetBranch`：默认 `custom/main`
- `-UpstreamRemote`：默认 `upstream`
- `-UpstreamBranch`：默认 `main`
- `-OriginRemote`：默认 `origin`
- `-Push`：同步后自动 `git push`
- `-DryRun`：只检查，不改动仓库
- `-AllowDirty`：允许在脏工作区运行，不建议常用

## 推荐流程

每次准备同步上游时：

1. 先提交或暂存你本地未完成的修改
2. 运行 `-DryRun` 看上游是否真的有新提交
3. 再执行正式同步
4. 如果没有冲突，确认结果后推送到 `origin/custom/main`

## 发生冲突时

脚本检测到冲突后会停下，并列出冲突文件。处理方式：

```powershell
git status
git add <已解决的文件>
git commit
```

如果你想放弃这次合并：

```powershell
git merge --abort
```

## 当前仓库状态

按本次检查结果，当前 `custom/main` 相对 `upstream/main` 是你自己多了提交，而上游暂时没有新的提交，所以现在执行脚本通常会得到“已经是最新”的结果。
