CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_groups_fallback_group_id_on_prompt_audit_block
    ON groups(fallback_group_id_on_prompt_audit_block)
    WHERE deleted_at IS NULL AND fallback_group_id_on_prompt_audit_block IS NOT NULL;
