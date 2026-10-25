-- MySQL 方言版本（翻译自 ../235_group_model_allowlist.sql）
-- 翻译说明：
--   - PG DO 块（information_schema 检查 models_list_config 存在且 model_allowlist
--     不存在时才改名）→ 迁移执行器保证单次执行；143 号迁移已添加 models_list_config，
--     此前无任何迁移创建 model_allowlist，条件在执行时点恒成立，
--     去掉存在性分支直接改名；
--   - `groups` 为 MySQL 8 保留字，加反引号；
--   - COMMENT ON COLUMN 整句跳过。

-- 235: groups.models_list_config 更名为 model_allowlist，语义从「仅影响 /v1/models 展示」
-- 升级为分组级模型白名单（同时约束模型列表接口与请求准入）。数据原样保留。
ALTER TABLE `groups` RENAME COLUMN models_list_config TO model_allowlist;

-- (comment skipped) COMMENT ON COLUMN groups.model_allowlist IS
--     'Group model allowlist: constrains both model listing responses and request admission'
