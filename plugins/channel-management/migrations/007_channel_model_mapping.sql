SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE channels ADD COLUMN IF NOT EXISTS model_mapping JSONB DEFAULT '{}';
COMMENT ON COLUMN channels.model_mapping IS '渠道级模型映射，在账号映射之前执行。格式：{"source_model": "target_model"}';
