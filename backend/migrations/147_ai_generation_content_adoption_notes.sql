ALTER TABLE ai_generation_content
    ADD COLUMN IF NOT EXISTS adoption_notes TEXT NOT NULL DEFAULT '';
