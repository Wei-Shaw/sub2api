-- Optional, exact request-model image price expressions. Existing groups retain
-- their current pricing until an administrator explicitly configures a model.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS image_price_expressions JSONB;

COMMENT ON COLUMN groups.image_price_expressions IS
    'Optional image total-cost expressions keyed by exact, case-sensitive client request model';
