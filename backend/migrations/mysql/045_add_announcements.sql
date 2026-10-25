-- MySQL 方言版本（翻译自 ../045_add_announcements.sql）

-- 创建公告表
CREATE TABLE IF NOT EXISTS announcements (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    targeting JSON NOT NULL DEFAULT ('{}'),
    starts_at DATETIME(3) DEFAULT NULL,
    ends_at DATETIME(3) DEFAULT NULL,
    created_by BIGINT DEFAULT NULL,
    updated_by BIGINT DEFAULT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_announcements_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_announcements_updated_by FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 公告已读表
CREATE TABLE IF NOT EXISTS announcement_reads (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    announcement_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    read_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uq_announcement_reads_announcement_id_user_id (announcement_id, user_id),
    CONSTRAINT fk_announcement_reads_announcement_id FOREIGN KEY (announcement_id) REFERENCES announcements(id) ON DELETE CASCADE,
    CONSTRAINT fk_announcement_reads_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 索引
CREATE INDEX idx_announcements_status ON announcements(status);
CREATE INDEX idx_announcements_starts_at ON announcements(starts_at);
CREATE INDEX idx_announcements_ends_at ON announcements(ends_at);
CREATE INDEX idx_announcements_created_at ON announcements(created_at);

CREATE INDEX idx_announcement_reads_announcement_id ON announcement_reads(announcement_id);
CREATE INDEX idx_announcement_reads_user_id ON announcement_reads(user_id);
CREATE INDEX idx_announcement_reads_read_at ON announcement_reads(read_at);

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)

-- (comment skipped)
-- (comment skipped)
