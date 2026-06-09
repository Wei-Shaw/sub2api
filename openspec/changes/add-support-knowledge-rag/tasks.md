## 1. 数据库：pgvector 与 schema

- [ ] 1.1 在启动迁移逻辑中加入 `CREATE EXTENSION IF NOT EXISTS vector` —— 失败时记 fatal log 但不 panic
- [ ] 1.2 新建 `backend/internal/ent/schema/support_faq_item.go`：字段 `id / question (200) / answer (TEXT) / tags ([]string) / enabled (bool, default true) / sort_order (int) / embedding (Bytes 或 PG 向量类型，需 ent dialect 配置) / created_at / updated_at`
- [ ] 1.3 新建 `backend/internal/ent/schema/support_doc_chunk.go`：字段 `id / source_url (500) / chunk_text (TEXT) / content_hash (CHAR(64)) / embedding (vector) / fetched_at / created_at`，唯一约束 `(source_url, content_hash)`
- [ ] 1.4 ent 不直接支持 vector 类型 → 用自定义 dialect 字段（参考 ent 文档：`field.Other("embedding", &VectorType{Dim: 1536}).SchemaType(map[string]string{dialect.Postgres: "vector(1536)"})`）；如太复杂则 schema 不管 embedding 列，迁移层手写 SQL `ALTER TABLE ... ADD COLUMN embedding vector(1536)`
- [ ] 1.5 启动后跑 `CREATE INDEX IF NOT EXISTS faq_embedding_idx ON support_faq_items USING ivfflat (embedding vector_cosine_ops) WITH (lists=100);` 同款 `doc_embedding_idx`
- [ ] 1.6 `go generate ./internal/ent/...` 重新生成 ent client；`go build ./...`

## 2. 后端：Embedding Service

- [ ] 2.1 新建 `backend/internal/service/embedding_service.go`：接口 `Embed(ctx, text string) ([]float32, error)` 和 `EmbedBatch(ctx, texts []string) ([][]float32, error)`
- [ ] 2.2 实现：取 `support_chat_api_key_id` 对应 key 的明文 token；调本进程内 OpenAI embeddings handler（直接函数级调用，避免 HTTP 自调）；返回 `[]float32` 数组
- [ ] 2.3 错误传播：上游 5xx / 网络错 / key 失效 → 返回错误，**不 retry**（让上层决定）
- [ ] 2.4 单测：mock 上游函数，覆盖单条/批量/失败/输入空文本（直接拒绝）

## 3. 后端：FAQ Repository & Service

- [ ] 3.1 新建 `backend/internal/repository/support_faq_repo.go`：`Create / Get / List(orderBySort) / Update / Delete / SetEmbedding(id, vec) / Reindex(id)`
- [ ] 3.2 新建 `backend/internal/service/support_faq_service.go`：CRUD + 创建/更新时同步调 embeddingService 计算并写入 embedding；失败时 row 仍写入但 embedding=NULL，service 返回 `(faq, warning="embedding_failed", nil)`
- [ ] 3.3 List 方法在 dto 里附加 `Indexed bool` 字段（embedding 列不为 NULL）
- [ ] 3.4 单测：CRUD happy path + 失败 + reindex

## 4. 后端：FAQ Admin Handler & Routes

- [ ] 4.1 新建 `backend/internal/handler/admin_support_faq_handler.go`：`List / Create / Update / Delete / Reindex`
- [ ] 4.2 在 `internal/server/routes/support.go` 追加 admin 路由 `/api/admin/support/faqs/*`
- [ ] 4.3 单测覆盖每个 endpoint + 非 admin 403

## 5. 后端：Doc Pipeline

- [ ] 5.1 新建 `backend/internal/service/support_doc_pipeline.go`：核心 `Run(ctx) (Status, error)`
- [ ] 5.2 步骤实现：(a) 读取 setting `support_chat_rag_doc_url` 校验非空；(b) HTTP fetch（30s timeout，UA = "Sub2APIDocBot/1.0"）；(c) HTML parser（用 `golang.org/x/net/html` 或 `goquery`）+ 启发式 main content 抽取；(d) HTML→Markdown（用 `github.com/JohannesKaufmann/html-to-markdown` 或自行最小实现）；(e) 同域名 link 抽取（深度 1，max 50）；(f) chunk 切片（按 `# ## ###` 优先；过长按 chunk_size 硬切，overlap = chunk_overlap，最小 50 字符）；(g) sha256 content_hash；(h) UNIQUE 去重；(i) 批量 embed（100/批）；(j) upsert + clean orphan
- [ ] 5.3 PG advisory lock：`pg_try_advisory_lock(<hash key>)` 包裹整个 Run；锁未拿到 → 返回 status `already_running`
- [ ] 5.4 状态结构：`{LastRunAt, ChunksTotal, ChunksAdded, ChunksRemoved, ChunksFailedEmbed, Errors []{URL, Message}}`；持久化到 `system_settings` 一个 key `support_chat_rag_doc_index_status`（覆盖写）
- [ ] 5.5 50 页硬上限：在抓取循环里 `if len(visited) >= 50 { record error and break }`
- [ ] 5.6 单测：mock HTTP server + 静态 HTML，断言 chunk 边界、content_hash 去重（同 HTML 二次 run = 0 added 0 removed）、orphan clean、50 页上限触发、advisory lock 不可重入

