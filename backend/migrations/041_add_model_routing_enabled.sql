-- Add model_routing_enabled field to groups table
ALTER TABLE groups ADD COLUMN IF NOT EXISTS model_routing_enabled BOOLEAN NOT NULL DEFAULT false;
