-- 分组上游订阅档位（仅元数据/展示，可空）
ALTER TABLE groups ADD COLUMN IF NOT EXISTS upstream_plan VARCHAR(64) NULL;
