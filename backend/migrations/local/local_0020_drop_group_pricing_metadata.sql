-- Migration: Drop deprecated group pricing metadata columns
--
-- token_price_per_million, pricing_reference_model, input_output_ratio
-- were display-only metadata that never participated in actual billing.
-- Actual billing uses rate_multiplier × TotalCost = ActualCost.
-- Removing to prevent confusion.

ALTER TABLE groups
    DROP COLUMN IF EXISTS token_price_per_million,
    DROP COLUMN IF EXISTS pricing_reference_model,
    DROP COLUMN IF EXISTS input_output_ratio;
