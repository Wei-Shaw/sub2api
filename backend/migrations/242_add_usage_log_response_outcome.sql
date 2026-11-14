-- Downstream delivery outcome for GPT HTTP requests, separate from the transport
-- request_type and from consumption fields. NULL = legacy or noncovered path and
-- never implies proven delivery. Up-only and additive: rolling back to v0.2.13
-- means deploying the previous binary, which never names this column, so its
-- inserts leave it NULL and its queries ignore it. The nullable column and the
-- NOT VALID check stay in place; no destructive down migration is provided.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS response_outcome VARCHAR(32);

ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_response_outcome_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_response_outcome_check
    CHECK (response_outcome IS NULL OR response_outcome IN ('response_written', 'client_cancelled', 'write_failed', 'upstream_failed'))
    NOT VALID;

COMMENT ON COLUMN usage_logs.response_outcome IS
    'Downstream delivery outcome: response_written (server write completed), client_cancelled, write_failed, upstream_failed; NULL = legacy/noncovered';
