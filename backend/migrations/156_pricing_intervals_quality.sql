-- 为渠道定价区间表增加 quality 维度，支持图片 (尺寸档位 × 质量) 二维定价。
-- 可空，默认 NULL/空串表示不区分质量（兼容存量按尺寸单维定价）。
ALTER TABLE channel_pricing_intervals ADD COLUMN IF NOT EXISTS quality VARCHAR(16);
