## Why

`add-support-chat-widget` 已经让客服浮窗的 LLM 可以通过 admin 配置的 system prompt 回答问题，但这个 prompt 是一个静态 textarea —— 内容长了塞不下、改了得 admin 手动改、回答时 LLM 不知道平台具体的 API 错误码 / 充值规则 / 文档锚点。这次给浮窗装一个真正的"脑子"：用 pgvector 向量检索，从 admin 维护的 FAQ 条目 + doc_url 抓取来的文档片段里找 top-K 相关片段，注入到 system prompt 里再让 LLM 作答。

这是 **客服三件套** 的最后一个 change，把 `add-support-chat-widget` 中预留的 `{{rag_context}}` 占位符**激活**——浮窗不变，admin 配置面板扩展，但每次对话的"知识"质量从静态 prompt 升到动态检索。

## What Changes

- **新增** `support_faq_items` 表（admin 手维护的结构化 FAQ）：`id / question / answer / tags[] / enabled / sort_order / embedding (vector(1536)) / created_at / updated_at`。
- **新增** `support_doc_chunks` 表（doc_url 抓来的文档片段）：`id / source_url / chunk_text / embedding (vector(1536)) / fetched_at / content_hash / created_at`。
- **启用 PostgreSQL pgvector 扩展**：`CREATE EXTENSION IF NOT EXISTS vector` 加入项目启动迁移；两张表上的 `embedding` 列建 IVFFlat / HNSW 索引（按数据量选 IVFFlat 默认）。
- **新增** embedding service：`internal/service/embedding_service.go`，封装"文本 → 向量"调用，走平台自身的 `/v1/embeddings` 端点（已有），用 admin 配置的同一个客服 API key（与 chat 共用，避免再开一项）。
- **新增** doc 抓取与切片管线：`internal/service/support_doc_pipeline.go`：抓 `doc_url`（深度 1，同域名）→ 解析 HTML → 提取主体（去 nav/footer/script）→ 按 markdown h1/h2/h3 边界切片（每片 200..2000 字符，跨片 50 字 overlap）→ 计算 sha256 content_hash 去重 → embed → upsert 到 `support_doc_chunks`。
- **新增** 调度 / 触发：(a) 每天凌晨 03:00 cron job 增量重建（按 content_hash 跳过未变片段）；(b) admin 后台 "立即重建文档索引" 按钮触发同步 / 异步任务；(c) admin 修改 `doc_url` 设置时**不**自动重建（避免误操作；需手动点）。
- **新增** admin 端 FAQ CRUD API：`GET/POST/PUT/DELETE /api/admin/support/faqs`；保存时同步触发该条 FAQ 的 embedding 更新（不需要 cron）。
- **新增** admin 端文档索引管理 API：`POST /api/admin/support/doc-index/rebuild`（异步触发） + `GET /api/admin/support/doc-index/status`（查看上次抓取时间、chunk 数、错误信息）。
- **修改** chat handler：原本拼装 `<admin prompt> + <safety footer>` 的逻辑扩展为 `<admin prompt> + <RAG section> + <safety footer>`；RAG section 来自：用最新 user 消息 embed → 在 `support_faq_items.embedding` ∪ `support_doc_chunks.embedding` 上做 cosine similarity top-K 检索（K = `support_chat_rag_top_k`，默认 5）→ 按 `score DESC` 拼成"## 相关知识\n\n[FAQ] Q: ... A: ...\n\n[DOC] {chunk_text}\n\n..."注入。
- **修改** Chat 限流：每次对话额外消耗一次 embedding 请求（user message → vector），需要在限流文档中注明（不新增独立限流）。
- **替换** `add-support-chat-widget` 引入的 `support_chat_faqs` JSON setting：迁移到新的 `support_faq_items` 表里（M1 时 admin FAQ 数据可能为空，迁移即"读 JSON → INSERT 多行 + 触发 embedding"，老 setting key 保留向后兼容但 admin 后台改为读写表）。
- **新增** admin 端 RAG 配置（`support_chat` 设置 tab 内追加 RAG 子分组）：
  - `support_chat_rag_enabled` (bool, default false)
  - `support_chat_rag_doc_url` (string, default "" — 从现有 `doc_url` 设置回退)
  - `support_chat_rag_doc_depth` (enum: 0 / 1 / 2，default 1)
  - `support_chat_rag_doc_cron` (enum: `daily-03` / `weekly` / `manual`，default `daily-03`)
  - `support_chat_rag_embed_model` (string, default `text-embedding-3-small`)
  - `support_chat_rag_top_k` (int, default 5, range 1..20)
  - `support_chat_rag_chunk_size` (int, default 800 chars, range 200..2000)
  - `support_chat_rag_chunk_overlap` (int, default 80 chars, range 0..500)
