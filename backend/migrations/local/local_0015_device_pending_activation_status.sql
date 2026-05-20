-- Migration: Allow 'pending_activation' status for user_devices
-- Required by V-Claw device activation approval flow (PR #38)
-- The device claim service creates devices with status='pending_activation'
-- which are later activated by admin via POST /admin/users/:id/activate-devices

ALTER TABLE user_devices DROP CONSTRAINT IF EXISTS user_devices_status_check;
ALTER TABLE user_devices ADD CONSTRAINT user_devices_status_check
    CHECK (status IN ('active', 'pending_activation', 'revoked', 'blocked'));
