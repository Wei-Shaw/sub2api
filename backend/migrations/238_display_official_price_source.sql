-- Provenance metadata for the presentation-only official price snapshot.
--
-- These columns belong exclusively to display_model_prices. They are never
-- read by BillingService, PricingService, groups, channels, or gateway billing.

ALTER TABLE display_model_prices
    ADD COLUMN IF NOT EXISTS official_price_source VARCHAR(64) NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS official_price_source_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS official_price_synced_at TIMESTAMPTZ;
