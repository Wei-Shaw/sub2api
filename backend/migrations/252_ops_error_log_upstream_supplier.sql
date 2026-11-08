-- 为 ops_error_logs 增加「上游商户」两列。
--
-- 背景：渠道监控的失败原本既不出现在「错误请求」页，也无从知道是哪家上游商户
-- 把请求打回的，管理员看到失败却无从处置（想拉黑商户都没有依据）。
--
-- 数据来源与时机：A6 的失败类日志（GET /api/log/self?type=5）里带
-- marketplace_supplier_id / marketplace_supplier_name。⚠️ 该类日志保留量极小
-- （实测约 405 条，而消费日志 4.5 万+），过期即查不到，因此必须在失败后
-- 尽快回填；查不到时这两列为空是正常的，不代表写入逻辑有问题。
--
-- 网络层失败（连接被重置/超时等）请求根本没到达上游，取不到商户信息，同样留空。
--
-- 不加索引：这两列用于详情页展示，不参与筛选与排序，加了只会拖慢这张高写入表。

ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS upstream_supplier_id INTEGER;
ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS upstream_supplier_name TEXT;
-- 上游请求 ID：监控探针失败那一刻还查不到商户（要再打一次上游接口），
-- 先把 ID 存下来，由对账循环随后按它反查并回填上面两列。
-- 宽度与 usage_logs.upstream_request_id 保持一致。
ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS upstream_request_id VARCHAR(128);

COMMENT ON COLUMN ops_error_logs.upstream_supplier_id IS
  '打回该请求的上游商户 ID（A6 marketplace_supplier_id）；网络层失败或超出上游失败日志保留窗口时为空';
COMMENT ON COLUMN ops_error_logs.upstream_supplier_name IS
  '上游商户名，冗余存储供页面直接显示';
COMMENT ON COLUMN ops_error_logs.upstream_request_id IS
  '上游返回的请求 ID；对账循环据此反查上游商户并回填 upstream_supplier_id/name';
