-- Historical addresses are unknown; capture registration IP only for new users.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS registration_ip VARCHAR(45) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_login_ip VARCHAR(45) NOT NULL DEFAULT '';
