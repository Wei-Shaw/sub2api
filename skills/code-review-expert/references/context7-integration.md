# Context7 MCP Anti-Hallucination Integration

## Overview

Context7 MCP 提供两个工具，用于拉取第三方库的最新官方文档，消除 LLM 训练数据时效性导致的代码审核幻觉。

## Tools

### resolve-library-id

```
输入: libraryName (如 "gin", "gorm", "react", "express")
输出: Context7 兼容的 library ID (如 "/gin-gonic/gin")
```

- 必须在 `get-library-docs` 之前调用
- 用户已提供 `/org/project` 格式 ID 时可跳过
- 解析失败则记录到 `c7_failures`，跳过该库

### get-library-docs

```
输入:
  - context7CompatibleLibraryID: 从 resolve-library-id 获取
  - topic (可选): 聚焦主题 (如 "middleware", "hooks", "query")
  - tokens (可选): 最大返回 token 数 (默认 5000)
```

- 每个库每次审核最多调用 **3 次**
- 优先用 `topic` 缩小范围
- 缓存首次查询结果，后续复用

## Three-Layer Verification

### Layer 1: Pre-Review Warm-up (Phase 0.5)

在审核开始前预热文档缓存：

1. **扫描依赖清单**：
   ```bash
   for f in go.mod package.json requirements.txt Pipfile pyproject.toml \
            Cargo.toml Gemfile pom.xml build.gradle composer.json mix.exs \
            pubspec.yaml *.csproj; do
     [ -f "$f" ] && echo "FOUND: $f"
   done
   ```

2. **提取直接依赖**（按语言）：
   - Go: `go.mod` require 块（排除 `// indirect`）
   - Node: `package.json` 的 `dependencies`
   - Python: `requirements.txt` 或 `pyproject.toml` 的 `[project.dependencies]`
   - Rust: `Cargo.toml` 的 `[dependencies]`
   - Java: `pom.xml` 或 `build.gradle` 的 implementation 依赖

3. **优先级筛选**（最多 10 个库）：
   - P0 框架核心：Web 框架、ORM、核心运行时
   - P1 安全相关：认证库、加密库、JWT 库
   - P2 高频使用：import 次数最多的库
   - P3 其余依赖

4. **批量查询 Context7**：
   ```
   对每个库:
     id = resolve-library-id(libraryName)
     如果失败 → 记录到 c7_failures, 跳过
     docs = get-library-docs(id, topic="核心 API 概览", tokens=5000)
     缓存到 C7 知识缓存
     queries_remaining[库名] = 2
   ```

5. **构建缓存 JSON**：
   ```json
   {
     "session_id": "cr-20260207-143000-a1b2c3d4",
     "libraries": {
       "gin": {
         "context7_id": "/gin-gonic/gin",
         "docs_summary": "...(API 摘要)...",
         "key_apis": ["gin.Context", "gin.Engine"],
         "tokens_used": 5000
       REDACTED
     REDACTED,
     "queries_remaining": { "gin": 2 REDACTED,
     "c7_failures": []
   REDACTED
   ```

> 多个 `resolve-library-id` 可并行调用。

### Layer 2: In-Review Realtime Verification (Phase 2)

子 Agent 审核代码时的实时验证规则：

**必须验证的场景**：
1. 认为某个 API 调用方式错误 → 查 C7 确认当前版本签名
2. 认为某个 API 已废弃 → 查 C7 确认 deprecated 状态
3. 认为代码缺少某库提供的安全/性能特性 → 查 C7 确认该特性存在
4. 认为代码写法不兼容某版本 → 查 C7 拉取对应版本文档

**查询优先级**：
1. 先查 C7 知识缓存（Phase 0.5 预热结果）
2. 缓存未命中 → 调用 `get-library-docs(id, topic="{具体 API 名REDACTED")`
3. 遵守每库 3 次查询上限

**标注字段**：
```json
{
  "c7_verified": true,
  "c7_source": "gin.Context.JSON() accepts int status code and any interface{REDACTED",
  "verification_method": "c7_cache"
REDACTED
```

`verification_method` 取值：
- `c7_cache` — 从预热缓存验证
- `c7_realtime` — 实时调用 Context7 验证
- `model_knowledge` — 未使用 Context7（置信度自动降一级）

### Layer 3: Post-Review Cross-Validation (Phase 3)

主 Agent 汇总时的最终验证：

```
对于每个 finding:
  如果 c7_verified == false 且 severity in [critical, high]:
    如果涉及第三方库 API:
      docs = get-library-docs(libraryID, topic="{相关 APIREDACTED")
      如果文档支持 Agent 判断 → c7_verified = true, 保留
      如果文档与 Agent 矛盾 → 降级为 info 或删除, 标记 c7_invalidated
      如果 Context7 无数据 → 保留, 标注 unverifiable
    否则 (纯逻辑问题):
      跳过 C7 验证, 保持原判断
```

**强制规则**：`verification_method == "model_knowledge"` 的 critical/high API 相关发现，未完成交叉验证则自动降级为 medium。

## Degradation Strategy

| 场景 | 行为 |
|------|------|
| Context7 MCP 未配置 | 跳过所有 C7 阶段，报告标注 NONE 覆盖度 |
| 网络超时 | 重试 1 次，仍失败则跳过该库 |
| `resolve-library-id` 失败 | 记录到 `c7_failures`，跳过该库 |
| 查询配额耗尽 | 使用已缓存的最佳信息 |
| 子 Agent 中 C7 调用失败 | 标注 `verification_method: "model_knowledge"`，降低置信度 |

## Report Section: Verification Statistics

审核报告中包含的 Context7 统计节：

| 指标 | 说明 |
|------|------|
| 检测到的依赖库总数 | 项目直接依赖数 |
| C7 成功解析的库 | resolve-library-id 成功数 |
| C7 解析失败的库 | 失败列表 |
| Pre-Review 查询次数 | Phase 0.5 的 get-library-docs 调用数 |
| In-Review 查询次数 | Phase 2 子 Agent 的实时查询总数 |
| Post-Review 查询次数 | Phase 3 交叉验证查询数 |
| C7 验证通过的发现数 | c7_verified == true |
| C7 纠正的误判数 | c7_invalidated 标记数 |
| 验证覆盖度评级 | FULL / PARTIAL / LIMITED / NONE |

## Anti-Hallucination Corrections Table

报告中记录被 Context7 纠正的误判：

| # | Agent | 原 Severity | 原 Title | 纠正原因 | C7 Source |
|---|-------|------------|---------|---------|-----------|
| 1 | Security | high | API deprecated | C7 文档显示该 API 在 v2.x 中仍为 stable | /lib/docs... |
