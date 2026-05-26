-- Add sort_order field to groups table for custom ordering
ALTER TABLE groups ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;

-- Initialize existing groups with sort_order based on their ID
UPDATE groups SET sort_order = id WHERE sort_order = 0;

-- Create index for efficient sorting
CREATE INDEX IF NOT EXISTS idx_groups_sort_order ON groups(sort_order);
