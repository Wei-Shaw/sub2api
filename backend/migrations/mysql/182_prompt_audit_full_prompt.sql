-- MySQL 方言版本（翻译自 ../182_prompt_audit_full_prompt.sql）
-- Retain the full (unredacted) prompt text on audit events so admins can
-- review the exact content that triggered a finding. Scoped to events only:
-- transient processing jobs keep storing redacted metadata.
--
-- 说明：TEXT 列默认值 → 表达式默认值 DEFAULT ('')。
ALTER TABLE prompt_audit_events
    ADD COLUMN full_prompt TEXT NOT NULL DEFAULT ('');
