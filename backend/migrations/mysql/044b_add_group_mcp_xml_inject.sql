-- MySQL 方言版本（翻译自 ../044b_add_group_mcp_xml_inject.sql）

-- Add mcp_xml_inject field to groups table (for antigravity platform)
-- （`groups` 为 MySQL 8 保留字，加反引号）
ALTER TABLE `groups` ADD COLUMN mcp_xml_inject BOOLEAN NOT NULL DEFAULT TRUE;
