-- Display-pricing `platform` follows the request protocol, while `provider`
-- remains the vendor/brand used for customer-facing grouping. The x5m5x
-- catalogue is served entirely through an OpenAI-compatible protocol, so the
-- original Anthropic/Gemini/Grok seed rows must join the active OpenAI channel.
-- This migration only touches presentation data and never channel billing.

UPDATE display_model_prices AS target
SET platform = 'openai', updated_at = NOW()
WHERE target.provider IN ('anthropic', 'gemini', 'grok')
  AND target.platform IN ('anthropic', 'gemini', 'grok')
  AND NOT EXISTS (
      SELECT 1
      FROM display_model_prices AS existing
      WHERE existing.platform = 'openai'
        AND existing.model_name = target.model_name
        AND existing.billing_mode = target.billing_mode
  );
