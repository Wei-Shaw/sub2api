-- Migration: Add device_code column to user_devices and make login_redeem_code_id nullable
-- This supports the device-code-separation refactor where invite-login
-- uses user_devices.device_code directly instead of redeem_codes.

-- 1. Add device_code column (nullable, will be populated for existing devices)
ALTER TABLE user_devices ADD COLUMN IF NOT EXISTS device_code VARCHAR(19);

-- 2. Make login_redeem_code_id nullable (new devices won't have a redeem code)
ALTER TABLE user_devices ALTER COLUMN login_redeem_code_id DROP NOT NULL;

-- 3. Drop the old unique index on login_redeem_code_id (multiple NULLs not allowed with unique)
DROP INDEX IF EXISTS user_devices_login_redeem_code_id_key;

-- 4. Add unique partial index on device_code (only for non-null values)
CREATE UNIQUE INDEX IF NOT EXISTS user_devices_device_code_key
    ON user_devices(device_code) WHERE device_code IS NOT NULL;

-- 5. Backfill device_code from existing redeem_codes (type='device_login')
-- For existing devices that have a login_redeem_code_id, copy the code from redeem_codes
UPDATE user_devices ud
SET device_code = rc.code
FROM redeem_codes rc
WHERE ud.login_redeem_code_id = rc.id
  AND ud.device_code IS NULL
  AND rc.type = 'device_login';

-- 6. For any remaining devices without device_code, generate one from the pattern DLG-XXXX-XXXX-XXXX
-- This uses a deterministic approach based on device id to avoid conflicts
UPDATE user_devices
SET device_code = 'DLG-' ||
    UPPER(SUBSTRING(MD5(id::text || 'device-code-seed') FROM 1 FOR 4)) || '-' ||
    UPPER(SUBSTRING(MD5(id::text || 'device-code-seed') FROM 5 FOR 4)) || '-' ||
    UPPER(SUBSTRING(MD5(id::text || 'device-code-seed') FROM 9 FOR 4))
WHERE device_code IS NULL;
