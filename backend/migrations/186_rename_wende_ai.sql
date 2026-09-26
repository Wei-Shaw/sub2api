-- Rename brand from 沃德AI to 稳得AI.
-- Only touch rows still carrying the old default; operator overrides are untouched.
UPDATE settings
SET value = '稳得AI', updated_at = NOW()
WHERE key = 'site_name' AND value = '沃德AI';
