-- Seed a default group for fresh installs.
INSERT INTO groups (name, description)
SELECT 'default', 'Default group'
WHERE NOT EXISTS (SELECT 1 FROM groups);
