-- Rebrand only installations that still use the historical default name.
-- Operator-defined site names remain untouched.
UPDATE settings
SET value = '沃德AI', updated_at = NOW()
WHERE key = 'site_name' AND value = 'Sub2API';
