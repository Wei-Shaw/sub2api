## 1. Implementation
- [x] 1.1 为 AdminService 删除入口准备测试替身（user/group/proxy/redeem repo 与 cache）
- [x] 1.2 新增 AdminService.DeleteUser 单元测试（成功/不存在/错误传播/管理员保护）
- [x] 1.3 新增 AdminService.DeleteGroup 单元测试（成功/不存在/错误传播，缓存失效逻辑如适用）
- [x] 1.4 新增 AdminService.DeleteProxy 单元测试（成功/幂等删除/错误传播）
- [x] 1.5 新增 AdminService.DeleteRedeemCode 与 BatchDeleteRedeemCodes 单元测试（成功/幂等删除/错误传播/部分失败）
- [x] 1.6 运行 unit 测试并将结果记录在本 tasks.md 末尾

## Test Results
- `go test -tags=unit ./internal/service/...` (workdir: `backend`)
  - ok  	github.com/Wei-Shaw/sub2api/internal/service	0.475s
