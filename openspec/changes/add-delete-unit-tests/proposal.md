# Change: Add unit tests for delete paths (user/group/proxy/redeem)

## Why
删除流程缺少单元测试，容易在重构或边界条件变化时回归，且问题排查成本高。

## What Changes
- 新增服务层删除流程单元测试（覆盖 AdminService 删除入口与对应 Repo 错误传播）
- 覆盖成功/不存在/权限保护/幂等删除/底层错误等关键分支
- 需要的轻量测试替身（repositories / cache）

## Impact
- Affected specs: testing (new)
- Affected code: backend/internal/service/admin_service.go
