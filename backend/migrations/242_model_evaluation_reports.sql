-- Keep evaluation snapshots independently of account/group deletion.
CREATE TABLE IF NOT EXISTS model_evaluation_reports (
    id UUID PRIMARY KEY,
    target_type VARCHAR(16) NOT NULL CHECK (target_type IN ('account', 'group')),
    target_id BIGINT NOT NULL CHECK (target_id > 0),
    target_name TEXT NOT NULL,
    model VARCHAR(200) NOT NULL,
    effort VARCHAR(16) NOT NULL,
    rounds INTEGER NOT NULL CHECK (rounds BETWEEN 1 AND 20),
    benchmark VARCHAR(64) NOT NULL,
    prompt TEXT NOT NULL,
    created_by BIGINT,
    visibility VARCHAR(16) NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'public')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_model_evaluation_reports_target
    ON model_evaluation_reports (target_type, target_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS model_evaluation_rounds (
    report_id UUID NOT NULL REFERENCES model_evaluation_reports(id),
    round INTEGER NOT NULL CHECK (round BETWEEN 1 AND 20),
    status VARCHAR(16) NOT NULL CHECK (status IN ('running', 'correct', 'incorrect', 'ungraded', 'error')),
    result JSONB NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    PRIMARY KEY (report_id, round)
);
