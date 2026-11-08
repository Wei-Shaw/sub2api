-- Migration: 176_channel_monitor_grok_provider
-- Allow Grok as a channel-monitor provider. Grok checks use the existing
-- OpenAI-compatible chat completions protocol with model grok-4.5 by default.

DO $$
DECLARE
    monitor_constraint_def TEXT;
    template_constraint_def TEXT;
BEGIN
    SELECT pg_get_constraintdef(c.oid)
      INTO monitor_constraint_def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
     WHERE t.relname = 'channel_monitors'
       AND c.conname = 'channel_monitors_provider_check';

    IF monitor_constraint_def IS NULL OR position('grok' IN monitor_constraint_def) = 0 THEN
        ALTER TABLE channel_monitors
            DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok'));
    END IF;

    SELECT pg_get_constraintdef(c.oid)
      INTO template_constraint_def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
     WHERE t.relname = 'channel_monitor_request_templates'
       AND c.conname = 'channel_monitor_request_templates_provider_check';

    IF template_constraint_def IS NULL OR position('grok' IN template_constraint_def) = 0 THEN
        ALTER TABLE channel_monitor_request_templates
            DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
        ALTER TABLE channel_monitor_request_templates
            ADD CONSTRAINT channel_monitor_request_templates_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok'));
    END IF;
END $$;
