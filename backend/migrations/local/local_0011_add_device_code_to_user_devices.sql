-- Migration: Add device_code column to user_devices table
-- This separates device identity from the redeem_codes table.
-- device_code stores the DLG-XXXX-XXXX-XXXX value directly on user_devices.

-- Step 1: Add nullable device_code column
ALTER TABLE user_devices ADD COLUMN IF NOT EXISTS device_code VARCHAR(20);

-- Step 2: Backfill from redeem_codes (login_redeem_code)
UPDATE user_devices ud
SET device_code = rc.code
FROM redeem_codes rc
WHERE ud.login_redeem_code_id = rc.id
  AND ud.device_code IS NULL;

-- Step 3: Create unique index
CREATE UNIQUE INDEX IF NOT EXISTS user_devices_device_code_key ON user_devices(device_code) WHERE device_code IS NOT NULL;
