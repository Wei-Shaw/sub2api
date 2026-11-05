-- Opt-in per-account request-body compression. Off by default: compressed
-- request bodies depend on upstream support, which is negotiated at runtime
-- (br -> zstd -> deflate) and never persisted.
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS compress_request_enabled BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN accounts.compress_request_enabled IS
    'Compress outbound request bodies with a runtime-negotiated Content-Encoding; only the switch is persisted.';
