-- Add auto_mode_classifier_model column to groups table
-- Stores the model to use for Claude Code auto mode classifier requests (sync, no thinking).
-- Empty string means no rewrite (use original model).
ALTER TABLE groups ADD COLUMN IF NOT EXISTS auto_mode_classifier_model VARCHAR(100) NOT NULL DEFAULT '';
