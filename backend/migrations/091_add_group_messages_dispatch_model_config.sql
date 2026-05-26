ALTER TABLE groups
ADD COLUMN IF NOT EXISTS messages_dispatch_model_config JSONB NOT NULL DEFAULT '{}'::jsonb;
