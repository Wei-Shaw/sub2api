-- Per-group profit control for scheduling admission. Keep these expansion
-- columns nullable and without database defaults so the previous application
-- can continue creating groups during blue-green deployment and image rollback.
-- The new application maps NULL to false/zero and writes all three values for
-- newly created or updated groups.
--
-- Admission rule at request time: an account qualifies iff its cost multiplier
-- U (accounts.rate_multiplier) satisfies U <= D * (1 - margin - buffer), where
-- D is the requester's effective downstream multiplier at the request's
-- pricing instant.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS profit_control_enabled BOOLEAN;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS profit_min_margin DECIMAL(10,4);

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS profit_safety_buffer DECIMAL(10,4);
