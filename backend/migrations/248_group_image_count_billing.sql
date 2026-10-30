-- 分组级按张计费开关：仅 OpenAI 与 Composite 分组使用。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS image_count_billing BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN groups.image_count_billing IS '图片生成是否按实际出图张数计费';
