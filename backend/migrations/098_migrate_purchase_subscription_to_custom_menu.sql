-- 096_migrate_purchase_subscription_to_custom_menu.sql
--
-- Migrates the legacy purchase_subscription_url setting into custom_menu_items.
-- After migration, purchase_subscription_enabled is set to "false" and
-- purchase_subscription_url is cleared.
--
-- Idempotent: skips if custom_menu_items already contains
-- "migrated_purchase_subscription".

DO $$
DECLARE
    v_enabled  text;
    v_url      text;
    v_raw      text;
    v_items    jsonb;
    v_new_item jsonb;
BEGIN
    -- Read legacy settings
    SELECT value INTO v_enabled
      FROM settings WHERE key = 'purchase_subscription_enabled';
    SELECT value INTO v_url
      FROM settings WHERE key = 'purchase_subscription_url';

    -- Skip if not enabled or URL is empty
    IF COALESCE(v_enabled, '') <> 'true' OR COALESCE(TRIM(v_url), '') = '' THEN
        RETURN;
    END IF;

    -- Read current custom_menu_items
    SELECT value INTO v_raw
      FROM settings WHERE key = 'custom_menu_items';

    IF COALESCE(v_raw, '') = '' OR v_raw = 'null' THEN
        v_items := '[]'::jsonb;
    ELSE
        v_items := v_raw::jsonb;
    END IF;

    -- Skip if already migrated (item with id "migrated_purchase_subscription" exists)
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(v_items) elem
         WHERE elem ->> 'id' = 'migrated_purchase_subscription'
    ) THEN
        RETURN;
    END IF;

    -- Build the new menu item
    v_new_item := jsonb_build_object(
        'id',         'migrated_purchase_subscription',
        'label',      'Purchase',
        'icon_svg',   '',
        'url',        TRIM(v_url),
        'visibility', 'user',
        'sort_order', 100
    );

    -- Append to array
    v_items := v_items || jsonb_build_array(v_new_item);

    -- Upsert custom_menu_items
    INSERT INTO settings (key, value)
    VALUES ('custom_menu_items', v_items::text)
    ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

    -- Clear legacy settings
    UPDATE settings SET value = 'false' WHERE key = 'purchase_subscription_enabled';
    UPDATE settings SET value = ''      WHERE key = 'purchase_subscription_url';

    RAISE NOTICE '[migration-096] Migrated purchase_subscription_url (%) to custom_menu_items', v_url;
END $$;
