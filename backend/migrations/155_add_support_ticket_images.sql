-- 工单系统：为工单主帖 + 回复各加一列 images (jsonb)，用于承载多图上传能力。
--
-- 设计：
--   - 存储 [{key,url,size,mime}] 数组，NOT NULL DEFAULT '[]'，保证读回来永远是 array，
--     方便前端渲染无需处理 null / 缺失。
--   - 单条上限 5 张、单张 5 MB、格式仅 png/jpeg —— 全部由 service 层强制，DB 不加约束
--     以留出手动数据修复空间。
--   - 图片实体存放在通过 cos_image_transfer_config 配置的对象存储（前缀 support-tickets/），
--     DB 里只保存索引信息（key + 可外链的 URL）。
--   - 无独立开关：工单开启则附件能力开启，跟随 support_ticket_enabled。

ALTER TABLE support_tickets
    ADD COLUMN IF NOT EXISTS images jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE support_ticket_replies
    ADD COLUMN IF NOT EXISTS images jsonb NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN support_tickets.images        IS '工单首帖附带图片列表 [{key,url,size,mime}]，NOT NULL 默认空数组，最多 5 张';
COMMENT ON COLUMN support_ticket_replies.images IS '回复附带图片列表 [{key,url,size,mime}]，NOT NULL 默认空数组，最多 5 张';
