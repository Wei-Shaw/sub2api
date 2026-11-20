-- =============================================================================
-- 252: Account lifecycle ledger（账号订阅台账）
-- =============================================================================
-- 目的：回答「这个号什么时候开通、续了几次费、换过几次凭据、额度/套餐改过什么、
-- 什么时候到期被自动暂停、被谁删的」。251 的两张表只覆盖「请求 → 上游尝试」与
-- 「调度态变化（冷却/摘除）」，不覆盖订阅层面的生命周期。
--
-- 为什么用数据库触发器而不是在 Go 里埋点：accounts 的写入路径很多（后台单个/批量
-- 编辑、导入、到期自动暂停 AutoPauseExpiredAccounts、token 刷新、OAuth 重新授权、
-- Grok 对账……），任何一处漏埋都会让台账出现断档。触发器保证「凡是改了受跟踪字段，
-- 一定留痕」，这是台账的底线要求。代价是拿不到操作人，operator_id 预留给后续
-- 应用层补写。
--
-- 安全约束：触发器内部吞掉一切异常（只 RAISE WARNING），绝不能因为写台账失败而让
-- 账号更新本身失败。
--
-- 噪声控制：
--   * 只在受跟踪列变化时触发（WHEN 子句），限流/冷却等高频列（rate_limited_at、
--     temp_unschedulable_until、last_used_at、session_window_*）不触发。
--   * credentials / extra 是 JSONB，变化频繁：credentials 只比较凭据指纹，
--     extra 只比较套餐/额度相关键（plan_type、subscription*、quota*limit*、quota*reset*）。
--   * 台账不参与 ops 数据清理（与 251 的两张表不同），需长期保留。
-- =============================================================================

