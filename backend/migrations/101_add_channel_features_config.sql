ALTER TABLE channels ADD COLUMN IF NOT EXISTS features_config JSONB NOT NULL DEFAULT '{}';
COMMENT ON COLUMN channels.features_config IS '渠道特性配置（如 web_search_emulation），JSON 对象格式';
