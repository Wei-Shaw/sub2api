-- MySQL 方言版本（翻译自 ../085_channel_restrict_and_per_request_price.sql）
-- Add model restriction switch to channels
ALTER TABLE channels ADD COLUMN restrict_models BOOLEAN DEFAULT FALSE;

-- Add default per_request_price to channel_model_pricing (fallback when no tier matches)
ALTER TABLE channel_model_pricing ADD COLUMN per_request_price DECIMAL(20,10);
