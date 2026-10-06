-- Preserve the credential/proxy snapshot admitted for each turn. Older rows
-- remain unversioned and cannot newly enter submitting; recovery never replays.
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS account_updated_at TIMESTAMPTZ;
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS proxy_updated_at TIMESTAMPTZ;
