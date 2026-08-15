-- Migration 222: Composite 分组的端点默认路由兜底开关。解析顺序是「显式路由规则 -> 内置模型名
-- 检测器 -> 端点默认」：只有显式规则和检测器都未命中时，才按调用端点回落到内建
-- 默认 provider（messages/count_tokens -> anthropic，responses/chat_completions/
-- embeddings -> openai，gemini -> gemini）。因为排在检测器之后，开启该开关不会改变
-- 任何原本就能解析成功的请求。images 端点不参与该兜底。
--
-- 该字段属于 API Key auth snapshot，并且决定 composite 请求最终落到哪个 provider，
-- 因此必须扩展持久化失效触发器：带外修改分组（直接 SQL、应用层失效前崩溃）不能让
-- 缓存快照继续使用旧的路由开关。正常后台保存已经通过 InvalidateAuthCacheByGroupID
-- 失效，此触发器是持久化兜底。
-- 函数体基于 193_group_profit_control_auth_cache_invalidation.sql 的最新版本，
-- 仅追加 endpoint_default_routing_enabled 比较条件，profit-control / peak 相关的
-- 判定全部保留。

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS endpoint_default_routing_enabled BOOLEAN NOT NULL DEFAULT false;

CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_group_id BIGINT;
BEGIN
    target_group_id := OLD.id;
    IF TG_OP = 'UPDATE'
       AND OLD.status IS NOT DISTINCT FROM NEW.status
       AND OLD.is_exclusive IS NOT DISTINCT FROM NEW.is_exclusive
       AND OLD.allow_image_generation IS NOT DISTINCT FROM NEW.allow_image_generation
       AND OLD.platform IS NOT DISTINCT FROM NEW.platform
       AND OLD.subscription_type IS NOT DISTINCT FROM NEW.subscription_type
       AND OLD.rate_multiplier IS NOT DISTINCT FROM NEW.rate_multiplier
       AND OLD.peak_rate_enabled IS NOT DISTINCT FROM NEW.peak_rate_enabled
       AND OLD.peak_start IS NOT DISTINCT FROM NEW.peak_start
       AND OLD.peak_end IS NOT DISTINCT FROM NEW.peak_end
       AND OLD.peak_rate_multiplier IS NOT DISTINCT FROM NEW.peak_rate_multiplier
       AND OLD.profit_control_enabled IS NOT DISTINCT FROM NEW.profit_control_enabled
       AND OLD.profit_min_margin IS NOT DISTINCT FROM NEW.profit_min_margin
       AND OLD.profit_safety_buffer IS NOT DISTINCT FROM NEW.profit_safety_buffer
       AND OLD.endpoint_default_routing_enabled IS NOT DISTINCT FROM NEW.endpoint_default_routing_enabled
       AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN
        RETURN NEW;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.group_id = target_group_id
      AND k.deleted_at IS NULL
      AND k.key <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
