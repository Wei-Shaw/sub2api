## Context

API keys currently carry one nullable `group_id`. Authentication loads the key, owner, and primary group into a versioned cache snapshot, and gateway handlers pass `apiKey.GroupID` into protocol-specific account selectors. Selection failures are handled before an upstream request is sent, while some protocols also retry accounts inside the same group after upstream failures.

Groups have separate fallback fields for specialized group policies. Those fields are administrator-controlled properties of a group and do not represent the per-key, ordered availability policy requested here.

## Goals / Non-Goals

**Goals:**

- Store a stable, ordered same-platform fallback list per personal API key.
- Apply the same candidate order across supported gateway protocols.
- Preserve the effective group through selection, policy, billing, logging, and retries.
- Keep existing API keys and group-level fallback mechanisms backward compatible.
- Make edits visible immediately through authentication-cache invalidation.

**Non-Goals:**

- Replaying a request across groups after it has been transmitted upstream.
- Allowing enterprise keys to escape their organization subscription group.
- Automatically ranking groups by price, load, or health.
- Replacing the existing group-level Claude client or invalid-request fallback features.

## Decisions

### Store ordered IDs on `api_keys`

Add a non-null JSON column `fallback_group_ids` with default `[]` and expose it as `[]int64` in the service model. A JSON array preserves order, makes the common read part of the existing authentication query, and avoids a join on the authentication hot path. A normalized relation was considered, but it adds ordering/index maintenance and cache-loading complexity for a list capped at five entries.

### Validate the complete list atomically

Create and update validate the entire proposed primary/fallback configuration before persisting. Validation enforces at most five positive distinct IDs, excludes the primary ID, loads every group, requires every fallback platform to exactly match the primary platform, and reuses the same user-bind authorization rules as the primary group. Enterprise-bound keys require an empty list. Invalid input leaves the prior configuration unchanged.

### Prepare billing-eligible candidates and resolve them once per request

Authentication loads `[primary, fallbacks...]` into a request-scoped routing state. For each candidate it resolves the group, rejects any runtime candidate whose platform differs from the primary group, and prepares its billing context: metered groups require usable personal balance, subscription groups require an active subscription with remaining limits, and enterprise keys retain their single organization subscription. Billing-ineligible candidates are retained as diagnostics but skipped during account selection. This explicitly supports transitions between metered and subscription groups within one platform without allowing cross-platform routing.

A shared resolver tries eligible group IDs in order through the existing protocol selector and returns both the selected account and effective group ID. Protocol integrations use this result at the earliest selection point and atomically update the request-scoped API key group, group context, and subscription context.

Only errors that mean no eligible account/model route was selected advance to the next group. Context cancellation, malformed input, policy denial, billing denial, or infrastructure errors do not silently fall through.

### Preserve one effective group after selection

After an account is selected, all subsequent retries remain inside that group. The effective group and its prepared subscription are exposed through the existing API key and subscription accessors. This prevents charging one group's price or subscription while consuming another group's capacity.

### Version cache payloads compatibly

Add `fallback_group_ids` to authentication snapshots and service objects. Missing data from an older cache payload decodes as an empty slice, so mixed-version deployments retain primary-only behavior until invalidation or refresh. Create/update publish the existing key-based invalidation event.

### Keep the UI order explicit

The personal key form uses an ordered list with add, remove, move-up, and move-down controls. Available choices exclude the selected primary group and already selected fallbacks. Reordering changes array order directly; labels resolve from the existing user-visible group list.

## Risks / Trade-offs

- [Protocol selectors have different signatures and retry loops] -> Introduce a small shared candidate-order helper, then add focused tests at each selection integration rather than redesigning all gateways.
- [Fallback can change pricing unexpectedly] -> Display order clearly and make the effective group authoritative in usage records and billing.
- [Deleted or disabled fallback groups remain in stored JSON] -> Skip ineligible groups during runtime selection and reject them on future edits; group deletion does not need to rewrite every key synchronously.
- [Additional selection attempts increase worst-case latency] -> Cap fallback groups at five and stop immediately after the first success or any non-selection error.
- [Billing type is unknown until a group wins] -> Prepare every candidate's billing context during authentication and publish only the winning candidate to downstream billing.
- [Mixed-version cache entries omit the new field] -> Treat omission as an empty list and rely on existing versioned invalidation.

## Migration Plan

1. Add `fallback_group_ids JSONB NOT NULL DEFAULT '[]'` through the project migration mechanism and regenerate Ent code.
2. Deploy readers that tolerate missing/empty fallback lists and include the field in cache snapshots.
3. Deploy API validation, routing behavior, and frontend controls.
4. Rollback is behaviorally safe because older binaries ignore the new column; the column can remain until a later cleanup migration.

## Open Questions

None. The initial implementation caps the list at five and advances only on pre-send account-selection exhaustion.
