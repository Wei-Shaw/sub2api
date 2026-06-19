# AGENTS 记忆压缩 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将根目录 `AGENTS.md` 从流水账式长期记忆压缩为高频入口，并把完整压缩记忆统一归档到 `docs/ai/context/`。

**Architecture:** `AGENTS.md` 只承担启动时快速读取的索引和约束职责；压缩后的项目事实、运行态和历史索引写入新的 `docs/ai/context/*compressed-memory*.md` 文档。后续新增长期记忆继续用 `docs/ai/context/YYYYMMDD-HHMMSS-*.md`，不覆盖历史文件。

**Tech Stack:** Markdown 文档；无需代码构建。

---

### Task 1: 新增压缩记忆文档

**Files:**
- Create: `docs/ai/context/20260619-151920-sub2api-compressed-memory_CN.md`

- [ ] **Step 1: 提炼根目录现有记忆**

读取 `AGENTS.md`，按主题归并为：
- 架构定论
- 当前公网和本地链路
- 上游池、套餐、Key 与用户迁移
- 支付、邮件和用户页面状态
- 前端资源与 nginx/Cloudflare 注意事项
- 后续操作禁忌

- [ ] **Step 2: 写入压缩记忆文档**

新建 `docs/ai/context/20260619-151920-sub2api-compressed-memory_CN.md`，保留关键事实与详细记录引用，不写完整 API Key、内部 token、HMAC secret。

### Task 2: 压缩根目录 AGENTS

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: 替换为索引式短记忆**

将 `AGENTS.md` 改为：
- 指向新的压缩记忆文档
- 保留最重要的架构定论和操作禁忌
- 保留最新运行态摘要
- 明确后续长期记忆写入 `docs/ai/context/`

- [ ] **Step 2: 避免信息损失**

确保根目录删除的细节都能在新压缩文档中找到对应摘要或原始详细文档链接。

### Task 3: 验证

**Files:**
- Read: `AGENTS.md`
- Read: `docs/ai/context/20260619-151920-sub2api-compressed-memory_CN.md`

- [ ] **Step 1: 检查文件长度和路径**

Run: `wc -l AGENTS.md docs/ai/context/20260619-151920-sub2api-compressed-memory_CN.md docs/ai/context/20260619-151920-agents-memory-compression-plan_CN.md`

Expected: `AGENTS.md` 明显短于原文件，两个新文档位于 `docs/ai/context/`。

- [ ] **Step 2: 检查差异范围**

Run: `git diff -- AGENTS.md docs/ai/context/20260619-151920-sub2api-compressed-memory_CN.md docs/ai/context/20260619-151920-agents-memory-compression-plan_CN.md`

Expected: 只修改 `AGENTS.md`，只新增本次两个上下文文档。
