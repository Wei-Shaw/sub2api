-- Optional scale indexes for private platform groups (PR6).
-- Non-transactional: CREATE INDEX CONCURRENTLY cannot run inside a transaction.
--
-- 1) private-* partial index: keeps revoke / private-only prefix scans small as N×5 grows.
--    Complements groups_name_unique_active (all active names) with a private-only subset.
-- 2) non-private partial index: helps ListActiveExcludingPrivate and admin ListWithFilters
--    which default-exclude name LIKE 'private-%'.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_groups_private_name_active
    ON groups (name)
    WHERE deleted_at IS NULL
      AND name LIKE 'private-%';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_groups_active_non_private_sort
    ON groups (status, sort_order, id)
    WHERE deleted_at IS NULL
      AND name NOT LIKE 'private-%';
