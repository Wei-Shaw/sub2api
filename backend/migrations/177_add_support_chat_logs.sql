-- 客服浮窗对话审计：把用户与客服 LLM 的对话及回包持久化，供管理端只读查看。
--
-- 设计（openspec/changes/add-support-chat-transcript-log/design.md）：
--   - 方案 B：会话头 support_chat_conversations（以浏览器已在发送的 session_id 为业务键）
--     + 逐轮消息 support_chat_messages（1:N）。同一 session_id 的多轮问答归并到一个会话。
--   - 一轮问答 = 两行 message：user 行（无 status）+ assistant 行（带完整状态分类）。
--   - 匿名对话：user_id 可空（ON DELETE SET NULL），只存 client_ip。
--   - status 完整分类：success | upstream_auth | upstream_error | interrupted
--     | rate_limited | config_error。全部落库（含未打到上游就被拦下的）。
--   - content 单条落库前由 service 层截断到 50000 字符（对齐工单 chat_context 上限），
--     DB 不加长度约束，留出手动数据修复空间。
--   - 无独立开关：菜单可见性跟随 support_chat_enabled；admin 路由不卡 feature flag。
--   - 留存：M1 永久保留；未来若需合规清理，另加 retention 定时任务。

-- 会话头
CREATE TABLE IF NOT EXISTS support_chat_conversations (
    id            BIGSERIAL PRIMARY KEY,
    session_id    VARCHAR(128) NOT NULL UNIQUE,          -- 客户端生成的会话键（幂等 upsert 用）
    user_id       BIGINT REFERENCES users(id) ON DELETE SET NULL,  -- 可空：匿名对话无 user_id
    client_ip     VARCHAR(64),
    turn_count    INT NOT NULL DEFAULT 0,                -- 已归并的问答轮数
    last_status   VARCHAR(24),                           -- 最近一轮 assistant 状态
    first_at      TIMESTAMPTZ,                           -- 首轮时间
    last_at       TIMESTAMPTZ,                           -- 最近一轮时间
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- admin 按状态过滤 + 时间倒序
CREATE INDEX IF NOT EXISTS idx_support_chat_conversations_status_last
    ON support_chat_conversations (last_status, last_at DESC);

-- admin 按用户查 + 时间倒序
CREATE INDEX IF NOT EXISTS idx_support_chat_conversations_user_last
    ON support_chat_conversations (user_id, last_at DESC);

-- 逐轮消息
CREATE TABLE IF NOT EXISTS support_chat_messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES support_chat_conversations(id) ON DELETE CASCADE,
    role            VARCHAR(16) NOT NULL,                -- user | assistant
    content         TEXT NOT NULL,
    status          VARCHAR(24),                         -- 仅 assistant 行有值
    error_message   TEXT,                                -- status != success 时的细节
    model           VARCHAR(128),                        -- 本轮使用的上游模型
    latency_ms      INT,                                 -- 本轮耗时（进入到流收尾）
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 详情按时间取整段对话
CREATE INDEX IF NOT EXISTS idx_support_chat_messages_conv_created
    ON support_chat_messages (conversation_id, created_at);

-- 表/列注释
COMMENT ON TABLE  support_chat_conversations             IS '客服浮窗对话会话头（以客户端 session_id 归并多轮问答）';
COMMENT ON COLUMN support_chat_conversations.session_id  IS '客户端生成的会话键，唯一，用于幂等 upsert 归并同一段对话';
COMMENT ON COLUMN support_chat_conversations.user_id     IS '登录用户 ID；匿名对话为 NULL（用户删除时置 NULL）';
COMMENT ON COLUMN support_chat_conversations.turn_count  IS '已归并的问答轮数（每轮 +1）';
COMMENT ON COLUMN support_chat_conversations.last_status IS '最近一轮 assistant 状态: success | upstream_auth | upstream_error | interrupted | rate_limited | config_error';

COMMENT ON TABLE  support_chat_messages               IS '客服浮窗逐轮消息（user 行 + assistant 回包行）';
COMMENT ON COLUMN support_chat_messages.role          IS '角色: user | assistant';
COMMENT ON COLUMN support_chat_messages.content       IS '消息正文；落库前由 service 层截断到 50000 字符';
COMMENT ON COLUMN support_chat_messages.status        IS 'assistant 行状态: success | upstream_auth | upstream_error | interrupted | rate_limited | config_error（user 行为 NULL）';
COMMENT ON COLUMN support_chat_messages.error_message IS '失败细节（status != success 时填）';
COMMENT ON COLUMN support_chat_messages.latency_ms    IS '本轮耗时毫秒（handler 进入到流收尾）';
