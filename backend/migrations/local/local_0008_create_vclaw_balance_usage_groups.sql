-- V-Claw balance packages must point at usage/balance groups, not subscription groups.
-- Create standard usage groups by mirroring the V-Claw subscription plan groups,
-- copy account membership so routing stays identical, then map balance_packages by code.

WITH source_groups AS (
    SELECT *
    FROM groups
    WHERE name IN ('V-Claw Standard', 'V-Claw Pro', 'V-Claw Expert', 'V-Claw Business', 'V-Claw Enterprise')
      AND subscription_type = 'subscription'
), target_groups AS (
    INSERT INTO groups (
        name,
        description,
        rate_multiplier,
        is_exclusive,
        status,
        platform,
        subscription_type,
        daily_limit_usd,
        weekly_limit_usd,
        monthly_limit_usd,
        default_validity_days,
        image_price_1k,
        image_price_2k,
        image_price_4k,
        claude_code_only,
        fallback_group_id,
        model_routing,
        model_routing_enabled,
        fallback_group_id_on_invalid_request,
        mcp_xml_inject,
        supported_model_scopes,
        sort_order,
        allow_messages_dispatch,
        default_mapped_model,
        require_oauth_only,
        require_privacy_set,
        messages_dispatch_model_config,
        rpm_limit,
        allow_image_generation,
        image_rate_independent,
        image_rate_multiplier
    )
    SELECT
        replace(s.name, 'V-Claw ', 'V-Claw Balance '),
        'Usage/balance group mirrored from subscription group ' || s.name || ' for balance package billing.',
        s.rate_multiplier,
        s.is_exclusive,
        s.status,
        s.platform,
        'standard',
        s.daily_limit_usd,
        s.weekly_limit_usd,
        s.monthly_limit_usd,
        s.default_validity_days,
        s.image_price_1k,
        s.image_price_2k,
        s.image_price_4k,
        s.claude_code_only,
        s.fallback_group_id,
        s.model_routing,
        s.model_routing_enabled,
        s.fallback_group_id_on_invalid_request,
        s.mcp_xml_inject,
        s.supported_model_scopes,
        s.sort_order + 100,
        s.allow_messages_dispatch,
        s.default_mapped_model,
        s.require_oauth_only,
        s.require_privacy_set,
        s.messages_dispatch_model_config,
        s.rpm_limit,
        s.allow_image_generation,
        s.image_rate_independent,
        s.image_rate_multiplier
    FROM source_groups s
    ON CONFLICT (name) WHERE deleted_at IS NULL DO UPDATE SET
        description = EXCLUDED.description,
        rate_multiplier = EXCLUDED.rate_multiplier,
        status = EXCLUDED.status,
        platform = EXCLUDED.platform,
        subscription_type = 'standard',
        daily_limit_usd = EXCLUDED.daily_limit_usd,
        weekly_limit_usd = EXCLUDED.weekly_limit_usd,
        monthly_limit_usd = EXCLUDED.monthly_limit_usd,
        model_routing = EXCLUDED.model_routing,
        model_routing_enabled = EXCLUDED.model_routing_enabled,
        supported_model_scopes = EXCLUDED.supported_model_scopes,
        sort_order = EXCLUDED.sort_order,
        updated_at = NOW()
    RETURNING id, name
), mapping AS (
    SELECT
        s.id AS source_group_id,
        COALESCE(t.id, existing.id) AS target_group_id,
        CASE s.name
            WHEN 'V-Claw Standard' THEN 'standard'
            WHEN 'V-Claw Pro' THEN 'pro'
            WHEN 'V-Claw Expert' THEN 'expert'
            WHEN 'V-Claw Business' THEN 'business'
            WHEN 'V-Claw Enterprise' THEN 'enterprise'
        END AS package_code
    FROM source_groups s
    LEFT JOIN target_groups t ON t.name = replace(s.name, 'V-Claw ', 'V-Claw Balance ')
    LEFT JOIN groups existing ON existing.name = replace(s.name, 'V-Claw ', 'V-Claw Balance ') AND existing.deleted_at IS NULL
), copied_accounts AS (
    INSERT INTO account_groups (account_id, group_id, priority)
    SELECT ag.account_id, m.target_group_id, ag.priority
    FROM account_groups ag
    JOIN mapping m ON m.source_group_id = ag.group_id
    WHERE m.target_group_id IS NOT NULL
    ON CONFLICT (account_id, group_id) DO UPDATE SET priority = EXCLUDED.priority
    RETURNING group_id
)
UPDATE balance_packages bp
SET group_id = m.target_group_id,
    updated_at = NOW()
FROM mapping m
WHERE bp.code = m.package_code
  AND m.target_group_id IS NOT NULL;
