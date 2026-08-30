# x5m5x real-channel provisioning

`provision-x5m5x.ps1` idempotently creates or updates three isolated real-billing groups:

- `按量分组【成功率百分之99+】`: 33 token-billed models.
- `按次分组【成功率百分之95+】`: 14 models with the upstream's exact three context-tier prices.
- `生图分组-按次扣费`: one image model with image generation enabled.

Each group receives one OpenAI-compatible API-key account and one channel. Group and account multipliers are both `1.0`; the configured margin lives only in the channel price. All three groups remain exclusive by default.

This tool manages real channel billing only. It does not read or change the independent display-pricing catalogue used by the user-facing model-price page.

## Authoritative prices and models

Every run downloads and parses the current `https://api.x5m5x.com/pricing/` HTML. No upstream prices or model lists are embedded in the script.

Before any real write, the tool also calls authenticated `GET /v1/models` with each of the three keys. It requires exact `33 / 14 / 1` counts and case-insensitive set agreement with the three pricing-page catalogues. Missing, extra, case-insensitive duplicate, or unpriced models abort the run. When only casing differs, `/v1/models` is authoritative: the group whitelist, identity mapping, and channel-pricing entry all use its exact model spelling.

The pricing page prefixes its values with `¥`, but live x5m5x usage records debit the same numeric values with `unit=USD`. Therefore the tool intentionally does **not** apply a CNY/USD exchange-rate conversion:

- Token HTML values are treated as upstream numeric USD per million tokens, multiplied by `-Markup`, then divided by `1,000,000` for Sub2API's USD-per-token fields.
- Per-request HTML values are treated as upstream numeric USD per request and multiplied by `-Markup`.
- Image HTML values are treated as upstream numeric USD per image and multiplied by `-Markup`.

The default markup is `1.20`. For example, upstream `0.005 / 0.0085 / 0.010` becomes channel pricing `0.006 / 0.0102 / 0.012` USD per request. The three upstream per-request tiers are copied exactly before markup; the tool never synthesizes them as `1x / 1.5x / 2x`.

The upstream doubles DeepSeek token charges on workdays during Beijing time `09:00–12:00` and `14:00–18:00`. Each DeepSeek token entry therefore receives channel `time_pricing` for `Asia/Shanghai`, weekdays only, with a `2.0` multiplier during both periods. The markup is already present in the normal channel price, so it is preserved during peak billing. The run aborts if that upstream notice disappears or changes, rather than silently installing a stale schedule.

## Safety defaults

- New groups are staged as exclusive and remain exclusive unless `-PublishGroups` is explicitly passed.
- Existing managed groups are returned to exclusive mode on a normal run.
- All account and channel mappings are exact identity whitelists built from authenticated upstream model responses.
- Keys and the administrator JWT are accepted as `SecureString`, never embedded in the repository, and never printed.
- Upstream catalogue reads retry transient failures five times without logging credentials or response bodies.
- Writes use an `Idempotency-Key`; reruns find resources by exact managed name and update them.
- Non-loopback administrator API endpoints must use HTTPS.
- `-DryRun` sends no writes and does not request upstream keys.

The account credential table stores upstream credentials in PostgreSQL JSONB. Protect database backups and rotate keys that have ever been shared outside the intended secret manager.

## Run

PowerShell prompts securely for the administrator JWT and, during a real run, the three upstream keys:

```powershell
.\tools\provision-x5m5x.ps1 -ApiBase https://sub2api.example.com -DryRun
.\tools\provision-x5m5x.ps1 -ApiBase https://sub2api.example.com
```

For non-interactive execution, inject these process environment variables immediately before running:

```text
SUB2API_ADMIN_API_BASE
SUB2API_ADMIN_JWT
X5M5X_TOKEN_KEY
X5M5X_PER_REQUEST_KEY
X5M5X_IMAGE_KEY
```

Do not save those values in a tracked script. A secret manager that injects them only for the process lifetime is preferred.

To make all three groups public after a successful private run and review:

```powershell
.\tools\provision-x5m5x.ps1 -ApiBase https://sub2api.example.com -PublishGroups
```

Optional controls:

- `-Markup 1.20`: channel selling-price multiplier applied to every live upstream numeric price.
- `-PricingUrl`: live upstream HTML price source; HTTPS is required.
- `-UpstreamApiBase`: OpenAI-compatible upstream base; HTTPS is required.
- `-AccountConcurrency 3`: per-account concurrency. Zero is rejected because Sub2API interprets it as unlimited.
- `-AccountPriority 50`: scheduler priority.
- `-NamePrefix x5m5x`: managed channel/account prefix. The three required Chinese group names are fixed and unaffected.
- `-PublishGroups`: set all three groups to non-exclusive only after provisioning succeeds.

Users normally need a separate Sub2API API key for each group because several identical model IDs exist in both token and per-request modes.

## Offline dry-run fixture

For deterministic parser and dry-run testing without reaching x5m5x, pass `-FixtureDirectory`. It is accepted only together with `-DryRun`; a real provisioning run always verifies live state. The directory must contain:

```text
pricing.html
token-models.json
per-request-models.json
image-models.json
```

Each JSON file uses the normal OpenAI models shape:

```json
{"object":"list","data":[{"id":"model-name","object":"model"}]}
```

Example:

```powershell
.\tools\provision-x5m5x.ps1 `
  -ApiBase http://127.0.0.1:8080 `
  -DryRun `
  -FixtureDirectory .\testdata\x5m5x
```

## Post-provision checks

1. Confirm group and account multipliers are all `1.0`.
2. Confirm channels expose exactly `33`, `14`, and `1` model and pricing entries.
3. Verify a DeepSeek token entry has both weekday Beijing peak periods at `2.0`.
4. Verify a per-request model such as DeepSeek Flash retains the upstream's exact three tiers after the single markup multiplication.
5. Test one chat request from each text group and `POST /v1/images/generations` from the image group.
6. Compare recorded Sub2API cost with the x5m5x usage ledger before publishing groups.
