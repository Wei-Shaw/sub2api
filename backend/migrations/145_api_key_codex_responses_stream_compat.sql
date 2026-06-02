-- Add API key scoped Codex CLI Responses stream compatibility switch.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS codex_responses_stream_compat BOOLEAN NOT NULL DEFAULT FALSE;
