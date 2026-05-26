-- Add model_routing_enabled field to groups table
ALTER TABLE groups ADD COLUMN model_routing_enabled BOOLEAN NOT NULL DEFAULT false;
