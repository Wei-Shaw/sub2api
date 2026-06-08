## 1. Implementation

- [x] 1.1 新增 HTTP 入站保留 WSv2 的受限判定，并补 transport 决策测试
- [x] 1.2 抽出 passthrough body 预处理，供 HTTP passthrough 与 HTTP->WS passthrough 共用
- [x] 1.3 调整 `Forward` 分流顺序，让受限 Codex HTTP 流量进入 WSv2
- [x] 1.4 补充 HTTP 入站走 WSv2 / 保持 HTTP 的回归测试
- [x] 1.5 本地运行相关后端测试并验证通过
