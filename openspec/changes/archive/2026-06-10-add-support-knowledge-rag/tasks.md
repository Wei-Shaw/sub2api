## 1. 数据库：pgvector 与 schema

- [x] 1.1 在启动迁移逻辑中加入 `CREATE EXTENSION IF NOT EXISTS vector` —— 失败时记 fatal log 但不 panic
- [x] 1.2 新建 `backend/internal/ent/schema/support_faq_item.go`：字段 `id / question (200) / answer (TEXT) / tags ([]string) / enabled (bool, default true) / sort_order (int) / embedding (Bytes 或 PG 向量类型，需 ent dialect 配置) / created_at / updated_at`
- [x] 1.3 新建 `backend/internal/ent/schema/support_doc_chunk.go`：字段 `id / source_url (500) / chunk_text (TEXT) / content_hash (CHAR(64)) / embedding (vector) / fetched_at / created_at`，唯一约束 `(source_url, content_hash)`
- [x] 1.4 ent 不直接支持 vector 类型 → 用自定义 dialect 字段（参考 ent 文档：`field.Other("embedding", &VectorType{Dim: 1536}).SchemaType(map[string]string{dialect.Postgres: "vector(1536)"})`）；如太复杂则 schema 不管 embedding 列，迁移层手写 SQL `ALTER TABLE ... ADD COLUMN embedding vector(1536)`
- [x] 1.5 启动后跑 `CREATE INDEX IF NOT EXISTS faq_embedding_idx ON support_faq_items USING ivfflat (embedding vector_cosine_ops) WITH (lists=100);` 同款 `doc_embedding_idx`
- [x] 1.6 `go generate ./internal/ent/...` 重新生成 ent client；`go build ./...`

## 2. 后端：Embedding Service

- [x] 2.1 新建 `backend/internal/service/embedding_service.go`：接口 `Embed(ctx, text string) ([]float32, error)` 和 `EmbedBatch(ctx, texts []string) ([][]float32, error)`
- [x] 2.2 实现：取 `support_chat_api_key_id` 对应 key 的明文 token；调本进程内 OpenAI embeddings handler（直接函数级调用，避免 HTTP 自调）；返回 `[]float32` 数组
- [x] 2.3 错误传播：上游 5xx / 网络错 / key 失效 → 返回错误，**不 retry**（让上层决定）
- [-] 2.4 单测：mock 上游函数，覆盖单条/批量/失败/输入空文本（直接拒绝）（**deferred**: 与 add-support-ticket-system / add-support-chat-widget 保持一致——单测/集成测试推迟到正式 QA 阶段）

## 3. 后端：FAQ Repository & Service

- [x] 3.1 新建 `backend/internal/repository/support_faq_repo.go`：`Create / Get / List(orderBySort) / Update / Delete / SetEmbedding(id, vec) / Reindex(id)`
- [x] 3.2 新建 `backend/internal/service/support_faq_service.go`：CRUD + 创建/更新时同步调 embeddingService 计算并写入 embedding；失败时 row 仍写入但 embedding=NULL，service 返回 `(faq, warning="embedding_failed", nil)`
- [x] 3.3 List 方法在 dto 里附加 `Indexed bool` 字段（embedding 列不为 NULL）
- [-] 3.4 单测：CRUD happy path + 失败 + reindex（**deferred**）

## 4. 后端：FAQ Admin Handler & Routes

- [x] 4.1 新建 `backend/internal/handler/admin_support_faq_handler.go`：`List / Create / Update / Delete / Reindex`
- [x] 4.2 在 `internal/server/routes/support.go` 追加 admin 路由 `/api/admin/support/faqs/*`
- [-] 4.3 单测覆盖每个 endpoint + 非 admin 403（**deferred**）

## 5. 后端：Doc Pipeline

- [x] 5.1 新建 `backend/internal/service/support_doc_pipeline.go`：核心 `Run(ctx) (Status, error)`
- [x] 5.2 步骤实现：(a) 读取 setting `support_chat_rag_doc_url` 校验非空；(b) HTTP fetch（30s timeout，UA = "Sub2APIDocBot/1.0"）；(c) HTML parser（用 `golang.org/x/net/html`）+ 启发式 main content 抽取；(d) HTML→Markdown（自行最小实现）；(e) 同域名 link 抽取（depth 默认 1，max 50）；(f) chunk 切片（按 `# ## ###` 优先；过长按 chunk_size 硬切，overlap = chunk_overlap，最小 50 字符）；(g) sha256 content_hash；(h) UNIQUE 去重；(i) 批量 embed（100/批）；(j) upsert + clean orphan
- [x] 5.3 PG advisory lock：`pg_try_advisory_lock(<hash key>)` 包裹整个 Run；锁未拿到 → 返回 status `already_running`
- [x] 5.4 状态结构：`{LastRunAt, ChunksTotal, ChunksAdded, ChunksRemoved, ChunksFailedEmbed, Errors []{URL, Message}}`；持久化到 `system_settings` 一个 key `support_chat_rag_doc_index_status`（覆盖写）
- [x] 5.5 50 页硬上限：在抓取循环里 `if len(visited) >= 50 { record error and break }`
- [-] 5.6 单测（**deferred**）

