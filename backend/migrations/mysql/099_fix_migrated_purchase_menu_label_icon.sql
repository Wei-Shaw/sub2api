-- MySQL 方言版本（翻译自 ../099_fix_migrated_purchase_menu_label_icon.sql）
--
-- Fixes the custom menu item created by migration 096: updates the label
-- from hardcoded English "Purchase" to "充值/订阅", and sets the icon_svg
-- to a credit-card SVG matching the sidebar CreditCardIcon.
--
-- Idempotent: only modifies items where id = 'migrated_purchase_subscription'.
--
-- 翻译说明：
--   - DO $$ 匿名块 → 临时存储过程（CALL 后立即 DROP；不写 DELIMITER）。
--   - PL/pgSQL 的 FOR 循环遍历 jsonb 数组定位元素下标 → 改用
--     JSON_SEARCH(v_items, 'one', <id>, NULL, '$[*].id')：命中时返回形如 '$[N].id'
--     的路径，解析出 N 即元素下标（两者均取数组中首个匹配元素，语义等价）。
--   - jsonb_set(v_items, ARRAY[idx,'label'], '"充值/订阅"'::jsonb) /
--     jsonb_set(..., ARRAY[idx,'icon_svg'], to_jsonb(v_icon))
--     → JSON_SET(v_items, CONCAT('$[',N,'].label'), '充值/订阅',
--                CONCAT('$[',N,'].icon_svg'), '<svg ...>')
--     （SQL 字符串实参写入即 JSON 字符串，与 PG 的 jsonb 标量语义一致；
--       JSON_SET 对缺失键会新建，与 jsonb_set 一致）。
--   - RAISE NOTICE 为日志输出，按规则跳过。

DROP PROCEDURE IF EXISTS __migration_099_tmp;
CREATE PROCEDURE __migration_099_tmp()
main: BEGIN
    DECLARE v_raw TEXT;
    DECLARE v_items JSON;
    DECLARE v_path VARCHAR(255);
    DECLARE v_idx INT;

    SELECT value INTO v_raw FROM settings WHERE `key` = 'custom_menu_items';

    IF COALESCE(v_raw, '') = '' OR v_raw = 'null' THEN
        LEAVE main;
    END IF;

    SET v_items = CAST(v_raw AS JSON);

    -- Find the index of the migrated item (JSON_SEARCH returns a path like '$[N].id')
    SET v_path = JSON_SEARCH(v_items, 'one', 'migrated_purchase_subscription', NULL, '$[*].id');

    IF v_path IS NULL THEN
        LEAVE main;  -- item not found, nothing to fix
    END IF;

    SET v_idx = CAST(REPLACE(REPLACE(v_path, '$[', ''), '].id', '') AS UNSIGNED);

    -- Credit card SVG (Heroicons outline, matches CreditCardIcon in AppSidebar)
    -- Update label and icon_svg
    SET v_items = JSON_SET(
        v_items,
        CONCAT('$[', v_idx, '].label'),    '充值/订阅',
        CONCAT('$[', v_idx, '].icon_svg'), '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 002.25-2.25V6.75A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25v10.5A2.25 2.25 0 004.5 19.5z"/></svg>'
    );

    UPDATE settings SET value = v_items WHERE `key` = 'custom_menu_items';
END main;

CALL __migration_099_tmp();
DROP PROCEDURE __migration_099_tmp;
