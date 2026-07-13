-- 工单系统：用户提交结构化反馈，admin 列表/回复/关闭。
-- 总开关 settings.support_ticket_enabled 控制 API 与前端入口的可见性，关闭时 API 返回 404。

-- 工单主表
CREATE TABLE IF NOT EXISTS support_tickets (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title          VARCHAR(200) NOT NULL,
    content        TEXT NOT NULL,
    category       VARCHAR(50) NOT NULL,
    status         VARCHAR(20) NOT NULL DEFAULT 'open',     -- open | in_progress | closed
    priority       VARCHAR(20) NOT NULL DEFAULT 'normal',   -- low | normal | high
    chat_context   TEXT,                                    -- 可空：浮窗带过来的对话快照（不解析）
    closed_at      TIMESTAMPTZ,                             -- 仅 status = closed 时填
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 用户工单列表索引（按用户 + 状态 + 时间倒序）
CREATE INDEX IF NOT EXISTS idx_support_tickets_user_status_created
    ON support_tickets (user_id, status, created_at DESC);

-- admin 过滤索引（按 status × priority × 时间）
CREATE INDEX IF NOT EXISTS idx_support_tickets_status_priority_created
    ON support_tickets (status, priority, created_at DESC);

-- 工单回复表
CREATE TABLE IF NOT EXISTS support_ticket_replies (
    id          BIGSERIAL PRIMARY KEY,
    ticket_id   BIGINT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    author_id   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    is_admin    BOOLEAN NOT NULL DEFAULT FALSE,
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_support_ticket_replies_ticket_created
    ON support_ticket_replies (ticket_id, created_at);

-- 表/列注释（运维查表/排查时有用）
COMMENT ON TABLE  support_tickets             IS '用户工单（站内反馈/客服）';
COMMENT ON COLUMN support_tickets.status      IS '工单状态: open | in_progress | closed';
COMMENT ON COLUMN support_tickets.priority    IS '优先级: low | normal | high';
COMMENT ON COLUMN support_tickets.chat_context IS '可空：浮窗带过来的对话上下文快照（不解析、最长 50000 字符）';
COMMENT ON COLUMN support_tickets.closed_at   IS '关闭时间（仅 status = closed 时填）';

COMMENT ON TABLE  support_ticket_replies            IS '工单回复（用户或 admin）';
COMMENT ON COLUMN support_ticket_replies.is_admin   IS '是否 admin 回复（写入时角色快照，避免降权丢失权威标识）';
