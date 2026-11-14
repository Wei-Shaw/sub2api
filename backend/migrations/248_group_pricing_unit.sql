ALTER TABLE groups ADD COLUMN IF NOT EXISTS pricing_unit VARCHAR(20) NOT NULL DEFAULT 'usd';
UPDATE groups SET pricing_unit = 'points' WHERE subscription_type = 'subscription';
ALTER TABLE groups ADD CONSTRAINT groups_pricing_unit_check CHECK (pricing_unit IN ('usd', 'points'));
