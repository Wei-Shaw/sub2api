-- video_pricings: fal 视频模型分辨率定价表（PR-1）。
-- 每一行代表 (model_slug × resolution) 组合下每秒的定价（price_per_second）。
-- 计费公式：cost = price_per_second * duration_seconds * rate_multiplier。
-- 默认定价由 VideoPricingSeeder 在服务启动时灌入（fal 官方价 × 1.1）。
-- 字段与 ent/migrate/schema.go 的 VideoPricingsColumns 严格一致，索引名与 ent 生成保持相同。

CREATE TABLE IF NOT EXISTS video_pricings (
    id                BIGSERIAL PRIMARY KEY,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model_slug        VARCHAR(200)   NOT NULL,
    resolution        VARCHAR(16)    NOT NULL,
    price_per_second  NUMERIC(20,10) NOT NULL DEFAULT 0,
    currency          VARCHAR(8)     NOT NULL DEFAULT 'USD',
    enabled           BOOLEAN        NOT NULL DEFAULT TRUE,
    note              VARCHAR(512)
);

CREATE UNIQUE INDEX IF NOT EXISTS videopricing_model_slug_resolution
    ON video_pricings (model_slug, resolution);
CREATE INDEX IF NOT EXISTS videopricing_model_slug
    ON video_pricings (model_slug);
