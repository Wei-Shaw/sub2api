-- Migration: 246_remove_unused_accounts_extra_gin_index
-- The generic accounts.extra GIN index was introduced for a containment lookup,
-- while the repository's general-purpose FindByExtraField helper uses JSONB
-- path equality. A broad GIN index on this frequently updated column imposes
-- substantial write and vacuum maintenance cost, while future containment workloads
-- can add narrower, query-specific indexes when their scale justifies them.
DROP INDEX CONCURRENTLY IF EXISTS public.idx_accounts_extra_gin;
