-- 添加提示词审计拦截兜底分组配置

ALTER TABLE groups
ADD COLUMN IF NOT EXISTS fallback_group_id_on_prompt_audit_block BIGINT REFERENCES groups(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_groups_fallback_group_id_on_prompt_audit_block
ON groups(fallback_group_id_on_prompt_audit_block)
WHERE deleted_at IS NULL AND fallback_group_id_on_prompt_audit_block IS NOT NULL;

COMMENT ON COLUMN groups.fallback_group_id_on_prompt_audit_block IS '提示词审计拦截后兜底使用的分组 ID';
