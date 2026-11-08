-- 保存经过签名校验的原始插件包，供多实例和无状态节点重新复验、解包。
-- 旧安装记录允许暂时为空；管理员重新上传该插件后会自动补齐。
ALTER TABLE sub2api_plugin_installations
    ADD COLUMN IF NOT EXISTS artifact_data BYTEA;
