#!/usr/bin/env bash
# 一键同步：拉取 origin(上游开源仓库) 最新 main，合并到当前分支，推送到 ccod(自己的 fork)
set -euo pipefail

branch=$(git rev-parse --abbrev-ref HEAD)

if [ -n "$(git status --porcelain)" ]; then
    echo "错误：工作区有未提交的改动，请先提交或 stash 再运行" >&2
    exit 1
fi

echo "==> fetch origin main"
git fetch origin main

count=$(git rev-list --count HEAD..origin/main)
if [ "$count" -eq 0 ]; then
    echo "==> 已是最新，无需合并"
else
    echo "==> 合并 origin/main ($count 个新提交) 到 $branch"
    if ! git merge origin/main -m "Merge upstream main into $branch"; then
        echo "" >&2
        echo "错误：合并有冲突，请手动解决后执行：" >&2
        echo "  git add <冲突文件> && git commit --no-edit && git push ccod $branch" >&2
        exit 1
    fi
fi

echo "==> push ccod $branch"
git push ccod "$branch"
echo "==> 完成"
