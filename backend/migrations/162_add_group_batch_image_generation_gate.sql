ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allow_batch_image_generation BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN groups.allow_batch_image_generation IS '是否允许该分组使用批量图片生成能力';
