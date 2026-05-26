-- 创建公告表
CREATE TABLE IF NOT EXISTS announcements (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    targeting JSONB NOT NULL DEFAULT '{}'::jsonb,
    starts_at TIMESTAMPTZ DEFAULT NULL,
    ends_at TIMESTAMPTZ DEFAULT NULL,
    created_by BIGINT DEFAULT NULL REFERENCES users(id) ON DELETE SET NULL,
    updated_by BIGINT DEFAULT NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 公告已读表
CREATE TABLE IF NOT EXISTS announcement_reads (
    id BIGSERIAL PRIMARY KEY,
    announcement_id BIGINT NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    read_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(announcement_id, user_id)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_announcements_status ON announcements(status);
CREATE INDEX IF NOT EXISTS idx_announcements_starts_at ON announcements(starts_at);
CREATE INDEX IF NOT EXISTS idx_announcements_ends_at ON announcements(ends_at);
CREATE INDEX IF NOT EXISTS idx_announcements_created_at ON announcements(created_at);

CREATE INDEX IF NOT EXISTS idx_announcement_reads_announcement_id ON announcement_reads(announcement_id);
CREATE INDEX IF NOT EXISTS idx_announcement_reads_user_id ON announcement_reads(user_id);
CREATE INDEX IF NOT EXISTS idx_announcement_reads_read_at ON announcement_reads(read_at);

COMMENT ON TABLE announcements IS '系统公告';
COMMENT ON COLUMN announcements.status IS '状态: draft, active, archived';
COMMENT ON COLUMN announcements.targeting IS '展示条件（JSON 规则）';
COMMENT ON COLUMN announcements.starts_at IS '开始展示时间（为空表示立即生效）';
COMMENT ON COLUMN announcements.ends_at IS '结束展示时间（为空表示永久生效）';

COMMENT ON TABLE announcement_reads IS '公告已读记录';
COMMENT ON COLUMN announcement_reads.read_at IS '用户首次已读时间';

