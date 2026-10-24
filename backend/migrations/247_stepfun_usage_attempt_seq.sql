-- Step Plan probes reserve a database sequence before contacting upstream.
-- Persisting a result is conditional on this token being newer than the row's
-- last token, which avoids cross-instance app-clock skew and out-of-order writes.
CREATE SEQUENCE IF NOT EXISTS stepfun_usage_attempt_seq AS BIGINT;
