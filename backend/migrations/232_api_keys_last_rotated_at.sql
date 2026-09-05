-- Track whether a credential has been rotated so ordinary read endpoints can
-- return only a masked value after the one-time rotation response.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS last_rotated_at TIMESTAMPTZ;
