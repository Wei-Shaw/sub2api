-- Overrides follow the same null/default and explicit-zero semantics as image/video prices.
ALTER TABLE groups ADD COLUMN minimax_rate_independent BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE groups ADD COLUMN minimax_rate_multiplier NUMERIC(10,4) NOT NULL DEFAULT 1 CHECK (minimax_rate_multiplier >= 0);
ALTER TABLE groups ADD COLUMN minimax_media_prices JSONB;