## 6. 后端：Doc Index Admin Handler

- [ ] 6.1 新建 `backend/internal/handler/admin_support_doc_index_handler.go`：`Rebuild(POST async, 返回 {accepted}) / Status(GET, 返回最新状态) / Purge(POST, 清空 doc_chunks)`
- [ ] 6.2 Rebuild 内部 spawn goroutine 调 pipeline.Run；不阻塞响应；用 advisory lock 防止并发
- [ ] 6.3 单测：admin 触发后立即返回；status 接口能读到 in-progress；非 admin 403

## 7. 后端：Cron 调度

- [ ] 7.1 检查项目是否已有 cron 框架；如有：注册一个 entry。如无：在 `internal/cron/support_doc_indexer.go` 起一个 ticker goroutine（每分钟检查一次"现在是否触发时间"，触发后调 pipeline）
- [ ] 7.2 调度规则：`daily-03` = 每天 03:00；`weekly` = 每周一 03:00；`manual` = 不调度
- [ ] 7.3 单测：mock 时钟，断言 03:00 触发、03:01 不触发；manual 永不触发

## 8. 后端：检索 Helper

- [ ] 8.1 在 `support_chat_service.go`（chat-widget change 已有）中新增 `RetrieveTopK(ctx, query string, k int) ([]Hit, error)`：embed query → 跑 D5 中的 SQL → 按 score DESC 返回（已经包含阈值 0.3 过滤）
- [ ] 8.2 SQL 用 `database/sql` 直接执行（ent 对 `<=>` 操作符支持有限）；参数化绑定向量
- [ ] 8.3 单测：integration test（带真 PG），插入若干 FAQ + chunks，断言 top-K 顺序与阈值过滤

## 9. 后端：Chat Handler RAG 注入

- [ ] 9.1 修改 `backend/internal/handler/support_chat_handler.go` 的 prompt 拼装函数：增加守卫 `if support_chat_rag_enabled == true`，调 `RetrieveTopK(latestUserMsg, top_k)`，把结果格式化为 `## 相关知识` 段插在 admin prompt 与 safety footer 之间
- [ ] 9.2 格式化模板：每条 `[FAQ] Q: ...\nA: ...\n\n` 或 `[DOC] (来源: <url>)\n<chunk>\n\n`
- [ ] 9.3 token 预算：知识段总长度 ≤ `max_request_tokens × 0.5 × 2`（chars≈tokens×2 估计）；超出 drop 低分项；至少保留最高分一条
- [ ] 9.4 embed/retrieve 失败：silent skip RAG 段（log warn）；chat 主流程不受影响
- [ ] 9.5 单测：覆盖 spec 的 5 个 scenario（enabled+success / enabled+empty / disabled / embedding fail / 预算 truncate）

## 10. 后端：Settings 接入

- [ ] 10.1 在 setting_service 增加新 key 与默认值：`support_chat_rag_enabled (bool, false) / support_chat_rag_doc_url (string, "") / support_chat_rag_doc_depth (int, 1, range 0..2) / support_chat_rag_doc_cron (enum: "daily-03"|"weekly"|"manual", default "daily-03") / support_chat_rag_embed_model (string, "text-embedding-3-small") / support_chat_rag_top_k (int, 5, range 1..20) / support_chat_rag_chunk_size (int, 800, range 200..2000) / support_chat_rag_chunk_overlap (int, 80, range 0..500)`
- [ ] 10.2 不进 PublicSettings（admin-internal）
- [ ] 10.3 admin GET / PATCH 路径自动覆盖（按现有 settings 分发器）

## 11. 后端：FAQ 数据迁移

- [ ] 11.1 在 application 启动钩子（migration after schema ready）增加 `MigrateLegacyFaqs(ctx)`：先 `SELECT count(*) FROM support_faq_items`，> 0 → return；否则读 setting `support_chat_faqs` JSON，每条 INSERT 一行（embedding=NULL）；之后异步 embed
- [ ] 11.2 异步 embed：起一个 goroutine 调 `EmbedBatch` 批量补回；失败的行保持 embedding=NULL
- [ ] 11.3 单测：覆盖三种情况（表非空 → 跳过；表空+setting 空 → 跳过；表空+setting 有 → 迁移）

## 12. 前端：Admin FAQ 子页面重写

