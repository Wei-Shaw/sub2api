-- Add image_output_price column to channel_pricing_intervals so each tiered
-- pricing band can carry its own image-output rate, mirroring the parent
-- channel_model_pricing.image_output_price field. This unblocks the gRPC
-- pricing publish path: encodeIntervals previously hard-coded "" for the
-- proto image_output_price field because the service struct lacked the
-- column; T29 plumbs the value end-to-end (proto → service → DB).
--
-- NUMERIC(20,12) matches the precision used by the rest of the plugin
-- pricing tables (channel_pricing_intervals.input_price et al.).

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE channel_pricing_intervals
    ADD COLUMN IF NOT EXISTS image_output_price NUMERIC(20, 12);

COMMENT ON COLUMN channel_pricing_intervals.image_output_price IS
    'image 模式：图片输出价（与 channel_model_pricing.image_output_price 对称）';