CREATE TABLE IF NOT EXISTS account_lifecycle_events (
    id                  BIGSERIAL PRIMARY KEY,
    account_id          BIGINT NOT NULL,
    -- created / deleted / restored / expired / renewed / expiry_shortened / expiry_set /
    -- expiry_cleared / plan_changed / quota_changed / disabled / enabled / errored /
    -- recovered / unschedulable / schedulable / credential_rotated / token_refreshed / updated
    event_type          VARCHAR(32) NOT NULL,
    -- 所有受跟踪字段的变化：{"field": {"from": ..., "to": ...}}
    changes             JSONB NOT NULL DEFAULT '{}'::jsonb,
    status_after        VARCHAR(20),
    expires_at_before   TIMESTAMPTZ,
    expires_at_after    TIMESTAMPTZ,
    credential_fp_before VARCHAR(16),
    credential_fp_after  VARCHAR(16),
    -- db：由触发器写入；manual / refresh / expiry 等预留给应用层
    source              VARCHAR(16) NOT NULL DEFAULT 'db',
    operator_id         BIGINT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_lifecycle_events_account_time
    ON account_lifecycle_events (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_account_lifecycle_events_type_time
    ON account_lifecycle_events (event_type, created_at DESC);

COMMENT ON TABLE account_lifecycle_events IS
    'Append-only account subscription ledger: creation, renewal, expiry, plan/quota and credential changes.';

-- 与 Go 侧 credentialFingerprint 保持一致：按 access_token → api_key → refresh_token
-- 取第一个非空值，sha256 前 8 字节的 hex。
CREATE OR REPLACE FUNCTION account_lifecycle_credential_fp(creds JSONB)
RETURNS VARCHAR(16)
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN v IS NULL THEN NULL
                ELSE encode(substring(sha256(convert_to(v, 'UTF8')) FROM 1 FOR 8), 'hex')
           END
    FROM (
        SELECT COALESCE(
            NULLIF(btrim(creds->>'access_token'), ''),
            NULLIF(btrim(creds->>'api_key'), ''),
            NULLIF(btrim(creds->>'refresh_token'), '')
        ) AS v
    ) t
$$;

-- 只保留套餐/额度相关的 extra 键，避免用量快照等高频字段制造噪声。
CREATE OR REPLACE FUNCTION account_lifecycle_plan_extra(extra JSONB)
RETURNS JSONB
LANGUAGE sql IMMUTABLE AS $$
    SELECT COALESCE(jsonb_object_agg(key, value), '{}'::jsonb)
    FROM jsonb_each(COALESCE(extra, '{}'::jsonb))
    WHERE key = 'plan_type'
       OR key LIKE 'subscription%'
       OR (key LIKE 'quota%' AND (key LIKE '%limit%' OR key LIKE '%reset%'))
$$;

CREATE OR REPLACE FUNCTION account_lifecycle_record()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    changes     JSONB := '{}'::jsonb;
    ev_type     TEXT;
    fp_old      VARCHAR(16);
    fp_new      VARCHAR(16);
    extra_old   JSONB;
    extra_new   JSONB;
    k           TEXT;
BEGIN
    BEGIN
        IF TG_OP = 'INSERT' THEN
            INSERT INTO account_lifecycle_events
                (account_id, event_type, changes, status_after, expires_at_after, credential_fp_after)
            VALUES
                (NEW.id, 'created',
                 jsonb_build_object(
                     'platform', NEW.platform, 'type', NEW.type, 'status', NEW.status,
                     'expires_at', NEW.expires_at,
                     'plan', account_lifecycle_plan_extra(NEW.extra)),
                 NEW.status, NEW.expires_at, account_lifecycle_credential_fp(NEW.credentials));
            RETURN NULL;
        END IF;

        IF TG_OP = 'DELETE' THEN
            INSERT INTO account_lifecycle_events
                (account_id, event_type, status_after, expires_at_before, credential_fp_before)
            VALUES
                (OLD.id, 'deleted', OLD.status, OLD.expires_at, account_lifecycle_credential_fp(OLD.credentials));
            RETURN NULL;
        END IF;

        -- UPDATE：逐字段收集变化
        IF OLD.status IS DISTINCT FROM NEW.status THEN
            changes := changes || jsonb_build_object('status', jsonb_build_object('from', OLD.status, 'to', NEW.status));
        END IF;
        IF OLD.schedulable IS DISTINCT FROM NEW.schedulable THEN
            changes := changes || jsonb_build_object('schedulable', jsonb_build_object('from', OLD.schedulable, 'to', NEW.schedulable));
        END IF;
        IF OLD.expires_at IS DISTINCT FROM NEW.expires_at THEN
            changes := changes || jsonb_build_object('expires_at', jsonb_build_object('from', OLD.expires_at, 'to', NEW.expires_at));
        END IF;
        IF OLD.auto_pause_on_expired IS DISTINCT FROM NEW.auto_pause_on_expired THEN
            changes := changes || jsonb_build_object('auto_pause_on_expired', jsonb_build_object('from', OLD.auto_pause_on_expired, 'to', NEW.auto_pause_on_expired));
        END IF;
        IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at THEN
            changes := changes || jsonb_build_object('deleted_at', jsonb_build_object('from', OLD.deleted_at, 'to', NEW.deleted_at));
        END IF;
        IF OLD.proxy_id IS DISTINCT FROM NEW.proxy_id THEN
            changes := changes || jsonb_build_object('proxy_id', jsonb_build_object('from', OLD.proxy_id, 'to', NEW.proxy_id));
        END IF;
        IF OLD.concurrency IS DISTINCT FROM NEW.concurrency THEN
            changes := changes || jsonb_build_object('concurrency', jsonb_build_object('from', OLD.concurrency, 'to', NEW.concurrency));
        END IF;
        IF OLD.priority IS DISTINCT FROM NEW.priority THEN
            changes := changes || jsonb_build_object('priority', jsonb_build_object('from', OLD.priority, 'to', NEW.priority));
        END IF;
        IF OLD.rate_multiplier IS DISTINCT FROM NEW.rate_multiplier THEN
            changes := changes || jsonb_build_object('rate_multiplier', jsonb_build_object('from', OLD.rate_multiplier, 'to', NEW.rate_multiplier));
        END IF;
        IF OLD.name IS DISTINCT FROM NEW.name THEN
            changes := changes || jsonb_build_object('name', jsonb_build_object('from', OLD.name, 'to', NEW.name));
        END IF;

        fp_old := account_lifecycle_credential_fp(OLD.credentials);
        fp_new := account_lifecycle_credential_fp(NEW.credentials);
        IF fp_old IS DISTINCT FROM fp_new THEN
            changes := changes || jsonb_build_object('credential_fp', jsonb_build_object('from', fp_old, 'to', fp_new));
        END IF;

        extra_old := account_lifecycle_plan_extra(OLD.extra);
        extra_new := account_lifecycle_plan_extra(NEW.extra);
        IF extra_old IS DISTINCT FROM extra_new THEN
            FOR k IN SELECT jsonb_object_keys(extra_old || extra_new) LOOP
                IF (extra_old -> k) IS DISTINCT FROM (extra_new -> k) THEN
                    changes := changes || jsonb_build_object('extra.' || k,
                        jsonb_build_object('from', extra_old -> k, 'to', extra_new -> k));
                END IF;
            END LOOP;
        END IF;

        IF changes = '{}'::jsonb THEN
            RETURN NULL;
        END IF;

        -- 主事件类型：按「对订阅生命周期的重要程度」取最高优先级的一条，
        -- 其余变化都在 changes 里。
        ev_type := CASE
            WHEN OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN 'deleted'
            WHEN OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL THEN 'restored'
            WHEN OLD.schedulable AND NOT NEW.schedulable
                 AND NEW.auto_pause_on_expired AND NEW.expires_at IS NOT NULL
                 AND NEW.expires_at <= NOW() THEN 'expired'
            WHEN OLD.expires_at IS NOT NULL AND NEW.expires_at IS NOT NULL
                 AND NEW.expires_at > OLD.expires_at THEN 'renewed'
            WHEN OLD.expires_at IS NOT NULL AND NEW.expires_at IS NOT NULL
                 AND NEW.expires_at < OLD.expires_at THEN 'expiry_shortened'
            WHEN OLD.expires_at IS NULL AND NEW.expires_at IS NOT NULL THEN 'expiry_set'
            WHEN OLD.expires_at IS NOT NULL AND NEW.expires_at IS NULL THEN 'expiry_cleared'
            WHEN (extra_old -> 'plan_type') IS DISTINCT FROM (extra_new -> 'plan_type') THEN 'plan_changed'
            WHEN extra_old IS DISTINCT FROM extra_new THEN 'quota_changed'
            WHEN OLD.status IS DISTINCT FROM NEW.status AND NEW.status = 'disabled' THEN 'disabled'
            WHEN OLD.status IS DISTINCT FROM NEW.status AND NEW.status = 'error' THEN 'errored'
            WHEN OLD.status IS DISTINCT FROM NEW.status AND OLD.status = 'error' THEN 'recovered'
            WHEN OLD.status IS DISTINCT FROM NEW.status AND NEW.status = 'active' THEN 'enabled'
            WHEN fp_old IS DISTINCT FROM fp_new AND NEW.type = 'apikey' THEN 'credential_rotated'
            WHEN fp_old IS DISTINCT FROM fp_new THEN 'token_refreshed'
            WHEN OLD.schedulable AND NOT NEW.schedulable THEN 'unschedulable'
            WHEN NOT OLD.schedulable AND NEW.schedulable THEN 'schedulable'
            ELSE 'updated'
        END;

        INSERT INTO account_lifecycle_events
            (account_id, event_type, changes, status_after,
             expires_at_before, expires_at_after, credential_fp_before, credential_fp_after)
        VALUES
            (NEW.id, ev_type, changes, NEW.status,
             OLD.expires_at, NEW.expires_at, fp_old, fp_new);
    EXCEPTION WHEN OTHERS THEN
        RAISE WARNING 'account_lifecycle_record failed for account %: %',
            COALESCE(NEW.id, OLD.id), SQLERRM;
    END;
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_account_lifecycle_insert ON accounts;
CREATE TRIGGER trg_account_lifecycle_insert
    AFTER INSERT ON accounts
    FOR EACH ROW EXECUTE FUNCTION account_lifecycle_record();

DROP TRIGGER IF EXISTS trg_account_lifecycle_delete ON accounts;
CREATE TRIGGER trg_account_lifecycle_delete
    AFTER DELETE ON accounts
    FOR EACH ROW EXECUTE FUNCTION account_lifecycle_record();

DROP TRIGGER IF EXISTS trg_account_lifecycle_update ON accounts;
CREATE TRIGGER trg_account_lifecycle_update
    AFTER UPDATE ON accounts
    FOR EACH ROW
    WHEN (
        OLD.status IS DISTINCT FROM NEW.status
        OR OLD.schedulable IS DISTINCT FROM NEW.schedulable
        OR OLD.expires_at IS DISTINCT FROM NEW.expires_at
        OR OLD.auto_pause_on_expired IS DISTINCT FROM NEW.auto_pause_on_expired
        OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at
        OR OLD.proxy_id IS DISTINCT FROM NEW.proxy_id
        OR OLD.concurrency IS DISTINCT FROM NEW.concurrency
        OR OLD.priority IS DISTINCT FROM NEW.priority
        OR OLD.rate_multiplier IS DISTINCT FROM NEW.rate_multiplier
        OR OLD.name IS DISTINCT FROM NEW.name
        OR OLD.credentials IS DISTINCT FROM NEW.credentials
        OR OLD.extra IS DISTINCT FROM NEW.extra
    )
    EXECUTE FUNCTION account_lifecycle_record();