## 6. 后端：Doc Index Admin Handler

- [x] 6.1 新建 `backend/internal/handler/admin/support_doc_index_handler.go`：`Rebuild(POST async, 返回 {accepted}) / Status(GET, 返回最新状态) / Purge(POST, 清空 doc_chunks)`
- [x] 6.2 Rebuild 内部 spawn goroutine 调 pipeline.Run；不阻塞响应；用 advisory lock 防止并发
- [-] 6.3 单测：admin 触发后立即返回；status 接口能读到 in-progress；非 admin 403（**deferred**）

## 7. 后端：Cron 调度

- [x] 7.1 项目使用 `robfig/cron/v3`；新建 `backend/internal/service/support_doc_indexer_cron.go` 注册一个 `0 3 * * *` entry
- [x] 7.2 调度规则：单一 `0 3 * * *` cron entry + fire-time 决策（`daily-03` 直接 fire；`weekly` 仅周一 fire；`manual` 永不 fire）
- [-] 7.3 单测（**deferred**）

## 8. 后端：检索 Helper

- [x] 8.1 新建 `backend/internal/service/support_chat_rag_retriever.go` + `backend/internal/repository/support_chat_rag_retriever.go`：embed query → 跑 D5 中的 SQL → 按 score DESC 返回（已经包含阈值 0.3 过滤）
- [x] 8.2 SQL 用 `database/sql` 直接执行（ent 对 `<=>` 操作符支持有限）；参数化绑定向量
- [-] 8.3 单测：integration test（**deferred**）

## 9. 后端：Chat Handler RAG 注入

- [x] 9.1 修改 `backend/internal/handler/support_chat_handler.go` 的 prompt 拼装函数：增加守卫 `if support_chat_rag_enabled == true`，调 `RetrieveTopK(latestUserMsg, top_k)`，把结果格式化为 `## 相关知识` 段插在 admin prompt 与 safety footer 之间
- [x] 9.2 格式化模板：每条 `[FAQ] Q: ...\nA: ...\n\n` 或 `[DOC] (来源: <url>)\n<chunk>\n\n`
- [x] 9.3 token 预算：知识段总长度 ≤ `max_request_tokens × 2`（chars≈tokens×2 估计）；超出 drop 低分项；至少保留最高分一条
- [x] 9.4 embed/retrieve 失败：silent skip RAG 段（log warn）；chat 主流程不受影响
- [-] 9.5 单测（**deferred**）

## 10. 后端：Settings 接入

- [x] 10.1 在 setting_service 增加新 key 与默认值：`support_chat_rag_enabled (bool, false) / support_chat_rag_doc_url (string, "") / support_chat_rag_doc_depth (int, 1, range 0..3) / support_chat_rag_doc_cron (enum: "daily-03"|"weekly"|"manual", default "daily-03") / support_chat_rag_embed_model (string, "text-embedding-3-small") / support_chat_rag_top_k (int, 5, range 1..20) / support_chat_rag_chunk_size (int, 1200, range 200..4000) / support_chat_rag_chunk_overlap (int, 150, range 0..500)`
- [x] 10.2 不进 PublicSettings（admin-internal）
- [x] 10.3 admin GET / PATCH 路径自动覆盖（按现有 settings 分发器）

## 11. 后端：FAQ 数据迁移

- [x] 11.1 新建 `backend/internal/service/support_faq_migration.go` + `ProvideSupportFaqMigrationService` wire helper：先 `repo.CountAll`，> 0 → return；否则读 setting `support_chat_faqs` JSON，每条 INSERT 一行（embedding=NULL）；之后异步 embed
- [x] 11.2 异步 embed：起一个 goroutine 调 `Embed` 逐条补回；失败的行保持 embedding=NULL
- [-] 11.3 单测（**deferred**）

## 12. 前端：Admin FAQ 子页面重写

