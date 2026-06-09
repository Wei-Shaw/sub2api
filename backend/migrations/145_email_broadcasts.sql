-- 管理员批量公告邮件记录表 / admin bulk-announcement email broadcasts.
-- 用途：记录每次管理员发起的"广播公告邮件"批次，包括正文、收件人、聚合状态与计数。
-- Purpose: persist each admin-initiated broadcast for audit/history/retry.

CREATE TABLE IF NOT EXISTS email_broadcasts (
    id                 BIGSERIAL PRIMARY KEY,
    subject            VARCHAR(200)   NOT NULL,
    body               TEXT           NOT NULL,
    body_format        VARCHAR(10)    NOT NULL DEFAULT 'html',
    recipients_mode    VARCHAR(20)    NOT NULL DEFAULT 'selected',
    recipient_user_ids JSONB,
    status             VARCHAR(20)    NOT NULL DEFAULT 'pending',
    total_count        INTEGER        NOT NULL DEFAULT 0,
    success_count      INTEGER        NOT NULL DEFAULT 0,
    failed_count       INTEGER        NOT NULL DEFAULT 0,
    error_message      TEXT,
    created_by         BIGINT,
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),

    CONSTRAINT email_broadcasts_status_check
        CHECK (status IN ('pending', 'sending', 'completed', 'failed')),
    CONSTRAINT email_broadcasts_body_format_check
        CHECK (body_format IN ('html', 'text')),
    CONSTRAINT email_broadcasts_recipients_mode_check
        CHECK (recipients_mode IN ('all', 'selected')),
    CONSTRAINT email_broadcasts_counts_non_negative
        CHECK (total_count >= 0 AND success_count >= 0 AND failed_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_email_broadcasts_status     ON email_broadcasts (status);
CREATE INDEX IF NOT EXISTS idx_email_broadcasts_created_at ON email_broadcasts (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_email_broadcasts_created_by ON email_broadcasts (created_by);
