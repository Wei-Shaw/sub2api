# Model Pricing Data

This directory contains a local copy of the mirrored model pricing data as a fallback mechanism.

## Source
The original file is maintained by the LiteLLM project and mirrored into the `price-mirror` branch of this repository via GitHub Actions:
- Mirror branch (configurable via `PRICE_MIRROR_REPO`): https://raw.githubusercontent.com/<your-repo>/price-mirror/model_prices_and_context_window.json
- Upstream source: https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json

The bundled fallback also contains a small set of entries whose pricing authority
is the model vendor rather than LiteLLM. These entries carry `pricing_source`,
`source_kind`, `source_url`, and `source_version` fields so catalog syncs preserve
their provenance:

- `glm-5.2`: [Z.AI pricing](https://docs.z.ai/guides/overview/pricing)
- `qwen3.7-max`, `qwen3.7-plus`: [Alibaba Cloud Model Studio pricing](https://www.alibabacloud.com/help/en/model-studio/model-pricing)
- `minimax-m3`: [MiniMax pay-as-you-go pricing](https://platform.minimax.io/docs/guides/pricing-paygo)
- `kimi-k2.7-code`: [Moonshot/Kimi pricing](https://platform.kimi.ai/docs/pricing/chat-k27-code)

## Purpose
This local copy serves as a fallback when the remote file cannot be downloaded due to:
- Network restrictions
- Firewall rules
- DNS resolution issues
- GitHub being blocked in certain regions
- Docker container network limitations

## Update Process
The pricingService will:
1. First attempt to download the latest version from GitHub
2. If download fails, use this local copy as fallback
3. Log a warning when using the fallback file

## Manual Update
Do not overwrite this file directly with the LiteLLM payload because that would
remove the official-vendor entries. Refresh the LiteLLM portion, then re-apply and
verify the vendor entries and their official source metadata.

To fetch the latest LiteLLM data for review (if automation is unavailable):
```bash
curl -fsSL https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json -o /tmp/model_prices_and_context_window.json
```

## File Format
The file contains JSON data with model pricing information including:
- Model names and identifiers
- Input/output token costs
- Context window sizes
- Model capabilities

Last updated: 2026-07-20
