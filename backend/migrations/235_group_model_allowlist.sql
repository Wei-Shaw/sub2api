-- 235: 将分组模型白名单扩展为分组级模型准入配置。
--
-- 兼容说明：v0.2.1 实例仍可能在蓝绿切换期间运行，并继续读取
-- groups.models_list_config。保留旧列、新增新列并双向同步，避免旧版实例
-- 因列重命名立即报错，同时确保两代实例看到同一份配置。
DO $$
BEGIN
    ALTER TABLE groups
        ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb,
        ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;

    UPDATE groups
    SET model_allowlist = CASE
        WHEN model_allowlist <> '{}'::jsonb THEN model_allowlist
        ELSE models_list_config
    END;

    UPDATE groups
    SET models_list_config = model_allowlist;
END
$$;

COMMENT ON COLUMN groups.model_allowlist IS
    'Group model allowlist: constrains both model listing responses and request admission';

COMMENT ON COLUMN groups.models_list_config IS
    'Legacy alias for model_allowlist; kept for mixed-version compatibility';

CREATE OR REPLACE FUNCTION sync_groups_model_allowlist_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.model_allowlist <> '{}'::jsonb THEN
            NEW.models_list_config := NEW.model_allowlist;
        ELSE
            NEW.model_allowlist := NEW.models_list_config;
        END IF;
    ELSIF NEW.model_allowlist IS DISTINCT FROM OLD.model_allowlist
       AND NEW.models_list_config IS NOT DISTINCT FROM OLD.models_list_config THEN
        NEW.models_list_config := NEW.model_allowlist;
    ELSIF NEW.models_list_config IS DISTINCT FROM OLD.models_list_config
       AND NEW.model_allowlist IS NOT DISTINCT FROM OLD.model_allowlist THEN
        NEW.model_allowlist := NEW.models_list_config;
    ELSIF NEW.model_allowlist IS DISTINCT FROM OLD.model_allowlist
       AND NEW.models_list_config IS DISTINCT FROM OLD.models_list_config THEN
        NEW.models_list_config := NEW.model_allowlist;
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS sync_groups_model_allowlist_columns_trigger ON groups;

CREATE TRIGGER sync_groups_model_allowlist_columns_trigger
BEFORE INSERT OR UPDATE OF model_allowlist, models_list_config ON groups
FOR EACH ROW
EXECUTE FUNCTION sync_groups_model_allowlist_columns();
