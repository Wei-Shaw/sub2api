-- Backfill the reported 1.0-rate OpenAI Sol rows written before the hidden
-- 2.0 multiplier was included in the stored base costs. actual_cost and
-- rate_multiplier were already correct and deliberately remain unchanged.
--
-- There is no durable wire-override marker on old usage rows. Restrict this
-- repair to today's specific persisted fingerprint: displayed rate 1.0 with
-- actual_cost approximately twice total_cost. This intentionally leaves other
-- displayed rates untouched because some normal per-request billing modes use
-- a different effective multiplier without a durable usage-log marker.
UPDATE usage_logs AS ul
SET
    input_cost = ul.input_cost * 2.0,
    image_input_cost = ul.image_input_cost * 2.0,
    output_cost = ul.output_cost * 2.0,
    image_output_cost = ul.image_output_cost * 2.0,
    cache_creation_cost = ul.cache_creation_cost * 2.0,
    cache_read_cost = ul.cache_read_cost * 2.0,
    total_cost = ul.total_cost * 2.0,
    account_stats_cost = CASE
        WHEN ul.account_stats_cost IS NULL THEN NULL
        ELSE ul.account_stats_cost * 2.0
    END
FROM accounts AS a
WHERE a.id = ul.account_id
  AND a.platform = 'openai'
  AND COALESCE(NULLIF(BTRIM(ul.upstream_model), ''), BTRIM(ul.model)) = 'gpt-5.6-sol'
  AND ul.created_at >= date_trunc('day', CURRENT_TIMESTAMP)
  AND ul.created_at < date_trunc('day', CURRENT_TIMESTAMP) + INTERVAL '1 day'
  AND ul.total_cost > 0
  AND ul.rate_multiplier = 1.0
  AND ul.actual_cost >= ul.total_cost * 1.999999
  AND ul.actual_cost <= ul.total_cost * 2.000001;

-- The application connection uses the configured server timezone, matching the
-- bucket boundaries used by dashboard aggregation.
WITH today_hourly AS (
    SELECT
        date_trunc('hour', created_at) AS bucket_start,
        COALESCE(SUM(total_cost), 0) AS total_cost,
        COALESCE(SUM(actual_cost), 0) AS actual_cost,
        COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) AS account_cost
    FROM usage_logs
    WHERE created_at >= date_trunc('day', CURRENT_TIMESTAMP)
      AND created_at < date_trunc('day', CURRENT_TIMESTAMP) + INTERVAL '1 day'
    GROUP BY 1
)
UPDATE usage_dashboard_hourly AS h
SET
    total_cost = today_hourly.total_cost,
    actual_cost = today_hourly.actual_cost,
    account_cost = today_hourly.account_cost,
    computed_at = NOW()
FROM today_hourly
WHERE h.bucket_start = today_hourly.bucket_start;

WITH today AS (
    SELECT
        COALESCE(SUM(total_cost), 0) AS total_cost,
        COALESCE(SUM(actual_cost), 0) AS actual_cost,
        COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) AS account_cost
    FROM usage_logs
    WHERE created_at >= date_trunc('day', CURRENT_TIMESTAMP)
      AND created_at < date_trunc('day', CURRENT_TIMESTAMP) + INTERVAL '1 day'
)
UPDATE usage_dashboard_daily AS d
SET
    total_cost = today.total_cost,
    actual_cost = today.actual_cost,
    account_cost = today.account_cost,
    computed_at = NOW()
FROM today
WHERE d.bucket_date = CURRENT_DATE;