- **i18n**：扩展 `admin.settings.supportChat.rag.*` 与 `admin.tickets.faqs.*`（如果选择 FAQ 走独立 admin 子页面）。

## Capabilities

### New Capabilities

- `support-knowledge-rag`: FAQ 表 + 文档片段表的 schema 与 CRUD、embedding 生成的协议、文档抓取/切片管线、定时调度、向量检索 top-K 的语义、"知识来源"在 admin 后台的可视化（chunk 数 / 上次抓取时间 / 错误日志）。

### Modified Capabilities

- `support-chat`: chat 端点的 system prompt 拼装从"admin prompt + safety footer"扩展为"admin prompt + RAG context + safety footer"；新增 `support_chat_rag_enabled` 守卫——为 false 时维持原有静态 prompt 行为（确保本 change 可单独 toggle 关闭、回到 chat-widget 行为）。`support_chat_faqs` JSON setting 由"权威源"降级为"读取的兼容兜底"，权威源切换到 `support_faq_items` 表。

## Impact

- **后端**：
  - **新表**：`support_faq_items`、`support_doc_chunks`，对应 ent schema + 自动迁移；启动时执行 `CREATE EXTENSION IF NOT EXISTS vector`。
  - **向量索引**：每张表 `embedding` 列上 `CREATE INDEX ... USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)`（IVFFlat 用 cosine；后续数据量大可换 HNSW）。
  - `internal/service/embedding_service.go` 新增 —— 单条 / 批量 embed 接口；自动按 admin 客服 key 鉴权调本进程内 `/v1/embeddings`。
  - `internal/service/support_doc_pipeline.go` 新增 —— 抓取 + 切片 + diff + embed + upsert，作为可单测单元。
  - `internal/repository/support_faq_repo.go` + `support_doc_chunk_repo.go` 新增。
  - `internal/handler/admin_support_faq_handler.go` + `admin_support_doc_index_handler.go` 新增（admin CRUD + 索引管理）。
  - `internal/handler/support_chat_handler.go` **修改**：注入 RAG section（受 `support_chat_rag_enabled` 守卫；为 false 时走 chat-widget change 的原 prompt 路径）。
  - `internal/cron/support_doc_indexer.go` 新增（或挂在现有 cron 系统下，复用项目调度器）—— 03:00 触发 doc pipeline。
  - `internal/server/routes/support.go` 追加 admin 路由：`/api/admin/support/faqs/*`、`/api/admin/support/doc-index/*`。
- **数据库迁移**：
  - `CREATE EXTENSION vector`（部署清单 / Helm chart 同步要求 PG 实例预装 pgvector，已确认可用）。
  - 两张新表 + 索引。
  - 原 `support_chat_faqs` setting 数据自动迁入 `support_faq_items`（启动时一次性 migration，幂等：检查表为空 + setting 非空才迁）。
- **前端**：
  - admin 设置页 `supportChat` tab 内 FAQ 子组件**重写**：从"内联 JSON 数组编辑"升级为"独立 list 视图 + 添加/编辑 modal"，调用 admin FAQ API；保留旧字段读，但保存路径切到新 API。
  - admin 设置页 `supportChat` tab 内追加 **RAG 分组**：开关、doc_url、深度、调度、embedding 模型、top_k、chunk_size、chunk_overlap、状态卡片（chunk 数 / 上次抓取 / 错误）+ "立即重建" 按钮（调 `POST /api/admin/support/doc-index/rebuild`）。
  - 浮窗本身**不改**（RAG 完全在后端透明发生）；UI 只在 assistant 消息底部新增可选的"参考来源"小字（如果 RAG section 检索到了，把 source_url 列出来）—— M1 可省，作为 nice-to-have。
- **不影响**：
  - 工单系统 / 浮窗 UI / 限流策略 / 计费（embedding 计费同样归到客服 key）；
  - footer `HomeContactSection` 等所有上述 capability 之外的功能。
- **测试**：
  - 后端：embedding service 单测（mock 上游）；doc pipeline 单测（mock fetch + 静态 HTML，断言切片边界、content_hash 去重、embedding 调用次数）；chat handler RAG 注入单测（mock 检索结果，断言 system prompt 含"## 相关知识"段）；admin handler 单测；cron 单测（手工 trigger）。
  - 数据库：integration test 跑真 PG（CI 要求 pgvector），断言 `<->` cosine 排序正确。
  - 前端：admin FAQ 列表 + 编辑 modal spec；RAG 状态卡片 spec；"立即重建"按钮 disabled 在 doc_url 为空时。
- **运维**：
  - 部署文档更新：PG 实例需 pgvector 扩展；首次启动会执行 `CREATE EXTENSION`（需 SUPERUSER 权限或预装）。
  - cron 调度：项目已有调度框架时复用；没有的话新建一个最小调度器（`time.Ticker` 起一个 goroutine 即可，03:00 用 `time.Until(next03)`）。