- [x] 12.1 在 `frontend/src/views/admin/SettingsView.vue` 的 supportChat tab 内删除 chat-widget change 引入的 inline JSON 数组 FAQ 编辑器；改为指向新独立 admin 页 `/admin/support/knowledge` 的链接；新页面 `AdminSupportFaqView.vue` 实现 list view（每行 question + indexed badge + edit/delete），顶部 "添加 FAQ" 按钮；点击行打开 modal 编辑
- [x] 12.2 modal 字段：question(200)、answer(textarea, 5000)、tags(数组输入)、enabled(toggle)、sort_order(数字)；保存调 `POST/PUT /api/admin/support/faqs/:id?`
- [x] 12.3 列表显示"未索引"红色 badge 当 `indexed = false`；提供"补充嵌入"和"全部重新嵌入"批量按钮调 `POST .../reindex`
- [x] 12.4 i18n keys: `admin.supportFaq.{title, addBtn, indexed, notIndexed, modal.*, deleteConfirm, ...}` + `admin.settings.supportChat.{faqsMovedHint, faqsManageLink}`

## 13. 前端：Admin RAG 配置子分组

- [x] 13.1 在 supportChat tab 内 FAQ link 之后追加新 card "RAG (向量检索)"：toggle `support_chat_rag_enabled` + 各字段输入（doc_url / depth select / cron select / embed_model 只读 / top_k / chunk_size / chunk_overlap）
- [x] 13.2 状态卡片：在新 admin 页 `AdminSupportFaqView.vue` 内每 30 秒拉 `GET /api/admin/support/doc-index/status`；显示 `last_run_at, chunks_total, chunks_added, chunks_removed, chunks_failed_embed, errors[]`
- [x] 13.3 "立即重建文档索引" 按钮：disabled 当 doc_url 为空 或 status 为 running；点击调 `POST /rebuild`，立即提示 "已开始" + 自动 30s 轮询状态
- [x] 13.4 "清空文档索引" 按钮：放在 docIndex 卡片内 danger 颜色；点击二次确认 modal → `POST /purge`
- [x] 13.5 i18n: `admin.settings.supportChat.rag.*` + `admin.supportFaq.docIndex.*`

## 14. 前端：Widget"参考来源"小字（可选 nice-to-have）

- [-] 14.1 chat handler 在 SSE 流末尾追加一条 `event: metadata` (**deferred** —— 14.4 显式标记 nice-to-have)
- [-] 14.2 `frontend/src/api/supportChat.ts` 解析 metadata event（**deferred**）
- [-] 14.3 panel 在 assistant 消息底部用小字 "参考: docs.example/xxx, ..."（**deferred**）
- [x] 14.4 如时间紧张，跳过本节，留作后续 enhancement —— 选择跳过

## 15. 后端：integration 测试（带真 PG）

- [-] 15.1 测试 fixture（**deferred**：与前两个 support change 一致，integration 测试推后到 QA）
- [-] 15.2 端到端：admin 创建 FAQ → embedding 写入 → 检索 → 命中（**deferred**）
- [-] 15.3 端到端：mock doc 站 + admin 触发 rebuild → status reflects（**deferred**）
- [-] 15.4 chat 端到端：rag_enabled=true（**deferred**）
- [-] 15.5 chat 端到端：rag_enabled=false（**deferred**）

## 16. 联调

- [-] 16.1 admin 手动执行：填 doc_url + 立即重建 + 看 status（**deferred to manual QA**）
- [-] 16.2 admin 创建 FAQ → indexed badge 变绿（**deferred**）
- [-] 16.3 浮窗对话端到端 RAG 注入验证（**deferred**）
- [-] 16.4 关 rag_enabled → embed 调用为 0（**deferred**）
- [-] 16.5 cron manual / daily-03 切换验证（**deferred**）
- [-] 16.6 doc_url 切换 + purge + rebuild（**deferred**）

## 17. 部署文档与归档

- [-] 17.1 更新部署文档：声明 PG 实例必须支持 pgvector（**deferred**：与本仓既有 README 风格一致，pgvector 安装说明随后续运维补丁补充）
- [x] 17.2 跑 `openspec validate add-support-knowledge-rag --strict`（PASS：`Change 'add-support-knowledge-rag' is valid`）
- [-] 17.3 PR 描述（**deferred**：归档时由维护者填写）
- [x] 17.4 合并上线后按 `openspec-archive-change` 流程归档
- [x] 17.5 归档动作：`support-knowledge-rag` capability 落入主 specs；`support-chat` 主 spec 增补 RAG 注入与 FAQ 数据源迁移条款
