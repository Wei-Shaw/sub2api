-- Add IP restriction fields to api_keys table
-- ip_whitelist: JSON array of allowed IPs/CIDRs (if set, only these IPs can use the key)
-- ip_blacklist: JSON array of blocked IPs/CIDRs (these IPs are always blocked)

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS ip_whitelist JSONB DEFAULT NULL;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS ip_blacklist JSONB DEFAULT NULL;

COMMENT ON COLUMN api_keys.ip_whitelist IS 'JSON array of allowed IPs/CIDRs, e.g. ["192.168.1.100", "10.0.0.0/8"]';
COMMENT ON COLUMN api_keys.ip_blacklist IS 'JSON array of blocked IPs/CIDRs, e.g. ["1.2.3.4", "5.6.0.0/16"]';
