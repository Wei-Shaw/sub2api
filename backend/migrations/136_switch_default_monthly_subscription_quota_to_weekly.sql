-- Switch the free/default $100 subscription from a 30-day monthly grant
-- to a 7-day weekly grant.
--
-- This is intentionally scoped to subscription groups that look like the
-- previously bulk-granted/default quota group:
--   - subscription group
--   - monthly_limit_usd = 100
--   - weekly_limit_usd is unset or zero
--   - referenced by default grant settings, or already assigned to users while
--     not currently sold as a subscription plan

DROP TABLE IF EXISTS tmp_weekly_grant_target_groups;

CREATE TEMP TABLE tmp_weekly_grant_target_groups ON COMMIT DROP AS
WITH default_grant_groups AS (
    SELECT DISTINCT (item ->> 'group_id')::BIGINT AS group_id
    FROM settings s
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE
            WHEN btrim(s.value) LIKE '[%' THEN s.value::jsonb
            ELSE '[]'::jsonb
        END
    ) AS item
    WHERE s.key IN (
        'default_subscriptions',
        'auth_source_default_email_subscriptions',
        'auth_source_default_linuxdo_subscriptions',
        'auth_source_default_oidc_subscriptions',
        'auth_source_default_wechat_subscriptions',
        'auth_source_default_github_subscriptions',
        'auth_source_default_google_subscriptions'
    )
      AND (item ->> 'group_id') ~ '^[0-9]+$'
)
SELECT g.id
FROM groups g
WHERE g.deleted_at IS NULL
  AND g.subscription_type = 'subscription'
  AND g.monthly_limit_usd = 100
  AND COALESCE(g.weekly_limit_usd, 0) = 0
  AND (
      EXISTS (
          SELECT 1
          FROM default_grant_groups dgg
          WHERE dgg.group_id = g.id
      )
      OR (
          EXISTS (
              SELECT 1
              FROM user_subscriptions us
              WHERE us.group_id = g.id
                AND us.deleted_at IS NULL
          )
          AND NOT EXISTS (
              SELECT 1
              FROM subscription_plans sp
              WHERE sp.group_id = g.id
                AND sp.for_sale = TRUE
          )
      )
  );

UPDATE groups g
SET weekly_limit_usd = 100,
    monthly_limit_usd = NULL,
    default_validity_days = LEAST(g.default_validity_days, 7),
    updated_at = NOW()
WHERE g.id IN (SELECT id FROM tmp_weekly_grant_target_groups);

-- Keep future default grants at 7 days for those same groups.
WITH rewritten_settings AS (
    SELECT
        s.key,
        jsonb_agg(
            CASE
                WHEN (elem.item ->> 'group_id') ~ '^[0-9]+$'
                     AND (elem.item ->> 'group_id')::BIGINT IN (SELECT id FROM tmp_weekly_grant_target_groups)
                THEN jsonb_set(elem.item, '{validity_days}', '7'::jsonb, TRUE)
                ELSE elem.item
            END
            ORDER BY elem.ord
        ) AS value
    FROM settings s
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE
            WHEN btrim(s.value) LIKE '[%' THEN s.value::jsonb
            ELSE '[]'::jsonb
        END
    ) WITH ORDINALITY AS elem(item, ord)
    WHERE s.key IN (
        'default_subscriptions',
        'auth_source_default_email_subscriptions',
        'auth_source_default_linuxdo_subscriptions',
        'auth_source_default_oidc_subscriptions',
        'auth_source_default_wechat_subscriptions',
        'auth_source_default_github_subscriptions',
        'auth_source_default_google_subscriptions'
    )
    GROUP BY s.key
    HAVING BOOL_OR(
        (elem.item ->> 'group_id') ~ '^[0-9]+$'
        AND (elem.item ->> 'group_id')::BIGINT IN (SELECT id FROM tmp_weekly_grant_target_groups)
    )
)
UPDATE settings s
SET value = rs.value::TEXT,
    updated_at = NOW()
FROM rewritten_settings rs
WHERE s.key = rs.key
  AND s.value IS DISTINCT FROM rs.value::TEXT;

-- Shorten existing bulk/default user grants to at most 7 days from start.
WITH shortened AS (
    SELECT
        us.id,
        LEAST(us.expires_at, us.starts_at + INTERVAL '7 days') AS expires_at
    FROM user_subscriptions us
    WHERE us.deleted_at IS NULL
      AND us.group_id IN (SELECT id FROM tmp_weekly_grant_target_groups)
      AND us.expires_at > us.starts_at + INTERVAL '7 days'
)
UPDATE user_subscriptions us
SET expires_at = shortened.expires_at,
    status = CASE
        WHEN shortened.expires_at <= NOW() THEN 'expired'
        ELSE us.status
    END,
    updated_at = NOW()
FROM shortened
WHERE us.id = shortened.id;
