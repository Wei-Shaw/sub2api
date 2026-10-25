-- MySQL 方言版本（翻译自 ../032_add_api_key_ip_restriction.sql）
-- Add IP restriction fields to api_keys table
-- ip_whitelist: JSON array of allowed IPs/CIDRs (if set, only these IPs can use the key)
-- ip_blacklist: JSON array of blocked IPs/CIDRs (these IPs are always blocked)

ALTER TABLE api_keys ADD COLUMN ip_whitelist JSON DEFAULT NULL;
ALTER TABLE api_keys ADD COLUMN ip_blacklist JSON DEFAULT NULL;

-- (comment skipped)
-- (comment skipped)
