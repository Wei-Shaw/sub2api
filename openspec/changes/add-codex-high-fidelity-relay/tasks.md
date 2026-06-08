## 1. Implementation

- [x] 1.1 为 Codex 官方客户端新增高保真会话识别与 session hash 生成策略
- [x] 1.2 收紧 WSv2 转发逻辑，禁止高保真模式跨 turn 复用 turn state 与上游连接
- [x] 1.3 关闭高保真模式下 `previous_response_not_found` 的自动删锚点恢复
- [x] 1.4 补充并更新 HTTP WSv2 / ingress WS 回归测试
