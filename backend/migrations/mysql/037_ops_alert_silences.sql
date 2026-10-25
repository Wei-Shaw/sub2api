-- MySQL 方言版本（翻译自 ../037_ops_alert_silences.sql）
-- （goose 指令注释：+goose Up/Down/StatementBegin/StatementEnd，非 SQL 语义，翻译时去掉；
--   Down 段为 DROP TABLE IF EXISTS，MySQL 语法相同，不再重复保留。）
-- Ops alert silences: scoped (rule_id + platform + group_id + region)

CREATE TABLE IF NOT EXISTS ops_alert_silences (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,

    rule_id BIGINT NOT NULL,
    platform VARCHAR(64) NOT NULL,
    group_id BIGINT,
    region VARCHAR(64),

    `until` DATETIME(3) NOT NULL,
    reason TEXT NULL,

    created_by BIGINT,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_ops_alert_silences_lookup
    ON ops_alert_silences (rule_id, platform, group_id, region, `until`);
