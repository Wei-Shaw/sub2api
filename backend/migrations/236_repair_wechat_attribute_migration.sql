-- 修复 019_migrate_wechat_to_attributes.sql 的自我回滚。
--
-- 019 曾包含 goose Down 块，但迁移运行器
-- （internal/repository/migrations_runner.go）不解析 goose 指令：整个文件在同一个
-- 事务里一次性执行，因此 Down 块紧接着 Up 块运行，把刚迁移好的数据又还原了回去：
--   * wechat 属性定义被软删除（deleted_at 置位）
--   * user_attribute_values 中的 wechat 取值被删除
--   * users.wechat 列被重新加回并写入旧值
--
-- 019 已清理 goose 标记，新装库不再受影响；本迁移用于修复已经记录过 019 的旧库。
-- 全部操作幂等，可重复执行。

-- Step 1: 恢复被误软删除的 wechat 属性定义（仅当当前没有生效定义时）
UPDATE user_attribute_definitions
SET deleted_at = NULL,
    updated_at = NOW()
WHERE id = (
    SELECT id
    FROM user_attribute_definitions
    WHERE key = 'wechat' AND deleted_at IS NOT NULL
    ORDER BY id DESC
    LIMIT 1
)
AND NOT EXISTS (
    SELECT 1 FROM user_attribute_definitions WHERE key = 'wechat' AND deleted_at IS NULL
);

-- Step 2: 定义彻底缺失时补建（与 019 Up Step 1 保持一致）
INSERT INTO user_attribute_definitions (key, name, description, type, options, required, validation, placeholder, display_order, enabled, created_at, updated_at)
SELECT 'wechat', '微信', '用户微信号', 'text', '[]'::jsonb, false, '{}'::jsonb, '请输入微信号', 0, true, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM user_attribute_definitions WHERE key = 'wechat' AND deleted_at IS NULL
);

-- Step 3: 把 Down 块回填到 users.wechat 的取值重新搬回 user_attribute_values。
-- users.wechat 只在受影响的旧库上存在（修好后的 019 会 DROP 掉它），因此必须用动态
-- SQL，否则新装库解析该语句时会因列不存在而失败。
DO $repair$
DECLARE
    v_attr_id BIGINT;
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'users'
          AND column_name = 'wechat'
    ) THEN
        RETURN;
    END IF;

    SELECT id INTO v_attr_id
    FROM user_attribute_definitions
    WHERE key = 'wechat' AND deleted_at IS NULL
    LIMIT 1;

    IF v_attr_id IS NULL THEN
        RETURN;
    END IF;

    EXECUTE format($q$
        INSERT INTO user_attribute_values (user_id, attribute_id, value, created_at, updated_at)
        SELECT u.id, %s, u.wechat, NOW(), NOW()
        FROM users u
        WHERE u.wechat IS NOT NULL
          AND u.wechat <> ''
          AND u.deleted_at IS NULL
        ON CONFLICT (user_id, attribute_id) DO NOTHING
    $q$, v_attr_id);
END
$repair$;
