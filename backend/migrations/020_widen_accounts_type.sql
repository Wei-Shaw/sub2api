-- Widen accounts.type column to support longer type names (e.g. "apikey-chat-completions" = 23 chars)
ALTER TABLE accounts ALTER COLUMN type TYPE VARCHAR(40);
