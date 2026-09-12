-- Preserve the reference amount at recording time, independently of customer
-- pricing and multipliers. NULL means unknown; existing rows are not repriced.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS api_reference_cost NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS api_reference_pricing JSONB;

COMMENT ON COLUMN usage_logs.api_reference_cost IS
    'Model-catalog API reference USD at request recording; NULL when unavailable';
COMMENT ON COLUMN usage_logs.api_reference_pricing IS
    'Versioned reference pricing inputs, price card, and component costs';
