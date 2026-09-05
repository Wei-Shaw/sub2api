ALTER TABLE groups ADD COLUMN IF NOT EXISTS allow_audio_transcription BOOLEAN NOT NULL DEFAULT false;
