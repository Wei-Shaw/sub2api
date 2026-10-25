-- MySQL 方言版本（翻译自 ../192_group_profit_control.sql）
-- Per-group profit control for scheduling admission.
-- Admission rule at request time: an account qualifies iff its cost multiplier
-- U (accounts.rate_multiplier) satisfies U <= D * (1 - margin - buffer), where
-- D is the requester's effective downstream multiplier at the request's
-- pricing instant.
ALTER TABLE `groups`
    ADD COLUMN profit_control_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN profit_min_margin DECIMAL(10,4) NOT NULL DEFAULT 0,
    ADD COLUMN profit_safety_buffer DECIMAL(10,4) NOT NULL DEFAULT 0;
