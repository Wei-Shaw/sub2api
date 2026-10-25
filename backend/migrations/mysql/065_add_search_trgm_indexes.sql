-- MySQL 方言版本（翻译自 ../065_add_search_trgm_indexes.sql）
-- Improve admin fuzzy-search performance on large datasets.
-- （PG 版为 DO $$ 块内按 pg_trgm 扩展可用性条件创建 GIN trigram 索引；
--   MySQL 无 pg_trgm，统一改为普通 B-tree 索引：
--   - VARCHAR 列直接索引；
--   - TEXT 列（users.notes）使用 191 前缀长度。）

CREATE INDEX idx_users_email_trgm
    ON users (email);
CREATE INDEX idx_users_username_trgm
    ON users (username);
CREATE INDEX idx_users_notes_trgm
    ON users (notes(191));

CREATE INDEX idx_accounts_name_trgm
    ON accounts (name);

CREATE INDEX idx_api_keys_key_trgm
    ON api_keys (`key`);
CREATE INDEX idx_api_keys_name_trgm
    ON api_keys (name);
