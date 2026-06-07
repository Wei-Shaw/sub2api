## ADDED Requirements

### Requirement: Public Recharge Promo Endpoint

The system SHALL expose `GET /api/v1/plaza/recharge-promo` as an unauthenticated public endpoint that returns the currently-active recharge bonus campaign in a display-oriented shape suitable for anonymous marketing surfaces (e.g. the homepage). The endpoint SHALL NOT require authentication and SHALL ignore any credentials supplied. It SHALL be registered alongside `/api/v1/plaza/models` and `/api/v1/plaza/plans` and SHALL share the same Redis-backed rate limiter (60 requests / minute / IP, fail-open when Redis is unavailable).

The response body SHALL be JSON of shape:

```
{
  "promo": {
    "name":        string,                 // 1..120 chars, never empty
    "valid_from":  RFC3339 string | null,  // null when unbounded
    "valid_until": RFC3339 string | null,  // null when unbounded
    "tiers":       [{"min_amount": number, "bonus_rate": number}, ...],
    "version":     string                  // stable hash matching checkout-info
  } | null
}
```

When **any** of the following hold, the endpoint SHALL return `{ "promo": null }`:

- No row exists with `enabled = true` in the `recharge_promo_activities` table;
- The active row's effective configuration fails `IsActiveAt(time.Now())` (i.e. now is before `valid_from` or at-or-after `valid_until`);
- The active row's `tiers` array is empty (defensive: should not happen post-validation, but the endpoint SHALL not crash).

The response SHALL NOT include `enabled` (presence in the response IS the enabled signal) or `activity_id` (an internal audit field). The `version` token SHALL match the value returned by `GET /api/payment/checkout-info` for the same campaign so frontends can correlate cached state across endpoints.

#### Scenario: Anonymous client requests an active campaign

- **GIVEN** an enabled row in `recharge_promo_activities` with `name = "618 充值返现"`, tiers `[(100, 0.03), (500, 0.05)]`, `valid_until` in the future
- **WHEN** an anonymous client (no Authorization header) issues `GET /api/v1/plaza/recharge-promo`
- **THEN** the response is `200 OK` with `promo.name = "618 充值返现"`, `promo.tiers` containing the same two entries, a non-empty `promo.version`, and no `enabled` or `activity_id` keys

#### Scenario: No active campaign

- **GIVEN** zero rows in `recharge_promo_activities` with `enabled = true`
- **WHEN** the client issues `GET /api/v1/plaza/recharge-promo`
- **THEN** the response is `200 OK` with body `{ "promo": null }`

#### Scenario: Active row outside its time window

- **GIVEN** an enabled row with `valid_until = 2026-06-01T00:00:00Z` and the server clock at `2026-06-01T00:00:01Z`
- **WHEN** the client issues `GET /api/v1/plaza/recharge-promo`
- **THEN** the response is `200 OK` with body `{ "promo": null }`

#### Scenario: Invalid auth header is ignored

- **WHEN** a client issues `GET /api/v1/plaza/recharge-promo` with `Authorization: Bearer not-a-real-token`
- **THEN** the response is `200 OK` and the body is identical to what an anonymous client would receive

#### Scenario: Version token matches checkout-info

- **GIVEN** an authenticated user `U` and an active campaign
- **WHEN** `U` calls `GET /api/payment/checkout-info` and an anonymous client calls `GET /api/v1/plaza/recharge-promo` within the same admin-config epoch
- **THEN** `checkout-info.recharge_promo.version` equals `plaza/recharge-promo.promo.version`

#### Scenario: Rate limit fail-open

- **GIVEN** Redis is unreachable
- **WHEN** the client issues `GET /api/v1/plaza/recharge-promo`
- **THEN** the response is still `200 OK` with the live campaign data; the request is not blocked

## MODIFIED Requirements

### Requirement: Checkout API Surfaces Active Campaign

The `GET /api/payment/checkout-info` response SHALL include a `recharge_promo` object whenever the configured campaign would resolve to a non-zero bonus for at least one tier at the current server time. The shape SHALL be:

```
recharge_promo: {
  enabled:     boolean,
  name:        string,                   // active campaign display name (1..120 chars)
  valid_from:  RFC3339 string | null,
  valid_until: RFC3339 string | null,
  tiers:       [{min_amount: number, bonus_rate: number}, ...],
  version:     string                    // stable hash of the campaign content
}
```

The `name` field SHALL match the `name` column of the active row in `recharge_promo_activities`. When the campaign is disabled or outside its time window, the response SHALL omit `recharge_promo` (or set it to `null`); the frontend SHALL treat both forms as "no active campaign". The `version` value SHALL be identical to that returned by `GET /api/v1/plaza/recharge-promo` for the same campaign epoch.

#### Scenario: Active campaign is exposed to the checkout

- **GIVEN** an enabled campaign with `name = "夏季满返"`, tiers `[(100, 0.03), (500, 0.05)]` and `valid_until` in the future
- **WHEN** the user fetches `/api/payment/checkout-info`
- **THEN** the response includes `recharge_promo` with `name = "夏季满返"`, the same tiers, and a non-empty `version`

#### Scenario: Disabled campaign is hidden from checkout

- **GIVEN** a campaign with `enabled = false`
- **WHEN** the user fetches `/api/payment/checkout-info`
- **THEN** the response either omits `recharge_promo` or sets it to `null`

#### Scenario: Version stability across no-op saves

- **GIVEN** an admin saves the campaign without changing any field
- **WHEN** the new checkout response is compared with the previous one
- **THEN** `recharge_promo.version` is unchanged

#### Scenario: Name change updates version

- **GIVEN** an admin renames the active campaign from `"夏季满返"` to `"夏季福利"` while keeping all other fields intact
- **WHEN** subsequent checkouts are fetched
- **THEN** `recharge_promo.name` reflects the new value and `recharge_promo.version` differs from the pre-rename value
