-- 加速 "公告已读情况" 后台页面按已读时间排序的查询。
-- Speeds up the admin "announcement read status" page when sorting by read_at.
--
-- 现有索引 idx_announcement_reads_read_at 是全局单列索引,不适合
-- "某个公告内按 read_at 排序"的查询模式 —— 优化器会回退到先按 announcement_id
-- 过滤再内存排序。本索引把 announcement_id 作为前导列,使排序能直接走索引扫描。
--
-- The existing idx_announcement_reads_read_at is a global single-column index
-- that does not help the "within a single announcement, order by read_at"
-- query pattern. This composite index lets that ordering be served directly
-- from index scans.

CREATE INDEX IF NOT EXISTS idx_announcement_reads_announcement_id_read_at
    ON announcement_reads (announcement_id, read_at DESC NULLS LAST);
