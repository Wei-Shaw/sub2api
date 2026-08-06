-- Restore the Sol costs changed by migration 194. The Sol-to-Terra wire-model
-- override must use Sol pricing with the ordinary configured multiplier.
--
-- Migration 194 doubled the selected rows' stored costs while leaving
-- actual_cost unchanged. Its recorded applied_at timestamp lets this repair
-- exclude rows written after that migration, which already use the corrected
-- pricing rule.
WITH cutoff AS (
    SELECT applied_at
    FROM schema_migrations
    WHERE filename = '194_openai_sol_hidden_billing_backfill.sql'
)
UPDATE usage_logs AS ul
SET
    input_cost = ul.input_cost / 2.0,
    image_input_cost = ul.image_input_cost / 2.0,
    output_cost = ul.output_cost / 2.0,
    image_output_cost = ul.image_output_cost / 2.0,
    cache_creation_cost = ul.cache_creation_cost / 2.0,
    cache_read_cost = ul.cache_read_cost / 2.0,
    total_cost = ul.total_cost / 2.0,
    account_stats_cost = CASE
        WHEN ul.account_stats_cost IS NULL THEN NULL
        ELSE ul.account_stats_cost / 2.0
    END
FROM accounts AS a
CROSS JOIN cutoff
WHERE a.id = ul.account_id
  AND a.platform = 'openai'
  AND COALESCE(NULLIF(BTRIM(ul.upstream_model), ''), BTRIM(ul.model)) = 'gpt-5.6-sol'
  AND ul.created_at >= date_trunc('day', cutoff.applied_at)
  AND ul.created_at < cutoff.applied_at
  AND ul.total_cost > 0
  AND ul.rate_multiplier = 1.0
  AND ul.actual_cost >= ul.total_cost * 0.999999
  AND ul.actual_cost <= ul.total_cost * 1.000001;

WITH cutoff AS (
    SELECT date_trunc('day', applied_at) AS day_start
    FROM schema_migrations
    WHERE filename = '194_openai_sol_hidden_billing_backfill.sql'
), hourly AS (
    SELECT
        date_trunc('hour', ul.created_at) AS bucket_start,
        COALESCE(SUM(ul.total_cost), 0) AS total_cost,
        COALESCE(SUM(ul.actual_cost), 0) AS actual_cost,
        COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) AS account_cost
    FROM usage_logs AS ul
    CROSS JOIN cutoff
    WHERE ul.created_at >= cutoff.day_start
      AND ul.created_at < cutoff.day_start + INTERVAL '1 day'
    GROUP BY 1
)
UPDATE usage_dashboard_hourly AS h
SET
    total_cost = hourly.total_cost,
    actual_cost = hourly.actual_cost,
    account_cost = hourly.account_cost,
    computed_at = NOW()
FROM hourly
WHERE h.bucket_start = hourly.bucket_start;

WITH cutoff AS (
    SELECT date_trunc('day', applied_at) AS day_start
    FROM schema_migrations
    WHERE filename = '194_openai_sol_hidden_billing_backfill.sql'
), day_cost AS (
    SELECT
        COALESCE(SUM(ul.total_cost), 0) AS total_cost,
        COALESCE(SUM(ul.actual_cost), 0) AS actual_cost,
        COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) AS account_cost
    FROM usage_logs AS ul
    CROSS JOIN cutoff
    WHERE ul.created_at >= cutoff.day_start
      AND ul.created_at < cutoff.day_start + INTERVAL '1 day'
)
UPDATE usage_dashboard_daily AS d
SET
    total_cost = day_cost.total_cost,
    actual_cost = day_cost.actual_cost,
    account_cost = day_cost.account_cost,
    computed_at = NOW()
FROM cutoff, day_cost
WHERE d.bucket_date = cutoff.day_start::date;
