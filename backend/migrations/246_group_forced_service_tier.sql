-- Replace the boolean "force Fast" flag with an explicit service-tier override.
-- '' follows the client; 'default', 'priority' and 'ultrafast' force that tier.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS forced_service_tier TEXT NOT NULL DEFAULT '';

UPDATE groups
SET forced_service_tier = 'priority'
WHERE force_openai_fast
  AND forced_service_tier = '';

COMMENT ON COLUMN groups.forced_service_tier IS
    'Forced OpenAI service_tier for this group; empty follows the request';

-- Independent operator subsidy for UltraFast billing.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS free_openai_ultrafast BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.free_openai_ultrafast IS
    'Bill this group''s UltraFast requests at Standard prices';
