-- 模型介绍表：管理员可为每个模型配置介绍文案、默认参数、封面图。
-- 与"渠道定价 (channel_model_pricings)"表解耦；model_key 是对外模型名（如
-- "bytedance/seedance-2.5/text-to-video"、"gpt-4o"），与定价条目按名字关联。
--
-- 字段说明：
--   * model_key       主键。对外模型名（大小写敏感）。
--   * title           展示标题（可选，缺省用 model_key）。
--   * description     纯文本介绍。
--   * cover_url       封面图 URL（http/https，或站内静态资源相对路径）。
--   * default_params  默认参数（JSON 对象）。前端用 KV 编辑器编辑。
--   * sort_order      展示排序，小值靠前。
--   * enabled         是否启用（用户端未来展示时会过滤）。
--   * created_at / updated_at 时间戳。
--
-- forward-only；使用 IF NOT EXISTS 保证幂等。

CREATE TABLE IF NOT EXISTS model_intros (
    model_key       VARCHAR(255) PRIMARY KEY,
    title           VARCHAR(255) NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    cover_url       VARCHAR(1024) NOT NULL DEFAULT '',
    default_params  JSONB NOT NULL DEFAULT '{}'::jsonb,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS modelintros_sort_order_idx
    ON model_intros (sort_order, model_key);

CREATE INDEX IF NOT EXISTS modelintros_enabled_idx
    ON model_intros (enabled);
