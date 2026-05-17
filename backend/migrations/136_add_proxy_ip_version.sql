-- Add proxy exit IP version preference for IPv4/IPv6-aware connectivity probes.

ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS ip_version VARCHAR(10) NOT NULL DEFAULT 'ipv4';

UPDATE proxies
SET ip_version = 'ipv4'
WHERE ip_version IS NULL OR ip_version = '';

ALTER TABLE proxies
    DROP CONSTRAINT IF EXISTS proxies_ip_version_check;

ALTER TABLE proxies
    ADD CONSTRAINT proxies_ip_version_check
    CHECK (ip_version IN ('ipv4', 'ipv6'));
