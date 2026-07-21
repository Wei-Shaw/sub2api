-- 通用信箱（general-inbox）：把工单站内通知泛化为账号级"消息信箱"，同时支持
-- 单播（direct）与广播（broadcast）两类消息，客户端通过 WebSocket 实时接收 +
-- catchup 拉取补齐。设计文档见 openspec/changes/general-inbox/design.md。
--
-- 三张表：
--   1) direct_messages   —— 单播（fan-out on write，每用户每消息一行）
--   2) broadcasts        —— 广播（只存一份，fan-out on read，读取时按 targeting 匹配）
--   3) user_inbox_state  —— 每用户累积 ack 水位（首次访问懒初始化到"当前时刻"seq）
--
-- 关键设计点：
--   - seq 不使用 BIGSERIAL，而由应用层通过 Redis（TIME + INCR，ms<<20 | incr）分配，
--     单播与广播共享同一全局 seq 数轴，因此 seq 直接作为主键（无 DEFAULT）。
--   - 允许 seq 有洞：dedup 命中时会浪费一个 seq，客户端不依赖 seq 连续性，靠推送携带的
--     unacked_list 做 gap 探测。
--   - payload 限制 <= 8KB，避免大对象写入信箱。
--   - 前向迁移：单文件建三张新表 + 索引；forward-only，回滚 SQL 见
--     backend/migrations/README.md（DROP TABLE user_inbox_state, broadcasts, direct_messages;）。

-- ============================================================================
-- 1) 单播表：per-(user, message) 单行，fan-out on write
-- ============================================================================
CREATE TABLE IF NOT EXISTS direct_messages (
    seq         BIGINT       PRIMARY KEY,
    user_id     BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    namespace   VARCHAR(64)  NOT NULL,
    dedup_key   VARCHAR(128) NOT NULL,
    payload     JSONB        NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_direct_messages_user_ns_dedup UNIQUE (user_id, namespace, dedup_key),
    CONSTRAINT ck_direct_messages_payload_size CHECK (octet_length(payload::text) <= 8192)
);

-- catchup 主查询：WHERE user_id = $1 AND seq > $2 ORDER BY seq LIMIT N。
CREATE INDEX IF NOT EXISTS idx_direct_messages_user_seq
    ON direct_messages (user_id, seq);

-- 30 天保留期清理：DELETE WHERE created_at < cutoff。
CREATE INDEX IF NOT EXISTS idx_direct_messages_created
    ON direct_messages (created_at);

COMMENT ON TABLE  direct_messages            IS '通用信箱单播消息（每用户每消息一行，fan-out on write）';
COMMENT ON COLUMN direct_messages.seq        IS '全局单调 seq，由应用层 Redis 分配（ms<<20|incr），单播/广播共享数轴';
COMMENT ON COLUMN direct_messages.user_id    IS '接收者用户 ID（用户注销时级联清理）';
COMMENT ON COLUMN direct_messages.namespace  IS '业务域，如 support_ticket / affiliate / system';
COMMENT ON COLUMN direct_messages.dedup_key  IS '业务方生成的幂等键；(user_id, namespace, dedup_key) 唯一';
COMMENT ON COLUMN direct_messages.payload    IS '消息内容 JSON（<=8KB），由业务方定义结构';

-- ============================================================================
-- 2) 广播表：全局只存一份，fan-out on read
-- ============================================================================
CREATE TABLE IF NOT EXISTS broadcasts (
    seq         BIGINT       PRIMARY KEY,
    namespace   VARCHAR(64)  NOT NULL,
    dedup_key   VARCHAR(128) NOT NULL,
    targeting   JSONB        NOT NULL,
    payload     JSONB        NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_broadcasts_ns_dedup UNIQUE (namespace, dedup_key),
    CONSTRAINT ck_broadcasts_payload_size CHECK (octet_length(payload::text) <= 8192)
);

-- catchup 广播分支：WHERE seq > $1 AND created_at > cutoff ORDER BY seq LIMIT N。
-- 主键 (seq) 已覆盖 seq 范围扫描，此处补 created_at 索引服务保留期清理。
CREATE INDEX IF NOT EXISTS idx_broadcasts_created
    ON broadcasts (created_at);

COMMENT ON TABLE  broadcasts            IS '通用信箱广播消息（全局一份，读取时按 targeting 匹配用户，fan-out on read）';
COMMENT ON COLUMN broadcasts.seq        IS '全局单调 seq，与 direct_messages 共享同一数轴';
COMMENT ON COLUMN broadcasts.namespace  IS '业务域，如 system / announcement';
COMMENT ON COLUMN broadcasts.dedup_key  IS '业务方生成的幂等键；(namespace, dedup_key) 唯一';
COMMENT ON COLUMN broadcasts.targeting  IS 'JSON 属性过滤表达式（all_users / equals / in / and / or），读取时在应用层求值';
COMMENT ON COLUMN broadcasts.payload    IS '消息内容 JSON（<=8KB）';

-- ============================================================================
-- 3) 用户信箱状态：单一累积 ack 水位
-- ============================================================================
CREATE TABLE IF NOT EXISTS user_inbox_state (
    user_id     BIGINT      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    acked_seq   BIGINT      NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE  user_inbox_state           IS '每用户信箱累积 ack 水位（首次访问懒初始化到当前时刻 seq）';
COMMENT ON COLUMN user_inbox_state.acked_seq IS '累积已确认水位：seq <= acked_seq 视为已读；单调抬升（GREATEST）';
