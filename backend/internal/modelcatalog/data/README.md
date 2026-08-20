# Model catalog

This JSON file is the TokenSupply fork's owned model directory and fallback
price book.

- Amounts are USD per million tokens.
- `lock_price: true` means the listed fields overlay LiteLLM / the bundled
  mirror. Missing fields are left alone. Do not lock a card unless we intend
  to pin those fields over the remote table.
- Unlocked cards only fill models that are missing from both LiteLLM and the
  hardcoded fallback.
- `aliases` are directory metadata. Only Antigravity thinking-tier IDs
  (`-high` / `-low` / `-medium` / `-tiered`) share the base published rate
  card. Do not alias a different sold model onto another model's price.
- Do not put secrets or production-only config in this file.

See `docs/MODEL_CATALOG_AND_CHANNEL_STOREFRONT.md`.
