-- 151_openai_audio_transcription_duration.sql
-- Store native OpenAI audio transcription duration billing facts.
--
-- Rollback guidance:
--   Stop accepting /v1/audio/transcriptions requests before rollback. The column
--   can be left in place safely; drop it only after confirming no duration-billed
--   usage rows are needed for audit or reconciliation.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS billable_duration_seconds INTEGER NOT NULL DEFAULT 0;
