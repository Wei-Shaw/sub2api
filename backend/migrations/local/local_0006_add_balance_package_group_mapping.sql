-- Add optional group mapping for V-Claw balance packages.
-- The local namespace keeps V-Claw-specific payment package wiring out of upstream migrations.

ALTER TABLE balance_packages
    ADD COLUMN IF NOT EXISTS group_id BIGINT NULL REFERENCES groups(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_balance_packages_group_id ON balance_packages(group_id);
