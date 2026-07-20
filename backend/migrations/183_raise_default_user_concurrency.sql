-- Raise the default concurrency for newly created users and migrate existing
-- users that still have the legacy low default.
UPDATE settings
SET value = '50', updated_at = NOW()
WHERE (
    key = 'default_concurrency'
    OR key LIKE 'auth_source_default_%_concurrency'
  )
  AND value IN ('1', '2', '3', '4', '5', '6', '7', '8', '9');

ALTER TABLE users
    ALTER COLUMN concurrency SET DEFAULT 50;

UPDATE users
SET concurrency = 50,
    updated_at = NOW()
WHERE concurrency < 10;