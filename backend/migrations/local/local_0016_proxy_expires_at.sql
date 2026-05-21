-- Migration: local_0016_proxy_expires_at
-- Add expires_at column to proxies table for tracking proxy expiration.
-- Nullable: existing proxies without expiration remain NULL (never expires).

ALTER TABLE proxies ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

-- Index for efficient near-expiry queries
CREATE INDEX IF NOT EXISTS idx_proxies_expires_at ON proxies (expires_at) WHERE expires_at IS NOT NULL AND deleted_at IS NULL;
