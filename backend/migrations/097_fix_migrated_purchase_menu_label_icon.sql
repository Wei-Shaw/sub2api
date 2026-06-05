-- 097_fix_migrated_purchase_menu_label_icon.sql
--
-- Fixes the custom menu item created by migration 096: updates the label
-- from hardcoded English "Purchase" to "充值/订阅", and sets the icon_svg
-- to a credit-card SVG matching the sidebar CreditCardIcon.
--
-- Idempotent: only modifies items where id = 'migrated_purchase_subscription'.

DO $$
DECLARE
    v_raw   text;
    v_items jsonb;
    v_idx   int;
    v_icon  text;
    v_elem  jsonb;
    v_i     int := 0;
BEGIN
    SELECT value INTO v_raw
      FROM settings WHERE key = 'custom_menu_items';

    IF COALESCE(v_raw, '') = '' OR v_raw = 'null' THEN
        RETURN;
    END IF;

    v_items := v_raw::jsonb;

    -- Find the index of the migrated item by iterating the array
    v_idx := NULL;
    FOR v_elem IN SELECT jsonb_array_elements(v_items) LOOP
        IF v_elem ->> 'id' = 'migrated_purchase_subscription' THEN
            v_idx := v_i;
            EXIT;
        END IF;
        v_i := v_i + 1;
    END LOOP;

    IF v_idx IS NULL THEN
        RETURN;  -- item not found, nothing to fix
    END IF;

    -- Credit card SVG (Heroicons outline, matches CreditCardIcon in AppSidebar)
    v_icon := '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 002.25-2.25V6.75A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25v10.5A2.25 2.25 0 004.5 19.5z"/></svg>';

    -- Update label and icon_svg
    v_items := jsonb_set(v_items, ARRAY[v_idx::text, 'label'],    '"充值/订阅"'::jsonb);
    v_items := jsonb_set(v_items, ARRAY[v_idx::text, 'icon_svg'], to_jsonb(v_icon));

    UPDATE settings SET value = v_items::text WHERE key = 'custom_menu_items';

    RAISE NOTICE '[migration-097] Fixed migrated_purchase_subscription: label=充值/订阅, icon=CreditCard SVG';
END $$;
