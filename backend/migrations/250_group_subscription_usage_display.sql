ALTER TABLE groups ADD COLUMN IF NOT EXISTS subscription_usage_display VARCHAR(20) NOT NULL DEFAULT 'amount';
-- Preserve the previous display preference for existing subscription groups.
UPDATE groups SET subscription_usage_display = 'percentage'
WHERE subscription_type = 'subscription'
  AND EXISTS (SELECT 1 FROM settings WHERE key = 'subscription_usage_display' AND value = 'percentage');
ALTER TABLE groups ADD CONSTRAINT groups_subscription_usage_display_check
  CHECK (subscription_usage_display IN ('amount', 'percentage'));
