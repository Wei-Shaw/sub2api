# 项目协作规则

## Merge 到 main 前版本确认

只有当用户明确提出“merge到main”时，才执行以下版本确认：

1. 读取根目录 `version.txt`，向用户说明当前版本号。
2. 询问用户是否需要增加版本号。
3. 只有在用户明确确认后，才可以修改 `version.txt`；如果用户不增加版本号，则保持版本文件不变。

普通代码修改、提交和 push 操作不需要触发版本号提醒或询问。

## Merge 到 main 前 MR 说明

在合并到 `main` 分支前，MR 描述必须填写本次代码变更点，至少说明修改内容和影响范围。

用户明确提出“merge到main”时，合并成功后切换到本地 `main` 分支；合并失败或未完成时不得切换。

## Merge 合并方式

创建 Pull Request 后，使用 GitHub 的 `Merge when all checks have passed`，等待所有流水线检查通过后自动合入 `main`。

## Push 前检查

每次 push 代码前，必须先执行项目 lint 检查；lint 未通过时不得 push，需先修复问题并重新检查。
