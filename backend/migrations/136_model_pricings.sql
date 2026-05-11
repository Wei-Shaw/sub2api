-- 136_model_pricings.sql
-- Create model_pricings table for admin-managed model pricing configuration.

CREATE TABLE IF NOT EXISTS model_pricings (
    id                                  BIGSERIAL PRIMARY KEY,
    model                               VARCHAR(200)  NOT NULL,
    input_cost_per_token                DOUBLE PRECISION NOT NULL DEFAULT 0,
    output_cost_per_token               DOUBLE PRECISION NOT NULL DEFAULT 0,
    cache_creation_input_token_cost     DOUBLE PRECISION,
    cache_creation_input_token_cost_above_1hr DOUBLE PRECISION,
    cache_read_input_token_cost         DOUBLE PRECISION,
    input_cost_per_token_priority       DOUBLE PRECISION,
    output_cost_per_token_priority      DOUBLE PRECISION,
    cache_read_input_token_cost_priority DOUBLE PRECISION,
    output_cost_per_image               DOUBLE PRECISION,
    output_cost_per_image_token         DOUBLE PRECISION,
    long_context_input_token_threshold  INTEGER,
    long_context_input_cost_multiplier  DOUBLE PRECISION,
    long_context_output_cost_multiplier DOUBLE PRECISION,
    supports_service_tier               BOOLEAN       NOT NULL DEFAULT false,
    litellm_provider                    VARCHAR(100)  NOT NULL DEFAULT '',
    mode                                VARCHAR(50)   NOT NULL DEFAULT 'chat',
    supports_prompt_caching             BOOLEAN       NOT NULL DEFAULT false,
    locked                              BOOLEAN       NOT NULL DEFAULT false,
    source                              VARCHAR(20)   NOT NULL DEFAULT 'remote',
    created_at                          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at                          TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS model_pricings_model_key ON model_pricings (model);
