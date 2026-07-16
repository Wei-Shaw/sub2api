-- 工单未读游标 + 站内通知：为工单系统补充"未读工单数"聚合、铃铛面板工单 tab 数据源、
-- 以及邮件事件的持久化触发凭据。
--
-- 设计（openspec/changes/ticket-notifications/design.md）：
--   - support_ticket_reads：每个 (ticket_id, user_id) 一行的读游标，记录该用户
--     最后一次查看/回复该工单详情的时刻；未读判定 = 存在 reply.created_at > last_read_at。
--     用户 GET detail 或 admin 回复自己工单时 upsert（ON CONFLICT DO UPDATE）。
--     两个 FK 都用 CASCADE：工单删除时读游标一并清；用户注销时游标清理。
--
--   - support_ticket_notification：面向铃铛面板的持久化通知记录，每个 recipient
--     一行。三类事件：ticket_created（工单新建，发给所有 admin）、
--     admin_replied（admin 回复，发给工单 owner）、user_replied（用户回复，发给所有 admin）。
--     title_snapshot 是写入时的工单标题快照，工单标题后续被改动时通知面板依然显示原值。
--     actor_user_id 用 SET NULL：审计意义大于外键完整性，用户注销后通知不要跟着消失。
--
--   - 邮件通道独立并行：同事件同 recipient 也会走 NotificationEmailService 发邮件，
--     邮件与本表 rows 一一对应但没有跨表外键关系（邮件失败不影响通知记录）。
--
--   - 前向迁移：单文件建两张新表 + 索引；无兼容性负担。回滚仅需 DROP TABLE
--     support_ticket_notification, support_ticket_reads;（本项目 migrations 目录
--     为 forward-only，回滚 SQL 不放在此文件内，见 backend/migrations/README.md）。

-- ============================================================================
-- 1) 读游标：per-(ticket, user) 单行
-- ============================================================================
CREATE TABLE IF NOT EXISTS support_ticket_reads (
    id            BIGSERIAL PRIMARY KEY,
    ticket_id     BIGINT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    user_id       BIGINT NOT NULL REFERENCES users(id)           ON DELETE CASCADE,
    last_read_at  TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_support_ticket_reads_ticket_user UNIQUE (ticket_id, user_id)
);

-- 未读聚合按 user 扫最近读过的工单集合。
CREATE INDEX IF NOT EXISTS idx_support_ticket_reads_user_last
    ON support_ticket_reads (user_id, last_read_at);

COMMENT ON TABLE  support_ticket_reads              IS '工单每用户读游标（用于未读聚合）';
COMMENT ON COLUMN support_ticket_reads.ticket_id    IS '所属工单 ID（工单删除时级联清理）';
COMMENT ON COLUMN support_ticket_reads.user_id      IS '读游标持有者（工单作者或 admin）';
COMMENT ON COLUMN support_ticket_reads.last_read_at IS '该用户最后一次读取该工单详情的时刻';

-- ============================================================================
-- 2) 通知记录：per-(event, recipient) 单行
-- ============================================================================
CREATE TABLE IF NOT EXISTS support_ticket_notification (
    id                 BIGSERIAL PRIMARY KEY,
    recipient_user_id  BIGINT NOT NULL REFERENCES users(id)           ON DELETE CASCADE,
    ticket_id          BIGINT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    event_type         VARCHAR(50)  NOT NULL,
    title_snapshot     VARCHAR(200) NOT NULL,
    excerpt            VARCHAR(500),
    actor_user_id      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    is_read            BOOLEAN NOT NULL DEFAULT FALSE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at            TIMESTAMPTZ
);

-- 铃铛面板主查询：某 recipient 的最新 N 条 + 未读数聚合。
CREATE INDEX IF NOT EXISTS idx_support_ticket_notification_recipient_read_created
    ON support_ticket_notification (recipient_user_id, is_read, created_at DESC);

-- ticket 详情内批量操作 & 级联删除辅助。
CREATE INDEX IF NOT EXISTS idx_support_ticket_notification_ticket
    ON support_ticket_notification (ticket_id);

COMMENT ON TABLE  support_ticket_notification                    IS '工单站内通知（铃铛面板工单 tab 数据源）';
COMMENT ON COLUMN support_ticket_notification.recipient_user_id  IS '接收该通知的用户（工单作者或 admin）';
COMMENT ON COLUMN support_ticket_notification.ticket_id          IS '关联工单 ID（工单删除时级联清理）';
COMMENT ON COLUMN support_ticket_notification.event_type         IS '事件类型: ticket_created | admin_replied | user_replied';
COMMENT ON COLUMN support_ticket_notification.title_snapshot     IS '写入时工单标题快照（工单标题后续被改动时面板仍显示当时值）';
COMMENT ON COLUMN support_ticket_notification.excerpt            IS '事件正文摘要（首帖或回复内容首 200 字符），可空';
COMMENT ON COLUMN support_ticket_notification.actor_user_id      IS '触发事件的用户 ID；用户被删除时置 NULL（审计意义大于外键完整性）';
COMMENT ON COLUMN support_ticket_notification.is_read            IS 'recipient 是否已读该条通知';
COMMENT ON COLUMN support_ticket_notification.read_at            IS '标记已读的时刻；is_read=true 时非空';
