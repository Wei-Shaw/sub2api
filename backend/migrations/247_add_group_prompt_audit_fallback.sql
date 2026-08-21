-- 添加提示词审计拦截兜底分组配置。列保持可空，旧版本写入不受影响。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS fallback_group_id_on_prompt_audit_block BIGINT;

-- Preserve the self-referencing foreign-key behavior without adding a blocking
-- foreign key to an already released table. The DELETE branch mirrors
-- ON DELETE SET NULL for the rare hard-delete path; normal group deletion is
-- a soft delete and deliberately leaves existing fallback behavior unchanged.
-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION sub2api_group_prompt_audit_fallback_guard()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        UPDATE groups
        SET fallback_group_id_on_prompt_audit_block = NULL
        WHERE fallback_group_id_on_prompt_audit_block = OLD.id;
        RETURN OLD;
    END IF;

    IF NEW.fallback_group_id_on_prompt_audit_block IS NOT NULL THEN
        PERFORM 1
        FROM groups
        WHERE id = NEW.fallback_group_id_on_prompt_audit_block
        FOR KEY SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'groups.fallback_group_id_on_prompt_audit_block references missing group %', NEW.fallback_group_id_on_prompt_audit_block
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE TRIGGER groups_prompt_audit_fallback_guard
BEFORE INSERT OR UPDATE OF fallback_group_id_on_prompt_audit_block ON groups
FOR EACH ROW
EXECUTE FUNCTION sub2api_group_prompt_audit_fallback_guard();

-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE TRIGGER groups_clear_prompt_audit_fallback_on_delete
AFTER DELETE ON groups
FOR EACH ROW
EXECUTE FUNCTION sub2api_group_prompt_audit_fallback_guard();
