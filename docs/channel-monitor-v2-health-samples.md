# Channel Monitor V2: minimum samples by view

The health sample minimum applies separately to each group/model and time
bucket. A single common minimum can leave short-window buckets unscored even
when a longer window has enough traffic.

The monitor settings page supports optional overrides for each view:

| View | Bucket duration |
| --- | --- |
| `90m` | 5 minutes |
| `24h` | 1 hour |
| `7d` | 12 hours |
| `30d` | 1 day |

For example, the following health settings use progressively larger samples:

```json
{
  "minimum_sample": 30,
  "minimum_sample_by_range": {
    "90m": 10,
    "24h": 30,
    "7d": 50,
    "30d": 100
  }
}
```

These are example values, not new installation defaults. Existing configs
without overrides continue using their common `minimum_sample` for all views.
Leave an override blank in the settings page to restore that view's common
minimum. Values are bounded to 1–10000; unsupported range keys are ignored.

The resolved minimum is used consistently for snapshot, trend, matrix and
model health results. Buckets with too few requests remain unscored/grey;
first-token scoring also requires enough first-token observations. Switching
views does not change the saved common minimum or any other view's override.

Overrides are stored within the existing `health_thresholds` JSON config;
no schema migration or aggregation backfill is required. Error/latency/cache
budgets, weights, raw metrics and traffic collection are unchanged.