- [ ] 12.1 在 `frontend/src/views/admin/SettingsView.vue` 的 supportChat tab 内删除 chat-widget change 引入的 inline JSON 数组 FAQ 编辑器；改为 list view（每行 question + indexed badge + edit/delete），底部 "添加 FAQ" 按钮；点击行打开 modal 编辑
- [ ] 12.2 modal 字段：question(200)、answer(textarea, 5000)、tags(数组输入)、enabled(toggle)、sort_order(数字)；保存调 `POST/PUT /api/admin/support/faqs/:id?`
- [ ] 12.3 列表显示"未索引"红色 badge 当 `indexed = false`；点 row 上的"重新索引"按钮调 `POST .../reindex`
- [ ] 12.4 i18n keys: `admin.settings.supportChat.faqs.{title, addBtn, indexed, notIndexed, reindexBtn, modalTitle, ...}`

## 13. 前端：Admin RAG 配置子分组

- [ ] 13.1 在 supportChat tab 内 FAQ 之后追加新 card "RAG (向量检索)"：toggle `support_chat_rag_enabled` + 各字段输入（doc_url / depth select / cron select / embed_model 只读 / top_k / chunk_size / chunk_overlap）
- [ ] 13.2 状态卡片：每 30 秒拉 `GET /api/admin/support/doc-index/status`；显示 `last_run_at, chunks_total, chunks_added, chunks_removed, chunks_failed_embed, errors[].URL`
- [ ] 13.3 "立即重建文档索引" 按钮：disabled 当 doc_url 为空 或 status 为 in-progress；点击调 `POST /rebuild`，立即提示 "已开始" + 自动加密轮询状态
- [ ] 13.4 "清空文档索引" 按钮：放在 collapsed/danger zone；点击二次确认 modal → `POST /purge`
- [ ] 13.5 i18n: `admin.settings.supportChat.rag.{enabled, docUrl, docDepth, docCron, embedModel, topK, chunkSize, chunkOverlap, statusCard, rebuildBtn, purgeBtn, ...}`

## 14. 前端：Widget"参考来源"小字（可选 nice-to-have）

- [ ] 14.1 chat handler 在 SSE 流末尾追加一条 `event: metadata\ndata: {"sources": [...source_urls]}\n\n`（用最后一次检索的 doc 来源）
- [ ] 14.2 `frontend/src/api/supportChat.ts` 解析 metadata event；store 把 sources 关联到 assistant 消息
- [ ] 14.3 panel 在 assistant 消息底部用小字 "参考: docs.example/xxx, ..."（仅 doc 来源，FAQ 不显示）；hover 显示 tooltip
- [ ] 14.4 如时间紧张，跳过本节，留作后续 enhancement

## 15. 后端：integration 测试（带真 PG）

- [ ] 15.1 测试 fixture：spawn PG with pgvector，跑 schema 迁移
- [ ] 15.2 端到端：admin 创建 FAQ → embedding 写入 → 检索 → 命中
- [ ] 15.3 端到端：mock 一个 doc 站（local HTTP server），admin 触发 rebuild → status reflects → 检索同时命中 doc + FAQ
- [ ] 15.4 chat 端到端：rag_enabled=true + 知识库非空 → 响应中 mock 上游收到的 system message 含 `## 相关知识` 段
- [ ] 15.5 chat 端到端：rag_enabled=false → 不发 embedding 请求 + system message 不含相关知识段

## 16. 联调

- [ ] 16.1 admin 手动执行：填 doc_url（指向项目自身 docs site 或 README）→ 点立即重建 → 等 1 min → 看 status chunks 数 > 0
- [ ] 16.2 admin 创建 1 条 FAQ "怎么充值？" → 看 indexed badge 变绿
- [ ] 16.3 浮窗对话："怎么充值？" → 期望响应里包含来自 FAQ 的具体信息；后端 log 确认 RAG section 注入；UI 看到 metadata 来源（如做了 14）
- [ ] 16.4 关 rag_enabled → 浮窗回 chat-widget 行为（embed 调用计数应为 0）
- [ ] 16.5 改 cron 为 manual → 在 03:00 不应触发；改回 daily-03 验证下次触发
- [ ] 16.6 admin 关 doc_url 改成新域名 → 点 purge 清空旧 chunks → 重建 → 状态卡片刷新

## 17. 部署文档与归档

- [ ] 17.1 更新部署文档：声明 PG 实例必须支持 pgvector；提供 install snippets (Ubuntu apt + Docker image vars)
- [ ] 17.2 跑 `openspec validate add-support-knowledge-rag --strict`
- [ ] 17.3 PR 描述：依赖关系（必须先合 chat-widget）、admin 操作截图（FAQ list / RAG status card）、迁移说明（legacy setting → table）、回滚说明
- [ ] 17.4 合并上线后按 `openspec-archive-change` 流程归档
- [ ] 17.5 归档动作：`support-knowledge-rag` capability 落入主 specs；`support-chat` 主 spec 增补 RAG 注入与 FAQ 数据源迁移条款
