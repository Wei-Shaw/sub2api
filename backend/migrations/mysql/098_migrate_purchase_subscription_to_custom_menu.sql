-- MySQL 方言版本（翻译自 ../098_migrate_purchase_subscription_to_custom_menu.sql）
--
-- Migrates the legacy purchase_subscription_url setting into custom_menu_items.
-- After migration, purchase_subscription_enabled is set to "false" and
-- purchase_subscription_url is cleared.
--
-- Idempotent: skips if custom_menu_items already contains
-- "migrated_purchase_subscription".
--
-- 翻译说明：
--   - DO $$ 匿名块 → 临时存储过程（CALL 后立即 DROP；服务器端解析，不写 DELIMITER）。
--     PL/pgSQL 的 RETURN 早退 → 标签块 LEAVE main。
--   - 「数组中已存在 id=migrated_purchase_subscription 的元素」检查：
--     jsonb_array_elements + EXISTS → JSON_CONTAINS(v_items, '{"id":...}' AS JSON, '$')
--     （JSON_CONTAINS 对数组目标按「存在任一元素包含候选对象」判定，语义等价）。
--   - v_items || jsonb_build_array(item) → JSON_ARRAY_APPEND(v_items, '$', item)。
--   - jsonb_build_object → JSON_OBJECT（sort_order 数值语义一致）。
--   - ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
--     → ON DUPLICATE KEY UPDATE value = VALUES(value)。
--   - settings.key 为 MySQL 保留字（KEY），一律写作 `key`。
--   - RAISE NOTICE 为日志输出，按规则跳过。

DROP PROCEDURE IF EXISTS __migration_098_tmp;
CREATE PROCEDURE __migration_098_tmp()
main: BEGIN
    DECLARE v_enabled TEXT;
    DECLARE v_url TEXT;
    DECLARE v_raw TEXT;
    DECLARE v_items JSON;
    DECLARE v_new_item JSON;

    -- Read legacy settings
    SELECT value INTO v_enabled FROM settings WHERE `key` = 'purchase_subscription_enabled';
    SELECT value INTO v_url FROM settings WHERE `key` = 'purchase_subscription_url';

    -- Skip if not enabled or URL is empty
    IF COALESCE(v_enabled, '') <> 'true' OR COALESCE(TRIM(v_url), '') = '' THEN
        LEAVE main;
    END IF;

    -- Read current custom_menu_items
    SELECT value INTO v_raw FROM settings WHERE `key` = 'custom_menu_items';

    IF COALESCE(v_raw, '') = '' OR v_raw = 'null' THEN
        SET v_items = CAST('[]' AS JSON);
    ELSE
        SET v_items = CAST(v_raw AS JSON);
    END IF;

    -- Skip if already migrated (item with id "migrated_purchase_subscription" exists)
    IF JSON_CONTAINS(v_items, CAST('{"id": "migrated_purchase_subscription"}' AS JSON), '$') THEN
        LEAVE main;
    END IF;

    -- Build the new menu item
    SET v_new_item = JSON_OBJECT(
        'id',         'migrated_purchase_subscription',
        'label',      'Purchase',
        'icon_svg',   '',
        'url',        TRIM(v_url),
        'visibility', 'user',
        'sort_order', 100
    );

    -- Append to array
    SET v_items = JSON_ARRAY_APPEND(v_items, '$', v_new_item);

    -- Upsert custom_menu_items
    INSERT INTO settings (`key`, value)
    VALUES ('custom_menu_items', v_items)
    ON DUPLICATE KEY UPDATE value = VALUES(value);

    -- Clear legacy settings
    UPDATE settings SET value = 'false' WHERE `key` = 'purchase_subscription_enabled';
    UPDATE settings SET value = ''      WHERE `key` = 'purchase_subscription_url';
END main;

CALL __migration_098_tmp();
DROP PROCEDURE __migration_098_tmp;
