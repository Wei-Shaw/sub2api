-- Presentation-only provider notes scoped by billing mode.
--
-- provider_note remains the token/usage-based note for backward compatibility.
-- The new fields let the customer-facing catalogue show independent copy for
-- per-request and image sections without touching channels or real billing.

ALTER TABLE display_pricing_providers
    ADD COLUMN IF NOT EXISTS per_request_note VARCHAR(4000) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS image_note VARCHAR(4000) NOT NULL DEFAULT '';
