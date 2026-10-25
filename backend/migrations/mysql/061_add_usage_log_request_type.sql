-- MySQL 方言版本（翻译自 ../061_add_usage_log_request_type.sql）
-- Add request_type enum for usage_logs while keeping legacy stream/openai_ws_mode compatibility.
ALTER TABLE usage_logs
    ADD COLUMN request_type SMALLINT NOT NULL DEFAULT 0;

-- PG DO 块（pg_constraint 存在性检查 + ADD CONSTRAINT）→ 执行器保证单次执行，直接添加。
-- 注意：CHECK 约束需 MySQL 8.0.16+ 才强制执行；8.0.13~8.0.15 解析但忽略。
ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_request_type_check
    CHECK (request_type IN (0, 1, 2, 3));

CREATE INDEX idx_usage_logs_request_type_created_at
    ON usage_logs (request_type, created_at);

-- Backfill from legacy fields in bounded batches.
-- Why bounded:
-- 1) Full-table UPDATE on large usage_logs can block startup for a long time.
-- 2) request_type=0 rows remain query-compatible via legacy fallback logic
--    (stream/openai_ws_mode) in repository filters.
-- 3) Subsequent writes will use explicit request_type and gradually dilute
--    historical unknown rows.
--
-- openai_ws_mode has higher priority than stream.
--
-- 翻译说明：PG DO 块（LOOP 有界批次回填）→ 临时存储过程改写；
--           RAISE NOTICE 为日志输出，跳过；UPDATE ... FROM batch → UPDATE ... JOIN (SELECT ...)。
--           LIMIT 批大小使用常量 5000（与 PG 版 v_batch_size 一致）。
CREATE PROCEDURE __mig_tmp()
BEGIN
    DECLARE v_rows INT DEFAULT 0;
    DECLARE v_total_rows INT DEFAULT 0;
    DECLARE v_started_at DATETIME(3) DEFAULT NOW(3);
    backfill_loop: LOOP
        UPDATE usage_logs ul
        JOIN (
            SELECT id
            FROM usage_logs
            WHERE request_type = 0
            ORDER BY id
            LIMIT 5000
        ) batch ON ul.id = batch.id
        SET ul.request_type = CASE
            WHEN ul.openai_ws_mode = TRUE THEN 3
            WHEN ul.stream = TRUE THEN 2
            ELSE 1
        END;

        SET v_rows = ROW_COUNT();
        IF v_rows = 0 THEN
            LEAVE backfill_loop;
        END IF;

        SET v_total_rows = v_total_rows + v_rows;
        IF TIMESTAMPDIFF(MICROSECOND, v_started_at, NOW(3)) >= 8000000 THEN
            LEAVE backfill_loop;
        END IF;
    END LOOP;
END;
CALL __mig_tmp();
DROP PROCEDURE __mig_tmp;
