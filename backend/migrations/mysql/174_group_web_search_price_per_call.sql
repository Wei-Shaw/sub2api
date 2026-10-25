-- MySQL 方言版本（翻译自 ../174_group_web_search_price_per_call.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN；`groups` 是 MySQL 8 保留字，加反引号。

-- Codex alpha/search 网页搜索按次计费：分组级单次价格覆盖。
-- NULL 表示使用内置默认价 0.01 USD/次（OpenAI 官方 web search 定价 $10/1000 次）。
ALTER TABLE `groups` ADD COLUMN web_search_price_per_call DECIMAL(20,8) NULL;
