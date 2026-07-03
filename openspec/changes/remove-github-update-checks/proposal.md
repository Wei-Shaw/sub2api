# Change: 移除 GitHub 远端更新检测与在线更新入口

## Why
当前后台版本徽标把“本地构建版本展示”和“GitHub Release 远端更新检测”耦合在一起，导致管理端只是展示版本号，也会隐式依赖 GitHub 作为上游版本真相源。现在产品要脱离 GitHub 主版本更新，这套检测与在线更新链路会持续制造错误提示、误导运维判断，并把部署方式错误固化进系统行为。

## What Changes
- 移除管理端版本徽标中的远端更新检测、更新提示点和在线更新交互
- 将管理端系统版本接口收敛为“仅返回当前构建版本”，不再触发 GitHub Release 查询
- 下线 GitHub 驱动的更新检查、在线更新、回滚入口及其后端依赖
- 保留与更新机制无关的本地版本展示和服务重启能力

## Impact
- Affected specs: `admin-system-version`
- Affected code: 管理端版本徽标、前端应用状态缓存、系统管理接口、GitHub Release 更新服务与依赖注入
