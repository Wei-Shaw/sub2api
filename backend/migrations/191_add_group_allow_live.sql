-- Keep the rollout additive and rollback-compatible. Existing NULL values are
-- read as false by Ent; new groups receive the application default of false.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS allow_live BOOLEAN;
