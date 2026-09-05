-- 235: groups.models_list_config 更名为 model_allowlist，语义从「仅影响 /v1/models 展示」
-- 升级为分组级模型白名单（同时约束模型列表接口与请求准入）。数据原样保留。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'groups'
          AND column_name = 'models_list_config'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'groups'
          AND column_name = 'model_allowlist'
    ) THEN
        ALTER TABLE groups RENAME COLUMN models_list_config TO model_allowlist;
    END IF;
END
$$;

COMMENT ON COLUMN groups.model_allowlist IS
    'Group model allowlist: constrains both model listing responses and request admission';
