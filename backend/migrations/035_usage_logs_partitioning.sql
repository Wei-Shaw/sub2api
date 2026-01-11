-- usage_logs monthly partition bootstrap.
-- Only creates partitions when usage_logs is already partitioned.
-- Converting usage_logs to a partitioned table requires a manual migration plan.

DO $$
DECLARE
    is_partitioned BOOLEAN := FALSE;
    has_data BOOLEAN := FALSE;
    month_start DATE;
    prev_month DATE;
    next_month DATE;
BEGIN
    SELECT EXISTS(
        SELECT 1
        FROM pg_partitioned_table pt
        JOIN pg_class c ON c.oid = pt.partrelid
        WHERE c.relname = 'usage_logs'
    ) INTO is_partitioned;

    IF NOT is_partitioned THEN
        SELECT EXISTS(SELECT 1 FROM usage_logs LIMIT 1) INTO has_data;
        IF NOT has_data THEN
            -- Automatic conversion is intentionally skipped; see manual migration plan.
            RAISE NOTICE 'usage_logs is not partitioned; skip automatic partitioning';
        END IF;
    END IF;

    IF is_partitioned THEN
        month_start := date_trunc('month', now() AT TIME ZONE 'UTC')::date;
        prev_month := (month_start - INTERVAL '1 month')::date;
        next_month := (month_start + INTERVAL '1 month')::date;

        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS usage_logs_%s PARTITION OF usage_logs FOR VALUES FROM (%L) TO (%L)',
            to_char(prev_month, 'YYYYMM'),
            prev_month,
            month_start
        );

        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS usage_logs_%s PARTITION OF usage_logs FOR VALUES FROM (%L) TO (%L)',
            to_char(month_start, 'YYYYMM'),
            month_start,
            next_month
        );

        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS usage_logs_%s PARTITION OF usage_logs FOR VALUES FROM (%L) TO (%L)',
            to_char(next_month, 'YYYYMM'),
            next_month,
            (next_month + INTERVAL '1 month')::date
        );
    END IF;
END $$;
