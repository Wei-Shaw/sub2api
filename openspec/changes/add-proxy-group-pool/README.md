# add-proxy-group-pool

在不改变现有「账号绑定单个代理」语义的前提下，新增代理池（代理分组）能力：账号可绑定一个代理组，由服务端在账号 hydration 时按策略从组内挑选一个健康代理，使 grok 及全部平台的上游出站请求获得多出口轮换能力。

阅读顺序：`proposal.md` → `design.md` → `specs/proxy-group-pool/spec.md` → `tasks.md` → `verification.md`。

实施分阶段，Phase 0 为可独立评审、可独立合并的纯重构，后续阶段依赖其成果。
