-- 添加分组支持的模型系列字段
ALTER TABLE groups
ADD COLUMN IF NOT EXISTS supported_model_scopes JSONB NOT NULL
DEFAULT '["claude", "gemini_text", "gemini_image"]'::jsonb;

COMMENT ON COLUMN groups.supported_model_scopes IS '支持的模型系列：claude, gemini_text, gemini_image';
