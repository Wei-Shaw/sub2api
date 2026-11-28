-- 分组级推理强度自适应降档开关：开启后按目标上游模型的
-- supported_reasoning_levels 把越界档位降到支持列表内最高档。
-- 默认关闭，保持历史原样透传行为。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS reasoning_effort_stepdown BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.reasoning_effort_stepdown IS
    '是否按上游模型实际支持的推理强度档位自适应降档；默认关闭';
