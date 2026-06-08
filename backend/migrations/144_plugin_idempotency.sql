-- Migration 144: create plugin_idempotency table.
--
-- Background:
--   Phase 0 of the payment-system plugin migration introduces reverse RPCs
--   (HostService.CreditBalance / DeductBalance / AssignSubscription /
--   RevokeSubscriptionDays / AccrueRebate) that are externally retried.
--   Each RPC carries an idempotency_key (typically the source payment
--   order_id or redeem code) so the host can ignore replays without
--   double-crediting users.
--
--   Redis-only TTL is insufficient because a partition between the host
--   and Redis could lose the dedup record while the SQL state already
--   reflects the credit; the database row is the source of truth.
--
-- Schema:
--   namespace      — RPC namespace ("credit_balance", "deduct_balance",
--                    "assign_sub", "revoke_sub", "rebate"). Keeps keys
--                    from different RPC types from colliding.
--   key            — caller-supplied idempotency_key.
--   result_payload — JSON-encoded response payload so duplicate calls
--                    return the original outcome (new_balance,
--                    subscription_id, etc.).
--   created_at     — set on first insert; index supports periodic cleanup
--                    once entries are old enough to be safely forgotten
--                    (e.g. >90 days).
--
-- Idempotency:
--   - CREATE TABLE / CREATE INDEX guarded by IF NOT EXISTS so re-running
--     is a no-op.

CREATE TABLE IF NOT EXISTS plugin_idempotency (
    namespace      TEXT NOT NULL,
    key            TEXT NOT NULL,
    result_payload JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (namespace, key)
);

CREATE INDEX IF NOT EXISTS idx_plugin_idempotency_created_at
    ON plugin_idempotency (created_at);
