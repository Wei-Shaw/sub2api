## Context

到此为止，浮窗已经能流式回答问题，但"知识"完全来自一个 admin 写的 textarea。这次的任务是：把"知识"做成可结构化检索的两类数据源（FAQ 条目 + 抓取的文档片段），用 pgvector 做向量检索，把 top-K 注入 LLM。技术栈上 PG 已确认支持 pgvector（用户已确认 5: Y），项目自身就有 `/v1/embeddings` 端点可用，cron 调度系统按项目实际情况要么复用要么用最小 ticker。

约束：
- 必须可单独 toggle 关闭：`support_chat_rag_enabled = false` 时退回到 `add-support-chat-widget` 的原 prompt 行为（safety net）。
- 单次对话**额外**消耗一次 embedding 调用（user message → vector）—— 计费上没有新路径，仍归客服 key。
- `doc_url` 抓取深度固定 1（同域名），admin 可改但默认 1。
- 切片策略 h1/h2/h3 标题边界优先；单片字符上限 2000；overlap 80。
- embedding 模型默认 `text-embedding-3-small`（1536 维），向量列也按 1536 定义；改大模型需要重建表（M1 不支持）。
- 抓取频率每天 03:00 一次，admin 可手动触发，doc_url 变更不自动触发（防误操作）。
- top-K 默认 5；admin 可配 1..20。
- FAQ 由 admin 在后台 CRUD 维护，每条保存时同步生成 embedding。

## Goals / Non-Goals

**Goals：**

- 让客服浮窗的 LLM 在每次对话时能结合"当前用户问题"实际从平台知识里检索片段作答。
- 让 admin 在后台能完整地：维护 FAQ 条目（CRUD）、设置抓取的 doc_url、查看抓取状态（chunk 数 / 上次时间 / 错误）、手动触发重建。
- 让 RAG 能力 **可关闭**：默认关、admin 显式开；关闭时浮窗降级到 chat-widget change 的原行为，无任何中断。
- embedding 与 LLM 共用同一把客服 API key，所有 token 消耗归到 owner 账户（一致性）。

**Non-Goals：**

- 不做多文档源（只支持一个 `doc_url`，跨域名抓取不支持）。
- 不做交互式索引（admin 上传 PDF / md 文件）—— M1 暂用 doc_url 抓取；以后单独 propose。
- 不做 reranker（cross-encoder 二阶段重排）—— top-K 直出。
- 不做 chunk-level admin UI（admin 看不到 chunk 列表，只看到聚合状态）。
- 不做 embedding 模型切换（首版固定 1536 维 `text-embedding-3-small`）。
- 不做向量缓存（同样问题反复 embed 浪费 —— 但 admin 客服一天对话量小，节省的钱不值得引入缓存复杂度）。
- 不做 admin 端"参考来源审计"工具（被 RAG 命中的某条 chunk 来自哪条 doc URL 的什么位置）—— M1 在响应里夹 source_url 即可，admin 看 chunk 表 SQL 自查。

## Decisions

