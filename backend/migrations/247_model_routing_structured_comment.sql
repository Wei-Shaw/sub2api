-- Update model_routing column comment for structured rule JSON
-- (legacy pattern -> []account_id remains readable by the application).
COMMENT ON COLUMN groups.model_routing IS '模型路由配置：模型模式 -> {max_concurrency, accounts[{id,priority,concurrency}]}；读兼容旧 []account_id';
