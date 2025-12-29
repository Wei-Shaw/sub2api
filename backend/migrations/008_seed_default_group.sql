-- Seed a default group for fresh installs.
INSERT INTO groups (name, description, created_at, updated_at)
SELECT 'default', 'Default group', NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM groups);