### D1. 表 schema

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE support_faq_items (
  id          BIGSERIAL PRIMARY KEY,
  question    VARCHAR(200) NOT NULL,
  answer      TEXT NOT NULL,                        -- markdown
  tags        TEXT[]   NOT NULL DEFAULT '{}',
  enabled     BOOLEAN  NOT NULL DEFAULT true,
  sort_order  INTEGER  NOT NULL DEFAULT 0,
  embedding   VECTOR(1536),                          -- nullable: 计算失败时为 NULL,
                                                     --           会被检索时跳过
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX faq_embedding_idx
  ON support_faq_items USING ivfflat (embedding vector_cosine_ops)
  WITH (lists = 100);

CREATE TABLE support_doc_chunks (
  id            BIGSERIAL PRIMARY KEY,
  source_url    VARCHAR(500) NOT NULL,
  chunk_text    TEXT NOT NULL,
  content_hash  CHAR(64) NOT NULL,                    -- sha256(chunk_text)
  embedding     VECTOR(1536),
  fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (source_url, content_hash)                   -- dedupe
);

CREATE INDEX doc_embedding_idx
  ON support_doc_chunks USING ivfflat (embedding vector_cosine_ops)
  WITH (lists = 100);
```

**理由**：
- `embedding nullable`：embed 调用失败不阻塞 row 写入；检索时 `WHERE embedding IS NOT NULL`。
- `IVFFlat / lists=100` 起步：M1 数据量预计 < 1 万 chunks，IVFFlat 足够；超过 10 万后再考虑 HNSW。
- `UNIQUE(source_url, content_hash)` —— 文档没变就 SKIP，节省 embedding 成本。
- ent schema 用 PG 自定义类型支持向量列（ent 有 dialect-specific column 配置）。

### D2. embedding service：内部直调，不走 HTTP 自调

```go
type EmbeddingService interface {
    Embed(ctx, text) ([]float32, error)
    EmbedBatch(ctx, texts []string) ([][]float32, error)
}
```

实现：
- 拿 `support_chat_api_key_id` 的明文 token；
- 直接调本进程的 OpenAI embeddings handler 的内部函数（`internal/handler/openai_embeddings.go` 已存在）；
- 错误传播到上层；上层根据上下文决定降级（FAQ 保存仍成功、chat 不带 RAG 段）。

**为什么不走 HTTP 自调**：性能 + 不污染 access log + 避免循环鉴权。

### D3. 文档抓取与切片管线

```
SupportDocPipeline.Run(ctx, opts):
  1. 校验：support_chat_rag_doc_url 非空，否则记 status="empty_doc_url" 退出
  2. fetch(doc_url)
       - User-Agent: "Sub2APIDocBot/1.0"
       - timeout 30s
       - 状态码 4xx/5xx → 记 status="fetch_failed" 退出
  3. parseHTML → extractMainContent (启发式: 优先 <main>, 退化到 <body> 去除 script/style/nav/footer)
  4. 转 Markdown (用 html-to-markdown 库或自己写最小子集)
  5. 抽链接 (深度 1):
       - 若 depth >= 1: 收集同域名下所有 <a href> → 去重 → 加入 todo 列表
       - depth = 0 时跳过 (本期默认 1)
       - 上限 50 页 (硬编码安全阀; admin 不可配)
  6. for each url in [doc_url, ...links]:
       a. fetch + parse + 转 markdown
       b. 按 markdown headers (h1/h2/h3) 切段
       c. 每段长度 > chunk_size 时按字数硬切, overlap = chunk_overlap
       d. 每段长度 < 50 时 SKIP (太短没语义)
  7. 计算 sha256 hash; SELECT existing → 如果 (source_url, hash) 已存在 SKIP
  8. embed 新 chunks (批量 100 条/批)
  9. upsert 到 support_doc_chunks (transaction)
  10. 删除 source_url 上"已不存在于本次抓取"的旧 chunks (clean-up)
  11. 写状态: chunks_total / chunks_added / chunks_removed / errors / fetched_at
```

**关键决策**：
- **抓取上限 50 页硬编码**——防止 admin 设了一个文档站根 URL 把整站 (10,000 页) 拖一遍把 embedding 配额薅光。
- **chunk min length 50**——避免 toc/页脚等噪声成 chunk。
- **失败容忍**：单页失败不中断全流程；最终状态报告每页成败。
- **clean-up**：旧 chunks 显式 DELETE，避免文档删页后残留。

### D4. 调度

- 项目里有现成 cron / scheduler 框架（如果有：复用；如果没有：起一个 `time.Ticker` 在每天最近的 03:00 触发）。
- admin 配置 `daily-03 / weekly / manual`：
  - `daily-03`：每天 03:00。
  - `weekly`：每周一 03:00。
  - `manual`：cron 不跑，仅 admin 后台按钮触发。
- 同一时刻只允许一个 pipeline 实例运行：用 PG advisory lock (`pg_try_advisory_lock(...)`) 或 Redis SETNX；选 PG advisory lock（不引入新依赖）。
- 异步触发：admin 点"立即重建" → API 启动 goroutine + advisory lock + 立即返回 `{accepted: true}`；状态由 `GET /admin/support/doc-index/status` 查询。

### D5. RAG 检索 SQL

```sql
WITH embedded AS (SELECT $1::vector AS q)
SELECT 'faq' AS source, id, question AS title, answer AS body,
       NULL  AS source_url, 1 - (embedding <=> q) AS score
  FROM support_faq_items, embedded
  WHERE enabled = true AND embedding IS NOT NULL
UNION ALL
SELECT 'doc' AS source, id, NULL AS title, chunk_text AS body,
       source_url, 1 - (embedding <=> q) AS score
  FROM support_doc_chunks, embedded
  WHERE embedding IS NOT NULL
ORDER BY score DESC
LIMIT $2;
```

`<=>` 是 pgvector 的 cosine 距离；`1 - distance = similarity`。

**理由**：UNION ALL 一次拉两个表的 top-K 候选，按 score 排序后取最终 K。FAQ 与 doc 不分桶，让 LLM 看到一个混合的"相关知识"列表。

### D6. RAG 注入 prompt 的格式

```
<admin support_chat_system_prompt>

## 相关知识

[FAQ] Q: 怎么充值？
      A: 进入 /payment 页面...

[DOC] (来源: https://docs.example/api-keys)
api key 创建步骤：1. 登录后...

...

---

[Platform safety rules]
你是 {{site_name}} 的客服助手...
```

**关键**：
- "## 相关知识" 用 markdown 二级标题，让 LLM 明确这是"参考材料"段。
- 每条 chunk 前带 `[FAQ]` / `[DOC]` 标签让 LLM 知道权威性差别（FAQ 是 admin 写的，DOC 是抓的）。
- 末尾的 safety footer 仍是最后输入 —— 优先级最高（D14 of chat-widget design 同款）。
- 当检索结果为空时，不渲染 `## 相关知识` 段（避免 LLM 困惑）。

### D7. RAG 守卫：可关闭

```
chat handler 拼装 prompt 流程:

  if support_chat_rag_enabled == true:
    embed user_message
    detect: top-K (cosine score >= 0.3 阈值的)
    if not empty:
      prompt = admin_prompt + "## 相关知识\n" + format(chunks) + "\n---\n" + safety_footer
    else:
      prompt = admin_prompt + safety_footer
  else:
    prompt = admin_prompt + safety_footer  // 退回 chat-widget 原行为
```

**阈值 0.3**：cosine similarity ≥ 0.3 才注入；低于阈值视作"没有相关知识"（不要给 LLM 喂噪声）。

**embedding 失败容忍**：embed user_message 调用失败 → 跳过 RAG 注入，按"无 RAG"路径继续 LLM 调用；浮窗对用户透明（顶多回答没那么准）。

### D8. FAQ 数据迁移

`add-support-chat-widget` 把 FAQ 存在 `support_chat_faqs` JSON setting 里。本 change 引入表后：

```
启动时迁移逻辑（幂等）:
  if support_faq_items 表为空 AND support_chat_faqs setting 非空:
    for each entry in setting:
      INSERT INTO support_faq_items (question, answer, sort_order, enabled)
        VALUES (...)
      // embedding 字段先留空，由 admin 触发重新计算 / 后台异步补
    异步触发：批量 embed 这些新行
  保留 support_chat_faqs setting key 但不再读它（admin 后台改写新 API）
```

**理由**：保留 setting 用于回滚——万一新表出问题，关掉 RAG 后浮窗仍能从 setting 读 FAQ（chat-widget 老路径）。归档 6 个月后再彻底删 setting。

### D9. Admin UI 在 SettingsView 的 supportChat tab 内组织

```
supportChat tab:
  ├ 总开关（chat-widget change 已加）
  ├ 外观 / 显示范围（chat-widget change 已加）
  ├ LLM 配置（chat-widget change 已加）
  ├ 限流（chat-widget change 已加）
  ├ FAQ 管理 ← 本 change 重写：从 inline JSON 数组编辑器 升级为
  │            "list + add/edit modal"，调用 admin FAQ API
  └ RAG（本 change 新增）
      ├ ☐ 启用 RAG (support_chat_rag_enabled)
      ├ doc_url
      ├ 抓取深度 (0/1/2)
      ├ 抓取频率 (daily-03 / weekly / manual)
      ├ embedding 模型（只读：text-embedding-3-small）
      ├ top-K (1..20)
      ├ chunk_size (200..2000)
      ├ chunk_overlap (0..500)
      ├ 状态卡片
      │     上次抓取: 2026-06-09 03:00:12
      │     总 chunks: 152
      │     最近一次错误: (无 / "fetch_failed: 502")
      └ [立即重建文档索引] 按钮 (异步触发)
```

### D10. embedding 计费

embedding 调用与 chat 走同一个客服 API key。每次对话产生 1 次 embedding（user message）+ 1 次 chat（流式）。文档抓取每次重建产生 N 次 batch embedding（按 chunk 数）。所有消耗记到客服 key owner 账户。

**预估成本**：`text-embedding-3-small` ≈ $0.02/1M tokens。一篇文档站 2000 chunks × 平均 500 tokens = 1M tokens ≈ $0.02。每次对话 1 次 embed × 平均 100 tokens ≈ $0.000002。可忽略。

## Risks / Trade-offs

- **[pgvector 扩展未安装]** → 启动时 `CREATE EXTENSION IF NOT EXISTS vector` 会失败，进而表创建失败，进而服务起不来。Mitigation：启动时把这个迁移做"软失败"：CREATE EXTENSION 失败 → 记 fatal log 但**不**panic；进而本 change 的所有 admin / chat 路径在 `support_chat_rag_enabled = true` 时返回明确错误"pgvector unavailable, contact admin"。这样老用户升级时如果忘了装扩展，最多 RAG 不能用，其它功能不受影响。
- **[doc 抓取被反爬虫拦截]** → User-Agent 设成自定义 + 不并发 + 30s 超时；如果还是被拦，admin 看到 status 错误即可。Mitigation：状态卡片明确显示"fetch_failed: 403"，admin 自己处理（白名单/换 URL）。
- **[运营在 doc_url 站维护期间触发抓取]** → 抓到一堆 5xx 错误页面被 embed → 污染知识库。Mitigation：pipeline 步骤 6.a 检查 HTTP status 200 才进入切片；非 200 SKIP 该 URL。
- **[抓取深度 1 太大或太小]** → 太大：嵌入成本爆炸；太小：盖不全。Mitigation：硬编码 50 页上限作为安全阀；admin 看到 status 显示"达到上限 50 页"会知道需要降深度或选另一个入口 URL。
- **[FAQ embedding 计算失败]** → admin 保存 FAQ 时 embedding 失败 → row 写入但 embedding=NULL。Mitigation：FAQ 列表 admin 端显示"未索引"badge 在该行；admin 可点"重新索引"按钮触发再算。同时检索 SQL 里 `WHERE embedding IS NOT NULL` 自然跳过。
- **[向量索引 IVFFlat 在小数据集上不一定比顺序扫快]** → < 1000 行时 IVFFlat 预热成本不划算。Mitigation：M1 不调优；后期如有 perf 报告再换 HNSW 或调 lists 参数。
- **[FAQ 数据迁移失败一半]** → 新表写一半挂了。Mitigation：迁移逻辑用事务包裹 + 幂等检查（"表为空"作为前置条件，避免重复迁移）。
- **[`support_chat_rag_enabled = false` 时仍然每次对话 embed 一次]** → 浪费成本。Mitigation：handler 在守卫为 false 时**不**调 embed（D7 中 if 分支已确保）。
- **[chunk_text 太长被 LLM context 截断]** → max_request_tokens 守卫已在 chat-widget change 实现；RAG section 注入后如果超 cap 会被截掉部分 chunks。Mitigation：D6 的 prompt 拼装顺序保证 RAG section 在 admin prompt 之后、safety footer 之前；超 cap 时优先 drop RAG（chat handler 对 prompt 段落实施大小预算：chunks 总长度上限 = max_request_tokens × 0.5）。

## Migration Plan

1. **依赖**：必须先合 `add-support-chat-widget`（提供 chat handler、settings 框架、客服 key）。
2. **预检**：在测试环境执行 `CREATE EXTENSION IF NOT EXISTS vector;` 确认 PG 实例支持；如果是新部署，部署文档同步要求。
3. **后端**：
   - schema 迁移：`CREATE EXTENSION` + 两张新表 + 索引；
   - embedding service / pipeline / cron / handlers 上线；
   - 默认 `support_chat_rag_enabled = false` —— 全链路保持沉默不影响现网。
4. **前端**：admin 设置 tab 增加 RAG 分组与新 FAQ list/modal；老的 inline JSON 编辑器删除。
5. **首次启用**：
   - admin 进入 supportChat → RAG 分组：(a) 填 doc_url (b) 点"立即重建" (c) 等 1-5 分钟看状态卡片报 "chunks=N, errors=0" (d) 勾"启用 RAG" (e) 调一次浮窗验证。
6. **回滚**：
   - 关 `support_chat_rag_enabled` → 浮窗回到 chat-widget 原行为；
   - 数据保留无害；
   - 真要彻底回滚代码：revert PR；两张表保留（无害）；FAQ 数据由迁移逻辑确保 setting 仍是兜底，可继续读。

## Open Questions

- **是否在 assistant 消息底部显示"参考来源"链接？**——倾向：M1 做。RAG section 的 `[DOC]` 条目里的 `source_url` 在响应 metadata 里带回；前端 panel 在该 assistant 消息底部用小字 "参考: docs.example/keys, docs.example/billing"。如时间紧可省。
- **chunk_size 的最佳值？**——查看 RAG 实践：500..1000 字符是 sweet spot。默认 800 + overlap 80。后续可让 admin 在小范围内微调，M1 不开放无限调。
- **embedding 模型升级路径？**——M1 写死 1536 维；如果 OpenAI 推 3-large（3072 维），需要 ALTER TABLE + 重新 embed 全部数据。这是个有意识的债，M1 不偿。
- **是否要对 chat 端点 user message 做"是否值得 RAG 检索"的预判？**——比如纯寒暄"你好"也每次都 embed 浪费。M1 不做（embed 太便宜）；如果以后要做，加一个 simple length / keyword filter。
- **doc_chunks 老数据清理？**——admin 改 doc_url 后旧域名的 chunks 没人删。Mitigation：pipeline 启动时若 doc_url 域名与历史 chunks 域名不一致 → 提示 admin 是否清理（M1 半自动：admin 后台加一个 "清理孤儿 chunks" 按钮）。
