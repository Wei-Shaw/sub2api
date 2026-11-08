-- Extend privileges for all Channel Monitor V2 tables (base + rollup + config).
-- Grant to the migration/application role rather than the historical hard-coded
-- "sub2api" role so custom DATABASE_USER deployments work as expected.
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
    channel_monitor_v2_config,
    channel_monitor_v2_watermarks,
    channel_monitor_v2_metrics_1m,
    channel_monitor_v2_user_metrics_1m,
    channel_monitor_v2_error_metrics_1m,
    channel_monitor_v2_latency_histograms_1m,
    channel_monitor_v2_metrics_rollup,
    channel_monitor_v2_user_metrics_rollup,
    channel_monitor_v2_error_metrics_rollup,
    channel_monitor_v2_latency_histograms_rollup
TO CURRENT_USER;
