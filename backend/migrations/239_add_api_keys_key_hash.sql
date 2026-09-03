-- api_keys.key_hash：认证查询改用摘要，明文 key 列本次仍保留。
--
-- 背景（M1/H6）：api_keys.key 明文存储且带唯一约束，GetByKeyForAuth 直接
-- 用 `key = $1` 命中，一份数据库备份泄漏就等于泄漏一批可直接使用的 sk-*。
--
-- 这是两次发布中的第 1 次：本次只新增 key_hash、回填、把认证查询切到摘要；
-- 删除明文 key 列是第 2 次发布的事。分两次是为了保留回滚能力——一次性
-- 删列意味着回滚旧版本后所有 Key 立刻失效。
--
-- 摘要算法选 SHA-256 而非 HMAC：
--   1. 本仓库的密钥库就是数据库本身（security_secrets 表存 JWT 等密钥），
--      HMAC 的 pepper 只能落在同一份 pg_dump 里，对"备份泄漏"这个威胁模型
--      几乎不增加强度；
--   2. 回填必须在纯 SQL 里完成，PostgreSQL 内置 sha256() 无需扩展，
--      而 hmac() 依赖 pgcrypto，且 pepper 会被写进迁移文件与 schema_migrations
--      校验和里，反而制造新的泄漏面；
--   3. 系统生成的 Key 是 32 字节 crypto/rand（256 bit 熵），彩虹表/暴力破解
--      不成立，加盐加胡椒都无意义。
--   残留风险：用户自定义 Key（ValidateCustomKey 最短仅 16 字符）熵可能很低，
--   这类摘要理论上可被离线穷举；这是本次发布明确接受的已知限制。

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS key_hash VARCHAR(64) NOT NULL DEFAULT '';

COMMENT ON COLUMN api_keys.key_hash IS
    'Lowercase hex SHA-256 of api_keys.key; authentication lookups use this column';

-- 回填：sha256()/convert_to()/encode() 均为 PostgreSQL 内置函数，不需要 pgcrypto。
-- key 只含 ASCII（sk- 前缀 + hex，或自定义 Key 的 [A-Za-z0-9_-]），UTF8 编码
-- 后的字节即原始字节，与 Go 侧 sha256.Sum256([]byte(key)) 逐字节一致。
-- 幂等：只回填仍为空串的行；软删除的 tombstone 行同样回填，保证唯一索引可建。
UPDATE api_keys
SET key_hash = encode(sha256(convert_to(key, 'UTF8')), 'hex')
WHERE key_hash = '';

-- 部分唯一索引，而不是整表唯一约束：
-- 滚动升级窗口里，尚未替换的老版本二进制插入 api_keys 时不会写 key_hash，
-- 该行会拿到 DEFAULT ''。整表唯一约束会让第二个这样的行插入失败（直接打断
-- 老实例的建 Key 功能），部分唯一索引则放行空串、只对真实摘要去重。
--
-- key 列本身已是唯一的（含 tombstone），所以摘要必然互不相同，本索引不会
-- 因为存量数据冲突而创建失败。
--
-- 认证查询会显式带上 `key_hash <> ''` 谓词，让规划器能证明部分索引可用，
-- 否则参数化的 `key_hash = $1` 无法蕴含索引谓词，会退化成顺序扫描。
CREATE UNIQUE INDEX IF NOT EXISTS uniq_api_keys_key_hash
    ON api_keys (key_hash)
    WHERE key_hash <> '';
