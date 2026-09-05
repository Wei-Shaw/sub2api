-- Runtime aggregator reads/writes the fixed rollup caches through the role that
-- executes migrations.  Use CURRENT_USER so custom DATABASE_USER values work;
-- PostgreSQL permits this form in GRANT and it remains idempotent.
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
    channel_monitor_v2_metrics_rollup,
    channel_monitor_v2_user_metrics_rollup,
    channel_monitor_v2_error_metrics_rollup,
    channel_monitor_v2_latency_histograms_rollup
TO CURRENT_USER;
