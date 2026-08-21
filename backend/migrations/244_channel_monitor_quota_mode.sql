-- Migration: 244_channel_monitor_quota_mode
-- 渠道监控配额模式。编号从上游 226 顺延，避免和 fork 已有 226 冲突。
--
-- 这些列保持可空、无数据库默认，保证蓝绿期间旧版本仍可写入。
-- 触发器在数据库边界补齐旧写入的 check_mode，并保持 account_id 的
-- 外键和删除置空语义。

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_monitors
    DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                       'antigravity', 'kimi', 'zhipu', 'deepseek')) NOT VALID;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_monitor_request_templates
    DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_monitor_request_templates
    ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                       'antigravity', 'kimi', 'zhipu', 'deepseek')) NOT VALID;

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS check_mode VARCHAR(32);

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS account_id BIGINT;

ALTER TABLE channel_monitor_histories
    ADD COLUMN IF NOT EXISTS quota JSONB;

-- Existing monitors retain their historical probe behavior. The trigger below
-- also covers old binaries that insert a monitor without the new column.
-- sub2api-managed-update: reviewed-compatible
UPDATE channel_monitors
SET check_mode = 'probe'
WHERE check_mode IS NULL;

-- The trigger emulates the former NOT NULL DEFAULT for old writers and keeps
-- account_id referentially valid. FOR KEY SHARE closes the check/delete race.
-- A separate DELETE trigger preserves ON DELETE SET NULL for hard deletes.
-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION sub2api_channel_monitor_write_guard()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.check_mode IS NULL THEN
        NEW.check_mode := 'probe';
    END IF;

    IF NEW.account_id IS NOT NULL THEN
        PERFORM 1
        FROM accounts
        WHERE id = NEW.account_id
        FOR KEY SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'channel_monitors.account_id references missing account %', NEW.account_id
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE TRIGGER channel_monitors_write_guard
BEFORE INSERT OR UPDATE OF check_mode, account_id ON channel_monitors
FOR EACH ROW
EXECUTE FUNCTION sub2api_channel_monitor_write_guard();

-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION sub2api_clear_channel_monitor_account_on_delete()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE channel_monitors
    SET account_id = NULL
    WHERE account_id = OLD.id;
    RETURN OLD;
END;
$$;

-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE TRIGGER accounts_clear_channel_monitor_account
AFTER DELETE ON accounts
FOR EACH ROW
EXECUTE FUNCTION sub2api_clear_channel_monitor_account_on_delete();

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_monitors
    DROP CONSTRAINT IF EXISTS channel_monitors_check_mode_check;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_check_mode_check
    CHECK (check_mode IN ('probe', 'quota', 'quota_probe')) NOT VALID;

INSERT INTO settings (key, value)
VALUES ('channel_monitor_show_quota', 'false')
ON CONFLICT (key) DO NOTHING;
