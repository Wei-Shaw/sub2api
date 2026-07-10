ALTER TABLE groups
ADD COLUMN IF NOT EXISTS claude_code_default_models JSONB NOT NULL DEFAULT '{}'::jsonb;
