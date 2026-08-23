-- Independent OpenAI-compatible prompt input audit.
-- Raw prompts and Guard credentials are intentionally absent from PostgreSQL.

CREATE TABLE IF NOT EXISTS prompt_audit_jobs (
    id                    BIGSERIAL PRIMARY KEY,
    request_id            VARCHAR(128) NOT NULL DEFAULT '',
    user_id               BIGINT REFERENCES users(id) ON DELETE SET NULL,
    username_snapshot     VARCHAR(255) NOT NULL DEFAULT '',
    user_email_snapshot   VARCHAR(320) NOT NULL DEFAULT '',
    api_key_id            BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    api_key_name_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    group_id              BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    group_name            VARCHAR(255) NOT NULL DEFAULT '',
    provider              VARCHAR(64) NOT NULL DEFAULT '',
    endpoint              VARCHAR(128) NOT NULL DEFAULT '',
    protocol              VARCHAR(64) NOT NULL DEFAULT '',
    model                 VARCHAR(255) NOT NULL DEFAULT '',
    prompt_hash           VARCHAR(64) NOT NULL DEFAULT '',
    redacted_preview      TEXT NOT NULL DEFAULT '',
    prompt_length         INT NOT NULL DEFAULT 0,
    message_count         INT NOT NULL DEFAULT 0,
    stage                 VARCHAR(32) NOT NULL DEFAULT 'http',
    execution_mode        VARCHAR(32) NOT NULL DEFAULT 'async_audit',
    config_version        BIGINT NOT NULL DEFAULT 1,
    status                VARCHAR(32) NOT NULL DEFAULT 'staging',
    attempts              INT NOT NULL DEFAULT 0,
    max_attempts          INT NOT NULL DEFAULT 3,
    claim_version         BIGINT NOT NULL DEFAULT 0,
    next_attempt_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_started_at TIMESTAMPTZ,
    processed_at          TIMESTAMPTZ,
    last_error_code       VARCHAR(64) NOT NULL DEFAULT '',
    last_error_message    VARCHAR(512) NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_prompt_audit_jobs_status
        CHECK (status IN ('staging', 'queued', 'processing', 'retry', 'done', 'failed')),
    CONSTRAINT chk_prompt_audit_jobs_execution_mode
        CHECK (execution_mode IN ('async_audit', 'blocking')),
    CONSTRAINT chk_prompt_audit_jobs_nonnegative
        CHECK (
            attempts >= 0 AND max_attempts >= 0 AND claim_version >= 0 AND
            prompt_length >= 0 AND message_count >= 0 AND config_version >= 1
        )
);

CREATE TABLE IF NOT EXISTS prompt_audit_events (
    id                       BIGSERIAL PRIMARY KEY,
    job_id                   BIGINT NOT NULL REFERENCES prompt_audit_jobs(id) ON DELETE CASCADE,
    request_id               VARCHAR(128) NOT NULL DEFAULT '',
    user_id                  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    username_snapshot        VARCHAR(255) NOT NULL DEFAULT '',
    user_email_snapshot      VARCHAR(320) NOT NULL DEFAULT '',
    api_key_id               BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    api_key_name_snapshot    VARCHAR(255) NOT NULL DEFAULT '',
    group_id                 BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    group_name               VARCHAR(255) NOT NULL DEFAULT '',
    provider                 VARCHAR(64) NOT NULL DEFAULT '',
    endpoint                 VARCHAR(128) NOT NULL DEFAULT '',
    protocol                 VARCHAR(64) NOT NULL DEFAULT '',
    model                    VARCHAR(255) NOT NULL DEFAULT '',
    prompt_hash              VARCHAR(64) NOT NULL DEFAULT '',
    redacted_preview         TEXT NOT NULL DEFAULT '',
    stage                    VARCHAR(32) NOT NULL DEFAULT 'http',
    decision                 VARCHAR(32) NOT NULL DEFAULT 'pass',
    risk_level               VARCHAR(32) NOT NULL DEFAULT 'low',
    action                   VARCHAR(32) NOT NULL DEFAULT 'Allow',
    categories               JSONB NOT NULL DEFAULT '[]'::jsonb,
    matched_scanners         JSONB NOT NULL DEFAULT '[]'::jsonb,
    scanner_scores           JSONB NOT NULL DEFAULT '{}'::jsonb,
    scanner_evidence         JSONB NOT NULL DEFAULT '{}'::jsonb,
    scanner_backend          VARCHAR(64) NOT NULL DEFAULT 'qwen3guard-openai',
    scanner_version          VARCHAR(128) NOT NULL DEFAULT '',
    guard_endpoint_id        VARCHAR(128) NOT NULL DEFAULT '',
    policy_id                VARCHAR(128) NOT NULL DEFAULT '',
    policy_version           INT NOT NULL DEFAULT 0,
    config_version           BIGINT NOT NULL DEFAULT 1,
    chunk_total              INT NOT NULL DEFAULT 0,
    latency_ms               INT NOT NULL DEFAULT 0,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_prompt_audit_events_decision
        CHECK (decision IN ('pass', 'flag', 'critical')),
    CONSTRAINT chk_prompt_audit_events_risk_level
        CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    CONSTRAINT chk_prompt_audit_events_action
        CHECK (action IN ('Allow', 'Warn', 'Block')),
    CONSTRAINT chk_prompt_audit_events_nonnegative
        CHECK (policy_version >= 0 AND config_version >= 1 AND chunk_total >= 0 AND latency_ms >= 0),
    CONSTRAINT chk_prompt_audit_events_categories_json
        CHECK (jsonb_typeof(categories) = 'array'),
    CONSTRAINT chk_prompt_audit_events_scanners_json
        CHECK (jsonb_typeof(matched_scanners) = 'array'),
    CONSTRAINT chk_prompt_audit_events_scores_json
        CHECK (jsonb_typeof(scanner_scores) = 'object'),
    CONSTRAINT chk_prompt_audit_events_evidence_json
        CHECK (jsonb_typeof(scanner_evidence) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_prompt_audit_jobs_schedule
    ON prompt_audit_jobs(status, next_attempt_at, id);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_jobs_request
    ON prompt_audit_jobs(request_id);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_jobs_user_created
    ON prompt_audit_jobs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_jobs_api_key_created
    ON prompt_audit_jobs(api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_jobs_group_created
    ON prompt_audit_jobs(group_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_jobs_prompt_hash
    ON prompt_audit_jobs(prompt_hash);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_jobs_created
    ON prompt_audit_jobs(created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_job
    ON prompt_audit_events(job_id);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_request
    ON prompt_audit_events(request_id);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_decision_created
    ON prompt_audit_events(decision, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_risk_created
    ON prompt_audit_events(risk_level, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_user_created
    ON prompt_audit_events(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_api_key_created
    ON prompt_audit_events(api_key_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_group_created
    ON prompt_audit_events(group_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_prompt_hash
    ON prompt_audit_events(prompt_hash);
CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_created
    ON prompt_audit_events(created_at DESC, id DESC);
