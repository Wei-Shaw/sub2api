-- Persist the model Skills selected as defaults for each group.
CREATE TABLE IF NOT EXISTS group_default_skills (
    group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    skill_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
