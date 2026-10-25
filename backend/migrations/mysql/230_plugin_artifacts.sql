-- MySQL 方言版本（翻译自 ../230_plugin_artifacts.sql）
-- 翻译说明：PG BYTEA → MySQL LONGBLOB（规则文档类型映射表未列出 BYTEA；
--           插件原始签名包体积上限未知，取容量最大的 LONGBLOB 以保安全）。

-- 保存经过签名校验的原始插件包，供多实例和无状态节点重新复验、解包。
-- 旧安装记录允许暂时为空；管理员重新上传该插件后会自动补齐。
-- TODO-REVIEW: PG BYTEA 无既定映射，此处选 LONGBLOB（MEDIUMBLOB 上限 16MB，
-- 可能不足以容纳插件包）；若确认插件包远小于 16MB，可改用 MEDIUMBLOB 省内存。
ALTER TABLE sub2api_plugin_installations
    ADD COLUMN artifact_data LONGBLOB NULL;
