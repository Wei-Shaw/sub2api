-- 修正 schema_migrations 中“本地改名”的迁移文件名
-- 适用场景：你已执行过旧文件名的迁移，合并后仅改了自己这边的文件名

BEGIN;

UPDATE schema_migrations
SET filename = '042b_add_ops_system_metrics_switch_count.sql'
WHERE filename = '042_add_ops_system_metrics_switch_count.sql'
  AND NOT EXISTS (
    SELECT 1 FROM schema_migrations WHERE filename = '042b_add_ops_system_metrics_switch_count.sql'
  );

UPDATE schema_migrations
SET filename = '043b_add_group_invalid_request_fallback.sql'
WHERE filename = '043_add_group_invalid_request_fallback.sql'
  AND NOT EXISTS (
    SELECT 1 FROM schema_migrations WHERE filename = '043b_add_group_invalid_request_fallback.sql'
  );

UPDATE schema_migrations
SET filename = '044b_add_group_mcp_xml_inject.sql'
WHERE filename = '044_add_group_mcp_xml_inject.sql'
  AND NOT EXISTS (
    SELECT 1 FROM schema_migrations WHERE filename = '044b_add_group_mcp_xml_inject.sql'
  );

UPDATE schema_migrations
SET filename = '046b_add_group_supported_model_scopes.sql'
WHERE filename = '046_add_group_supported_model_scopes.sql'
  AND NOT EXISTS (
    SELECT 1 FROM schema_migrations WHERE filename = '046b_add_group_supported_model_scopes.sql'
  );

COMMIT;
