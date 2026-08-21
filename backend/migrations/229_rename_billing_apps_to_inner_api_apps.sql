-- 将历史余额 RPC 的接入方表升级为通用内部 API RPC 接入方表。
-- 不修改历史迁移 161；当前部署无线上接入方和历史数据，直接改名即可。
ALTER TABLE IF EXISTS billing_apps RENAME TO inner_api_apps;

ALTER INDEX IF EXISTS billing_apps_app_id_key RENAME TO inner_api_apps_app_id_key;
ALTER INDEX IF EXISTS billing_apps_enabled_idx RENAME TO inner_api_apps_enabled_idx;
ALTER SEQUENCE IF EXISTS billing_apps_id_seq RENAME TO inner_api_apps_id_seq;

ALTER TABLE IF EXISTS inner_api_apps
    ADD COLUMN IF NOT EXISTS permissions JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS inner_api_apps_enabled_idx ON inner_api_apps (enabled);
