-- Persist external ownership so WARP reconciliation never infers control from
-- a user-visible name or a shared host:port endpoint.
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS managed_by VARCHAR(64);
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS external_id VARCHAR(255);

-- Before ownership columns existed, WarpSyncService created a dedicated group
-- with this description. Only rows already in that group are pending adoption;
-- a warp-* name alone is deliberately not ownership proof.
UPDATE proxies AS p
SET managed_by = 'warp-gateway', updated_at = NOW()
FROM proxy_groups AS pg
WHERE p.group_id = pg.id
  AND p.deleted_at IS NULL
  AND pg.deleted_at IS NULL
  AND p.managed_by IS NULL
  AND p.name LIKE 'warp-%'
  AND pg.description = 'Cloudflare WARP proxy pool (auto-managed by warp-gateway sync)';

CREATE INDEX IF NOT EXISTS idx_proxies_managed_by_external_id
    ON proxies (managed_by, external_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_proxies_live_external_owner
    ON proxies (managed_by, external_id)
    WHERE deleted_at IS NULL
      AND managed_by IS NOT NULL
      AND external_id IS NOT NULL;
