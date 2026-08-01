# Technical Design Document: Operational Model Catalog — DB-Authoritative v3.1

**Status:** Draft for architecture approval; no production implementation yet
**Created:** 2026-07-18
**Revision:** v3.1, adding an explicit zero-dependency-I/O request hot path, process-local immutable pricing/policy cache, refresh backpressure, outage semantics, and load-test gates after architecture review feedback

---

## 1. Decision summary

Sub2API will move from an instance-local JSON pricing map to a **DB-authoritative operational model catalog**.

After cutover:

- LiteLLM JSON, bundled fallback data, and explicitly supported local/custom models are **import sources**, not runtime authorities.
- PostgreSQL is authoritative for canonical model identity, operator lifecycle policy, published source state, capabilities, and immutable base-price revisions.
- Runtime services read an immutable catalog projection by publication epoch. They do not query remote JSON and do not each implement their own model heuristics.
- PostgreSQL is the **control-plane authority**; the request data plane reads an immutable in-process snapshot. A normal user request performs no catalog SQL query and no catalog Redis query for enabled/status or base pricing.
- The existing `PricingService` compatibility facade may remain, but its read path delegates to the current catalog snapshot rather than downloading or querying DB per request.
- Every new model decision passes through one `ModelCatalogReader`/`CatalogReadView` data-plane contract before pricing, channel override, routing, upstream execution, or model-list publication.
- A valid newly imported model defaults to operator-enabled. There is no `visible_models` allowlist.
- Admin **disable** means operational disable, not presentation-only hiding: the model is removed from public model lists and rejected for new requests across all protocols.
- Admin **retire** is a terminal, audited lifecycle action. A later sync cannot silently reactivate it.
- Channel pricing, group permissions, account mappings, and live provider availability remain separate domain data. They may narrow or price an active model; they cannot make an unknown, disabled, retired, or source-unavailable model usable.
- Accepted requests pin the catalog/model/price revision used for their decision. Later disable or price changes affect new work only and do not invalidate settlement of already accepted work.

This is a material billing and request-admission migration. It must be rolled out in shadow phases with parity gates; it must not be implemented as a single flag flip.

---

## 2. Why v3 replaces v2

The v2 draft used one mutable catalog row containing `status` and `pricing_json`, plus Redis invalidation. That design is insufficient once the catalog becomes operational and billing-authoritative:

1. Mutable price rows cannot reproduce the price used by an older usage record or asynchronous batch job.
2. A single mutable `status` conflates upstream presence with deliberate operator policy.
3. Redis Pub/Sub alone can lose events and leave HA instances on divergent policy.
4. Gating only the inbound model name allows aliases, channel mappings, or account mappings to resolve to a disabled effective model.
5. Falling back to legacy JSON during rollback can resurrect a model disabled in DB.
6. Source sync and publication were not separated, so an incomplete upstream payload could mass-disable models.
7. The pseudo `_sync_seen` column in v2 was not part of its schema.
8. Generic request audit logging was not enough to make lifecycle publication and audit atomic.

V3 replaces those contracts with immutable source revisions, a mutable publication pointer, stable operator policy, durable outbox invalidation, and request-level revision pinning.

---

## 3. Goals

1. Make DB the single source of truth for model identity, policy status, published source state, capabilities, and base pricing.
2. Make every model list, admission decision, and base-price lookup use the same catalog projection.
3. Ensure a disabled or retired model cannot be revived by channel pricing, aliasing, account mapping, hard-coded fallbacks, or stale cache writers.
4. Keep existing billing formulas and multiplier semantics unchanged while replacing the source of base pricing.
5. Preserve exact pricing inputs for historical usage and in-flight settlement.
6. Import new valid models automatically as operator-enabled and publicly listed when they are actually billable.
7. Remove source-missing or invalid models from new decisions without deleting their history.
8. Publish a complete validated snapshot atomically, with HA cache recovery and rollback.
9. Provide admin visibility into source, lifecycle state, price version, routes, sync history, and reasons.
10. Migrate through measurable shadow parity, not assumptions.
11. Keep catalog admission and base-pricing lookup off PostgreSQL/Redis on the normal request hot path, with measured cache, concurrency, and outage gates.

## 4. Non-goals

- Replacing `channel_model_pricing`; it remains a scoped pricing override.
- Treating catalog presence as proof that a provider/account route is currently healthy.
- Moving account credentials, group routing, channel assignment, or gateway health into the catalog.
- Changing `rate_multiplier`, account multiplier, service-tier, cache-token, long-context, balance-burn, or V-Claw credit formulas.
- Allowing direct ad-hoc edits of every imported price in phase 1.
- Deleting historical model or price rows when a model disappears upstream.
- Letting frontend filtering repair incorrect backend totals or facets.
- Using Redis, process memory, remote JSON, or local files as an **independent source of truth** after cutover. Process memory is explicitly allowed as the immutable read-only runtime projection derived from the DB publication.

---

## 5. Confirmed current-state findings

### 5.1 Current pricing/catalog source

`backend/internal/service/pricing_service.go`:

- loads a local fallback JSON file;
- polls remote hash/content;
- writes a downloaded local file;
- keeps `pricingData map[string]ModelPricing` under an `RWMutex`;
- serves `GetModelPricing`, `ListModelPricingCatalog`, and catalog status from instance-local state.

Consequences:

- instances can temporarily disagree;
- there is no durable price revision for usage audit;
- JSON/fallback data can bypass any DB lifecycle policy unless the runtime path is fully cut over.

### 5.2 Marketplace and provider catalog are composite projections

`backend/internal/service/model_marketplace_service.go` combines pricing catalog rows, billing fallback pricing, gateway/default-model inference, group scope, search, facets, sorting, and pagination.

`backend/internal/service/provider_catalog.go` reads available channels from DB but still derives provider metadata, reasoning support, modality, context window, and token limits from hard-coded heuristics. It must become a consumer of the effective catalog projection rather than remain a parallel model authority.

### 5.3 Pricing and billing have multiple consumers

The base model price is used through:

- `PricingService.GetModelPricing`;
- `ModelPricingResolver`, including `channel_model_pricing` precedence;
- `BillingService.CalculateCost` and protocol wrappers;
- cache-tier, service-tier, long-context, image, and default-price branches;
- account statistics pricing;
- batch-image submit/hold/settlement.

A critical bypass exists if a channel override returns a complete per-request/image price before global model lifecycle is checked. The lifecycle gate must therefore be before **all** pricing precedence.

### 5.4 Request admission is distributed

Model-bearing entry points include Anthropic-compatible messages, OpenAI-compatible chat/responses/embeddings/images and related variants, Gemini native handlers, provider-specific paths, batch image, WebSocket/stream first-turn setup, model-list handlers, scheduler/account selection, mappings, and manifests.

No single existing middleware owns every protocol body. V3 therefore requires:

- one catalog decision contract;
- protocol adapters that call it immediately after parsing;
- a defense-in-depth check at scheduler/upstream selection boundaries;
- revision metadata propagated through request/billing context.

### 5.5 Repository patterns support the design

Verified repository precedents:

- `backend/internal/repository/migrations_runner.go` records filename/SHA256, rejects applied-migration edits, and serializes migration execution with a PostgreSQL advisory lock.
- `backend/migrations/README.md` reserves `*_notx.sql` for concurrent indexes.
- `backend/ent/generate.go` enables `sql/upsert`, `sql/execquery`, and `sql/lock`.
- repositories can reuse caller-owned Ent transactions through `dbent.TxFromContext(ctx)`.
- `scheduler_outbox` plus `SchedulerSnapshotService` demonstrate durable ordered events, deduplication, watermarks, retry, full rebuild, and fencing. The catalog needs a separate outbox/namespace; it must not overload scheduler-specific tables.
- batch image already persists effective price fields and a pricing schema snapshot version. Catalog revision identity must be added separately rather than overloading that existing integer.

---

## 6. Options considered

### Option A — DB only for marketplace display

Keep billing and request acceptance on JSON/fallback code; use DB only for admin and public pricing presentation.

**Rejected:** this cannot satisfy global disable and preserves split-brain authorities.

### Option B — direct DB query from every subsystem

Have handlers, billing, marketplace, scheduler, and model-list code query catalog tables independently.

**Rejected:** SQL and lifecycle semantics would be duplicated, hot-path latency would rise, and consumers would drift.

### Option C — DB authority plus immutable projection service

Use PostgreSQL revisions and policy as authority; a background projection loader publishes a versioned immutable process-local snapshot exposed through one `ModelCatalogReader`. Redis or a validated revision artifact may accelerate distribution/restart but is not in the per-request lookup path.

**Selected:** this gives one business authority while enabled/status, alias resolution, and base pricing remain local-memory reads with no DB/Redis round trip per normal user request.

---

## 7. Authority matrix

| Concern | Authority after cutover | Notes |
|---|---|---|
| Canonical model identity | DB catalog | Remote/local sources only propose revisions |
| Aliases and canonicalization | DB catalog projection | Deterministic and scope-aware |
| Operator enabled/disabled/retired | Stable DB policy row | Sync cannot overwrite operator policy |
| Source presence/validity | Active published catalog revision | Derived from validated import sources |
| Capabilities and limits | Active published model revision | Heuristic fallback removed after parity |
| Base pricing | Immutable active model revision | Used by billing resolver |
| Channel override | `channel_model_pricing` | Applied only after lifecycle gate |
| Group permission | Existing group policy | Intersected with active catalog |
| Account/channel mapping | Existing account/channel config | Mapped target is gated again |
| Live route availability | Existing scheduler/gateway data | Catalog active does not imply route exists |
| Public model lists | Effective projection | Active + source-present + billable + scoped route where applicable |
| Historic settlement | Pinned revision/snapshot | Never re-resolve against current price |

The result is one catalog authority, not one flat table for every operational concern.

### 7.1 Historical settlement pin

A catalog-admitted usage row carries the nullable revision metadata and a value-owned `pricing_snapshot` copy. At request-time billing, the exact materialized cost is stored under the reserved `_settlement_cost` member of that snapshot:

```json
{
  "input_cost_per_token": 0.000001,
  "output_cost_per_token": 0.000002,
  "_settlement_cost": {
    "total_cost": 0.003,
    "actual_cost": 0.006,
    "billing_mode": "token",
    "long_context_billing_applied": false
  }
}
```

`UsageBillingCommand` and every deferred/retry settlement path must read `_settlement_cost` when present. They must not resolve the current catalog, legacy pricing map, or channel pricing again after the request has been accepted. The scalar cost columns remain a backward-compatible fallback for catalog-pinned rows created before `_settlement_cost` existed. The snapshot hash/fingerprint includes this materialized settlement member, so a retry after a new publication remains idempotent and cost-stable.

This member preserves the existing shadow-mode billing authority: it freezes the cost produced by the authority active at request time; it does not enable catalog enforcement or change legacy pricing precedence before the cutover gate.

### 7.2 Durable settlement outbox and redelivery

The request-time settlement boundary is write-ahead and idempotent:

```text
usage log with immutable pricing_snapshot
  + usage_billing_settlements(status=pending, command=jsonb)
  -- one PostgreSQL transaction -->
try UsageBillingRepository.Apply(command)
  -> status=applied
```

`usage_billing_settlements` is a separate durable outbox keyed by `(request_id, api_key_id)`. It stores the normalized `UsageBillingCommand`, the usage-log reference, the request fingerprint, lease/attempt state, and the next retry time. A worker claims due rows with `FOR UPDATE SKIP LOCKED`; expired processing leases are reclaimable after a process restart. Transient failures move the row to `retry` with bounded exponential backoff.

The normal request path still attempts billing synchronously, but the usage row and outbox exist before that attempt. A crash between enqueue and `Apply`, an `Apply` error, or a crash before the applied acknowledgement therefore leaves a recoverable command. Re-delivery is safe because `usage_billing_dedup` and the request fingerprint protect balance, subscription, and quota effects from double application. A worker retry reads the persisted command/snapshot and never resolves a newly published catalog price.

The outbox worker owns critical database settlement recovery. Cache invalidation, last-used scheduling, and notifications remain post-Apply side effects and must not be treated as the durable billing authority; they require their own reconciliation/observability work before production cutover.

---

## 8. Lifecycle model

Do not encode all lifecycle facts in one mutable enum. Keep operator intent and source observation separate.

### 8.1 Operator state

| State | Meaning | Sync behavior |
|---|---|---|
| `enabled` | Operator permits new decisions if source and price are valid | Preserved across sync |
| `disabled` | Reversible administrative stop | Preserved across disappearance/reappearance |
| `retired` | Terminal withdrawal/supersession | Never silently reopened by sync |

A retirement reversal is an explicit audited transition to a new model/policy revision, not an incidental source refresh.

### 8.2 Published source state

| State | Meaning |
|---|---|
| `present` | At least one approved import source supplies a valid current definition |
| `missing` | No approved source supplies the model in the validated published snapshot |
| `invalid` | Source row exists but fails required normalization/pricing invariants |

### 8.3 Effective lifecycle state

```text
retired     if operator_state = retired
disabled    if operator_state = disabled
unavailable if source_state != present
active      if operator_state = enabled and source_state = present
```

This is the global lifecycle result. **Decision eligibility is scope-aware and stricter:**

```text
eligible(request scope)
  = lifecycle state is active
  AND requested/effective mappings remain active
  AND selected scope has a complete valid price
  AND selected scope has a permitted route
```

A model may therefore be lifecycle-active but not globally billable when it has no global base price; a complete channel override can make it billable only in that channel scope. Pricing is still evaluated only after the lifecycle gate.

### 8.4 New, missing, and returning models

- A newly imported model with valid identity and pricing gets `operator_state=enabled`; it is active by default. No admin allowlist is required.
- A model missing from one source remains present if another approved source still provides it.
- After a validated full publication declares a model source-missing, it is unavailable for new calls and public lists. Historical rows remain.
- A source-missing model that returns becomes active again only if its operator state remains `enabled`.
- A manually disabled or retired model remains disabled/retired when it returns.
- A malformed or suspiciously incomplete source payload is rejected before publication; it does not mark the current catalog missing.

### 8.5 Presentation coupling

For this approved operational scope, public listing is derived from effective `active`; there is no independent `is_hidden` flag. If a future product requirement needs a private-but-callable model, add an explicit publication policy then. Do not overload lifecycle state prematurely.

## 9. Data model

Use forward-only SQL migrations, currently expected to start at the next available number after `182_prompt_audit_full_prompt.sql`. Do not modify an applied migration. Keep Ent schema and generated code synchronized with hand-authored SQL.

### 9.1 `catalog_models` — stable identity and operator policy

Representative fields:

```text
id BIGSERIAL PK
canonical_key TEXT NOT NULL
canonical_key_normalized TEXT NOT NULL UNIQUE
operator_state TEXT NOT NULL CHECK (enabled, disabled, retired)
operator_reason TEXT NULL
replacement_model_id BIGINT NULL REFERENCES catalog_models(id)
operator_version BIGINT NOT NULL DEFAULT 1
first_seen_at TIMESTAMPTZ NOT NULL
last_operator_change_at TIMESTAMPTZ NULL
retired_at TIMESTAMPTZ NULL
created_at TIMESTAMPTZ NOT NULL
updated_at TIMESTAMPTZ NOT NULL
```

This table is stable across source revisions. Sync may create a new model with `enabled`; it may update observation metadata through a controlled transaction, but it must never overwrite `disabled` or `retired` operator policy.

### 9.2 `catalog_sync_runs` — every import attempt

```text
id BIGSERIAL PK
source_set TEXT NOT NULL
trigger TEXT NOT NULL
actor_user_id BIGINT NULL
upstream_version TEXT NULL
upstream_etag TEXT NULL
upstream_hash TEXT NULL
normalized_hash TEXT NULL
normalizer_version TEXT NOT NULL
status TEXT NOT NULL
source_count INT NOT NULL DEFAULT 0
normalized_count INT NOT NULL DEFAULT 0
added_count INT NOT NULL DEFAULT 0
changed_count INT NOT NULL DEFAULT 0
missing_count INT NOT NULL DEFAULT 0
invalid_count INT NOT NULL DEFAULT 0
validation_errors JSONB NOT NULL DEFAULT '[]'
started_at TIMESTAMPTZ NOT NULL
completed_at TIMESTAMPTZ NULL
```

Retain failed/rejected runs. They are operational evidence and must not mutate active publication.

### 9.3 `catalog_revisions` — immutable candidate snapshots

```text
id BIGSERIAL PK
revision BIGINT NOT NULL UNIQUE
sync_run_id BIGINT NOT NULL REFERENCES catalog_sync_runs(id)
normalized_hash TEXT NOT NULL UNIQUE
normalizer_version TEXT NOT NULL
state TEXT NOT NULL
model_count INT NOT NULL
created_at TIMESTAMPTZ NOT NULL
validated_at TIMESTAMPTZ NULL
published_at TIMESTAMPTZ NULL
```

Rows and their child model revisions are immutable after validation. A rollback creates a new forward publication event pointing at previously validated immutable content; it never edits old content.

### 9.4 `catalog_model_revisions` — immutable model definition and base price

```text
id BIGSERIAL PK
catalog_revision_id BIGINT NOT NULL REFERENCES catalog_revisions(id)
model_id BIGINT NOT NULL REFERENCES catalog_models(id)
source_state TEXT NOT NULL CHECK (present, missing, invalid)
provider TEXT NOT NULL
platform TEXT NOT NULL
mode TEXT NOT NULL
capabilities JSONB NOT NULL
context_window BIGINT NULL
max_output_tokens BIGINT NULL
pricing_schema_version INT NOT NULL
pricing_json JSONB NULL
pricing_valid BOOLEAN NOT NULL
pricing_source TEXT NULL
source_metadata JSONB NOT NULL DEFAULT '{}'
source_hash TEXT NOT NULL
UNIQUE (catalog_revision_id, model_id)
```

`pricing_json` is not arbitrary raw upstream JSON. It is a normalized serialization of the complete Go `ModelPricing` contract after validation. It must preserve all billing-relevant fields, including token input/output, cache read/write variants, priority/service-tier variants, image token/output-image pricing, long-context threshold/multipliers, and future schema-versioned fields.

Core filter/sort fields should use typed columns. Values used only by billing may remain in the validated schema-versioned JSON payload so the projection can deserialize exactly one typed contract. The implementation must not maintain a second partial numeric price representation with different precedence.

### 9.5 `catalog_model_aliases`

```text
id BIGSERIAL PK
alias_normalized TEXT NOT NULL
platform_scope TEXT NOT NULL DEFAULT '*'
model_id BIGINT NOT NULL REFERENCES catalog_models(id)
source TEXT NOT NULL
state TEXT NOT NULL
introduced_revision_id BIGINT NULL
retired_revision_id BIGINT NULL
created_at TIMESTAMPTZ NOT NULL
updated_at TIMESTAMPTZ NOT NULL
UNIQUE (platform_scope, alias_normalized)
```

Canonicalization is exact and deterministic after normalization. Runtime fuzzy family fallback must not be allowed to make an unknown or disabled model billable. Existing accepted fuzzy/version aliases must be inventoried and backfilled explicitly before strict cutover.

### 9.6 `catalog_publications` — mutable active pointer

Initially one global scope is sufficient. The implementation uses a surrogate
`id BIGSERIAL` technical primary key because this repository's Ent generation
uses one global `int64` ID type; `scope` remains `NOT NULL UNIQUE` and is the
business identity used for publication fencing:

```text
id BIGSERIAL PK
scope TEXT NOT NULL UNIQUE
active_revision_id BIGINT NOT NULL REFERENCES catalog_revisions(id)
epoch BIGINT NOT NULL
updated_at TIMESTAMPTZ NOT NULL
```

The publication row is the only mutable current-catalog pointer. Apply locks it with `FOR UPDATE`, verifies the expected predecessor/epoch, advances the revision, and increments `epoch`.

### 9.7 `catalog_lifecycle_audits`

Append-only business audit:

```text
id BIGSERIAL PK
model_id BIGINT NULL
catalog_revision_id BIGINT NULL
action TEXT NOT NULL
actor_type TEXT NOT NULL
actor_user_id BIGINT NULL
reason TEXT NULL
before_state JSONB NULL
after_state JSONB NOT NULL
request_id TEXT NULL
correlation_id TEXT NULL
created_at TIMESTAMPTZ NOT NULL
```

Lifecycle audit is written in the same transaction as policy/publication mutation. General request middleware logging is supplementary, not authoritative.

### 9.8 `catalog_outbox`

```text
id BIGSERIAL PK
event_type TEXT NOT NULL
scope TEXT NOT NULL
publication_epoch BIGINT NOT NULL
catalog_revision_id BIGINT NOT NULL
model_id BIGINT NULL
payload JSONB NULL
dedup_key TEXT NULL
created_at TIMESTAMPTZ NOT NULL
```

Use a partial unique index for active dedup keys and ordered reads/watermarks modeled after `scheduler_outbox`, but keep a separate table, worker, cleanup lease, metrics, and cache namespace.

### 9.9 Usage revision references

Add nullable revision references during dual-write rollout:

```text
usage_logs.catalog_epoch BIGINT NULL
usage_logs.catalog_revision_id BIGINT NULL
usage_logs.requested_model_revision_id BIGINT NULL
usage_logs.effective_model_revision_id BIGINT NULL
usage_logs.pricing_source TEXT NULL
usage_logs.pricing_snapshot JSONB NULL
```

A complete immutable resolved-pricing snapshot is preferable for audit and protects against future parser/schema evolution. Foreign keys may be omitted or `NOT VALID` initially on very large tables, but application-level references must be immutable.

For batch image, keep existing `pricing_snapshot_version`, `base_unit_price`, multipliers, `billable_unit_price`, and `hold_unit_price`. Add catalog/model revision IDs separately; do not reinterpret the existing schema-version integer as a catalog publication ID.

---

## 10. Source set and normalization

### 10.1 Import sources at cutover

The initial candidate must represent the union of every model name that can currently influence behavior:

1. remote LiteLLM pricing JSON;
2. bundled local pricing fallback JSON;
3. Sub2API hard-coded billing fallback entries;
4. `channel_model_pricing` model keys;
5. group custom-model lists;
6. channel/account model mapping keys and values;
7. gateway/provider default model lists;
8. provider-specific manifest/static model lists;
9. batch-image supported model mappings.

This inventory prevents a legitimate custom model from disappearing solely because it is absent from LiteLLM.

Every source record must carry source identity and precedence. A model is source-present when at least one **approved enabled source** supplies a valid definition. Channel pricing and routing data can nominate a model identity during backfill but do not silently become global base-price authority.

### 10.2 Base-price precedence

Preserve current behavior explicitly in one normalizer/resolver:

```text
approved remote LiteLLM price
    > approved bundled/local base-price fallback
    > explicit imported Sub2API fallback
    > no global base price
```

`channel_model_pricing` is not part of global base-price precedence. It is applied later by `ModelPricingResolver` for the selected scope.

A model without a global price may still be billable only if the selected channel override supplies a complete pricing mode. It must still exist and be effective-active in the catalog. Unpriced/invalid models must fail before upstream execution rather than run unbilled.

### 10.3 Normalization invariants

Before a candidate can validate:

- canonical key is nonempty and uniquely normalized;
- alias target exists and aliases do not collide in scope;
- provider/platform/mode values are recognized or intentionally extensible;
- context and output limits are nonnegative;
- every price/multiplier is finite and nonnegative;
- required price fields for the declared pricing mode are present;
- long-context threshold and multipliers form a valid pair;
- JSON schema version is supported by all candidate readers;
- duplicate normalized models resolve deterministically or reject;
- candidate source count and deletion ratio pass safety thresholds;
- complete candidate hash is stable under ordering.

---

## 11. Sync, validate, and publish

### 11.1 Fetch and stage are not publication

1. Acquire a dedicated sync lease/advisory lock so scheduler and manual sync do not run concurrently.
2. Fetch source data and read approved local sources.
3. Parse, normalize, merge precedence, canonicalize, and hash outside the publication transaction.
4. Insert `catalog_sync_runs` and immutable staged revision/model rows.
5. Run schema, price, identity, alias, count, and diff validation.
6. Mark the candidate `validated` only when every invariant passes.
7. Keep current publication unchanged on any fetch, parse, validation, or staging failure.

### 11.2 Destructive-diff guard

A truncated upstream response must not disable most models. At minimum:

- reject a candidate below a configured fraction of the prior valid source count;
- reject removal spikes beyond a configured count/percentage;
- reject provider-wide disappearance unless explicitly approved;
- show all added/changed/missing/invalid rows in admin preview;
- require explicit override/reason for a quarantined destructive candidate.

Individual source removals in an otherwise valid snapshot become effective source-missing immediately after publication, matching the requirement that obsolete models stop being used.

### 11.3 Atomic apply transaction

Publication uses one Ent transaction at the service boundary:

1. wrap context with `dbent.NewTxContext`;
2. all repositories reuse `dbent.TxFromContext(ctx)`;
3. lock `catalog_publications(scope='global') FOR UPDATE`;
4. verify candidate is validated and expected predecessor epoch still matches;
5. create stable model rows for genuinely new identities with operator state `enabled`;
6. never overwrite existing disabled/retired operator policy;
7. mark immutable candidate published/supersede prior revision;
8. advance `active_revision_id`, increment epoch;
9. write lifecycle/publication audits;
10. write deduplicated `catalog_outbox` event;
11. commit;
12. only after commit rebuild or invalidate projections.

If audit or outbox insertion fails, the transaction rolls back and the old publication remains authoritative.

### 11.4 Manual sync

Admin manual sync and scheduled sync use the same service. The API supports:

- preview candidate/diff;
- apply a validated non-quarantined candidate;
- reject/quarantine with reason;
- inspect failed runs.

It never writes back to LiteLLM JSON and never directly mutates process-local pricing maps.

## 12. Runtime projection and HA invalidation

### 12.1 Projection contract

`CatalogProjectionService` loads one immutable projection for a publication epoch containing:

- canonical model and scoped alias indexes;
- operator/source/effective state;
- provider/platform/mode and capabilities;
- immutable normalized `ModelPricing` plus model revision IDs;
- source and revision metadata required for diagnostics.

Readers hold an atomic pointer to the complete projection. They never mutate maps in place.

### 12.2 DB authority and cache layers

The catalog uses separate control, refresh, and request data planes:

```text
PostgreSQL
  = control-plane authority: policy, revisions, publication epoch, audit, outbox

Catalog outbox + background watcher
  = refresh plane: notify, load, validate, build, and atomically publish snapshots

Process-local immutable snapshot
  = request data plane: O(1) model/status/alias/base-price reads
```

Required cache layers:

- **L1 process memory — mandatory hot-path cache:** one immutable `CatalogSnapshot` held by an `atomic.Pointer` or equivalent read-safe publication primitive. It contains canonical/alias indexes, effective lifecycle state, capabilities, normalized `ModelPricing`, revision IDs, epoch, loaded time, and source metadata. Request readers never mutate it in place.
- **L2 Redis/revision artifact — optional distribution and restart cache:** a revision-named serialized snapshot plus epoch/checksum may be stored in Redis and, where deployment storage semantics support it, as an atomically written local cache artifact. It is read only at startup/refresh, never by each user request, and is not an independent authority. A cache artifact must include the DB publication epoch, revision ID, schema version, built hash, and creation/verification time; remote/local LiteLLM JSON is not a substitute because it lacks operator policy.
- **L3 PostgreSQL — control-plane source:** used by sync/apply, admin mutations, startup load, background rebuild, audit, and outbox recovery. It is not used for per-request catalog status or base-price reads.

The current `PricingService.pricingData` map already demonstrates the required local-read shape (`RWMutex` protected map). The implementation should replace its mutable/JSON-authoritative role with an immutable DB-derived snapshot. A compatibility `PricingService.GetModelPricing` call must resolve from that snapshot in O(1) time.

A normal user request must not:

- execute `SELECT` against catalog tables to check enabled/disabled;
- query Redis for catalog status or base price;
- download/read the remote JSON;
- wait on a snapshot rebuild lock;
- mutate a shared pricing map.

Channel, group, account, scheduler, and channel-pricing data retain their existing cache/snapshot boundaries. The catalog snapshot supplies global model identity/lifecycle/base pricing; it does not force every unrelated route/config lookup onto the catalog DB.

### 12.3 Snapshot refresh and disable propagation

Publication and snapshot refresh are asynchronous to user requests:

1. Sync or admin lifecycle mutation commits DB publication, audit, and outbox in one transaction.
2. The publishing instance receives/creates the new epoch event.
3. A background worker on each instance consumes the catalog outbox, with optional best-effort Redis notification to reduce latency.
4. The worker loads the immutable revision from DB **once per revision**, validates hash/schema, and builds a complete snapshot off the request path.
5. On successful build, it atomically swaps the process-local pointer from epoch N to N+1.
6. On failure, the worker retains the previous complete snapshot, records lag/error metrics, and retries with backoff. It never installs a partial map.
7. A periodic watermark/epoch poll recovers missed notifications. It is one background check per instance/interval, not one query per user request.

No request waits for the worker. Requests that began before the swap use their already loaded snapshot/decision; requests after the swap use the new snapshot. The returned `CatalogDecision` carries the epoch and revision used.

Target propagation SLO: all healthy instances observe a committed epoch within 2 seconds under normal worker/Redis/DB health. This is an operational propagation bound, not a DB lookup on every request.

When a node observes epoch N, it must never accept a decision from an older epoch after the local pointer has advanced. Epoch fencing prevents a delayed rebuild for N from overwriting N+1.

### 12.4 Snapshot freshness, outage, and backpressure policy

The cache policy is designed to protect the high-concurrency request path without silently losing lifecycle safety:

| Condition | Request-path behavior | Background/control-plane behavior |
|---|---|---|
| Fresh valid snapshot | Serve from local memory; no DB/Redis catalog read | Normal watermark/health checks |
| DB temporarily unavailable, existing snapshot within `max_stale` | Continue from last-known-good local snapshot; emit degraded metrics | Retry rebuild; do not destroy snapshot |
| Remote JSON unavailable | No request impact | Keep last published DB revision; retry import |
| Snapshot build fails | Continue old complete snapshot | Keep candidate unpublished, retry, alert |
| No snapshot at startup | Process remains unready; no model traffic | Load DB publication before readiness |
| Snapshot older than configured strict `max_stale` | Fail new model decisions with 503 `catalog_unavailable`; do not query DB synchronously per request | Alert, retry, and require successful rebuild |
| Outbox notification missed | No immediate request lock | Watermark poll detects epoch gap and rebuilds |
| High refresh frequency | Coalesce revisions and build only latest needed epoch | Bound one rebuild per model revision/worker queue |

Treat **known stale** and **temporarily unverified** differently. If a node has observed `expected_epoch > local_epoch`, it must fail new admissions until it installs at least that epoch; continuing would knowingly bypass committed policy. If it has no evidence of a newer epoch but the watcher cannot verify dependencies, it may serve the last-known-good snapshot only within `max_stale`. This preserves a bounded availability window without claiming a disable update was observed when it was not.

`max_stale` is a safety/availability configuration, not a reason to put SQL on the hot path. Freshness is represented by local atomic health state such as `last_epoch_verified_at`, updated only by the background watcher when it confirms the active epoch/watermark. A request reads that local atomic value; it does not verify freshness by calling DB or Redis. Snapshot `LoadedAt` and epoch `VerifiedAt` must be tracked separately so merely rebuilding or restarting cannot fake control-plane freshness.

The chosen `max_stale` value must be load-tested and documented with the disable-propagation SLO. A stale-cache rejection is preferable to allowing a node with unverifiable policy to serve indefinitely; a short DB outage within the window must not cause a database connection storm or broad request failure.

The refresh worker must have bounded concurrency, exponential backoff, jitter, and queue coalescing. It must not consume request goroutines or exhaust the DB pool.

### 12.5 Snapshot shape and memory behavior

Representative runtime shape:

```go
type CatalogSnapshot struct {
    Epoch        int64
    RevisionID   int64
    LoadedAt     time.Time
    Models       map[string]CatalogRuntimeModel
    Aliases      map[ScopedAlias]string
    Lookup       map[string]string // precomputed normalized/legacy/family candidate -> canonical
    BuiltHash    string
}

type CatalogRuntimeModel struct {
    Canonical        string
    OperatorState    OperatorState
    SourceState      SourceState
    Provider         string
    Platform         string
    Mode             string
    Capabilities     CatalogCapabilities
    Pricing          *ModelPricing
    ModelRevisionID  int64
}

type CatalogRuntimeCache struct {
    Snapshot            atomic.Pointer[CatalogSnapshot]
    LastEpochVerifiedAt atomic.Int64 // Unix time, background-writer/local-reader only
}
```

The exact Go types may differ. Requirements are:

- maps and nested pricing values are fully built before publication;
- callers do not receive pointers that can be mutated by a later refresh;
- snapshot memory is bounded by catalog size, not request count;
- snapshot indexes precompute current normalization, legacy-name, family, and scoped-alias candidates so request lookup does not scan the catalog map;
- repeated model requests reuse the same parsed pricing object/value;
- no per-request JSON unmarshal, DB row scan, remote fetch, or O(catalog-size) fuzzy scan;
- old snapshots are released after in-flight readers finish through normal Go ownership/GC.

A catalog of the current scale (hundreds of model entries, including complete pricing/capability metadata) is small relative to the application process and should be benchmarked as a bounded memory budget. The implementation must measure snapshot bytes, rebuild duration, and allocation rate rather than assume.

### 12.6 Disable propagation contract

No distributed system can make a post-commit change visible to all nodes at the exact same nanosecond without coordination. Define an explicit operational contract:

- admin mutation returns committed epoch and propagation state;
- the request path performs only a local snapshot epoch/freshness read;
- epoch verification is performed by a background watcher, not a per-request DB/Redis call;
- once a node observes epoch N, it may never accept a decision from an older epoch;
- if freshness cannot be recovered beyond strict `max_stale`, new model decisions fail closed with `catalog_unavailable` rather than use arbitrarily stale policy;
- requests accepted under an earlier valid epoch may finish and settle from their pinned decision.

Metrics must expose active DB epoch, Redis/watermark epoch, local epoch per instance, snapshot age, propagation lag, outbox lag, rebuild failures, refresh queue depth, and stale-decision rejections.

### 12.7 Startup and failure behavior

| Situation | Behavior |
|---|---|
| DB has active publication | Load, validate, and build local snapshot before readiness |
| DB unavailable at restart, validated DB-derived L2 revision cache exists within configured `max_stale` window | Start degraded from that exact revision cache, mark epoch unverified, and retry DB in background; never reconstruct policy from LiteLLM JSON |
| DB empty on first deployment | Bootstrap through importer/validator/publication, then build snapshot |
| Remote source unavailable | Keep last published DB revision; do not replace it |
| Redis unavailable, DB available | Background worker/watermark rebuilds from DB; requests keep local snapshot |
| DB unavailable, fresh verified projection exists | Continue only within strict freshness window |
| DB and epoch verification unavailable beyond window | Fail new decisions with 503; do not fall back to JSON |
| Process starts with no valid projection | Remain unready |

After DB-authoritative cutover, remote/local JSON is not a hot-path emergency authority because it cannot carry the current operator disable policy.

---

## 13. `ModelCatalogReader` and projection contracts

Representative interfaces:

```go
type CatalogDecision struct {
    CatalogRevisionID         int64
    CatalogEpoch              int64
    RequestedModelID          int64
    RequestedModelRevisionID  int64
    EffectiveModelID          int64
    EffectiveModelRevisionID  int64
    RequestedCanonical        string
    EffectiveCanonical        string
    BasePricing               *ModelPricing
    PricingSource             string
    ResolvedPricingSnapshot   json.RawMessage
}

// CatalogReadView pins one immutable local snapshot for one admission decision.
// Requested-model and effective-mapped-model gates therefore cannot observe
// different epochs during an atomic refresh.
type CatalogReadView interface {
    Epoch() int64
    ResolveRequested(requested, platform string) (CatalogDecision, error)
    ResolveMapped(prior CatalogDecision, mapped, platform string) (CatalogDecision, error)
    ResolveBasePricing(decision CatalogDecision) (*ModelPricing, error)
}

// ModelCatalogReader is the request data-plane contract.
// BeginDecision/ListEffective read only the process-local snapshot.
// Implementations must not perform SQL, Redis, file, or network I/O.
type ModelCatalogReader interface {
    BeginDecision(now time.Time) (CatalogReadView, error)
    ListEffective(scope CatalogScope) ([]CatalogModel, error)
    LocalEpoch() int64
    SnapshotHealth(now time.Time) CatalogSnapshotHealth
}

// CatalogProjectionLoader is the background/control-plane contract.
// It is never called synchronously by a user request.
type CatalogProjectionLoader interface {
    LoadPublication(ctx context.Context, revisionID, epoch int64) (*CatalogSnapshot, error)
    RefreshToLatest(ctx context.Context) error
}
```

Exact types may change, but this interface split is mandatory: request callers depend only on `ModelCatalogReader`/`CatalogReadView`, while DB/Redis/outbox access is isolated behind the background projection loader. `BeginDecision` atomically acquires one immutable view that is reused through requested-model gate, mapping, effective-model gate, and base-price resolution. The view returns revision identity with the model/pricing decision, and callers must not re-resolve current pricing later.

### 13.1 Error taxonomy

Internal typed errors:

| Condition | Internal error | External behavior |
|---|---|---|
| unknown model/alias | `ErrCatalogModelNotFound` | protocol-shaped 404/model_not_found |
| operator disabled | `ErrCatalogModelDisabled` | protocol-shaped 503/model_unavailable |
| retired | `ErrCatalogModelRetired` | protocol-shaped 404/model_not_found |
| source missing/invalid | `ErrCatalogModelUnavailable` | protocol-shaped 503/model_unavailable |
| no effective price | `ErrCatalogPricingUnavailable` | 503 before upstream call |
| projection not fresh/ready | `ErrCatalogNotReady` | 503 catalog_unavailable |

Public error bodies follow each protocol's existing envelope; logs/admin diagnostics retain exact lifecycle reason without exposing internal details unnecessarily.

---

## 14. Request admission and mapping

### 14.1 Required ordering

```text
parse request
  -> BeginDecision(): atomically pin one local CatalogReadView (no DB/Redis I/O)
  -> normalize/resolve requested model against the pinned view
  -> GATE requested canonical state
  -> apply group/channel/account/provider mapping
  -> normalize/resolve effective mapped model or offering against the same view
  -> GATE effective canonical state
  -> verify group permission and routable account/channel
  -> resolve pricing (channel override or pinned-view base price)
  -> persist/propagate CatalogDecision
  -> call upstream
  -> settle from pinned decision/snapshot
```

Both gates are mandatory and use the same immutable view. Gating only the requested name allows an active alias to map to a disabled upstream target; gating only the mapped target can expose a disabled public alias or bypass policy analytics. Loading the local snapshot twice would risk a mixed-epoch decision during refresh, so the request pins one `CatalogReadView` before either gate.

### 14.2 Defense in depth

Because protocol handlers parse different body shapes, use layered enforcement:

1. shared protocol adapter immediately after request model extraction;
2. scheduler/account-selection boundary verifies the supplied `CatalogDecision` and effective mapped model;
3. billing resolver rejects missing/invalid/stale catalog decisions;
4. tests assert no upstream client is called on any catalog rejection.

The scheduler defense must validate, not independently canonicalize with different rules.

### 14.3 In-flight requests

Disable/retire applies to **new admission decisions** after the new epoch is observed.

- already accepted streaming requests may complete;
- usage is billed with the pinned price revision/snapshot;
- queued async/batch work accepted under a valid decision may execute and settle under that snapshot unless a separate explicit cancellation policy is introduced;
- retries must preserve the original decision for the same accepted job, not re-price at retry time;
- a new user retry is a new admission and sees current policy.

This avoids unbilled completed work and nondeterministic invoices.

### 14.4 Admission surfaces requiring parity/enforcement

The implementation inventory must cover at least:

- Anthropic messages and count-token related model paths;
- OpenAI chat completions, responses, embeddings, images, rerank/search or supported variants;
- Gemini native generate/stream/count-token/model paths;
- Grok/provider-specific handlers;
- WebSocket/stream first-turn model setup;
- batch image submission and worker retry context;
- OpenAI/general scheduler and account selection;
- channel/group/account model mappings;
- provider-specific passthrough/manifests.

A route-level test matrix, not a comment claiming one central gate, is the completion proof.

---

## 15. Billing and pricing resolution

### 15.1 Status gate precedes all pricing precedence

The effective resolver order is:

```text
CatalogDecision must be valid/effective-active
    -> complete channel_model_pricing override, if applicable
    -> immutable DB base-price revision
    -> pricing unavailable (fail before upstream)
```

A channel override cannot activate an unknown, disabled, retired, or source-unavailable model, including per-request and image overrides.

### 15.2 Preserve formulas, replace only the authority

Initial cutover must preserve current formula behavior for:

- input/output token prices;
- cache read and cache creation, including 5m/1h distinctions;
- service-tier/priority prices;
- long-context threshold and multipliers;
- image input/output token and output-image pricing;
- group `rate_multiplier`;
- account multiplier;
- channel pricing modes;
- batch discount and hold multiplier;
- final `TotalCost -> ActualCost -> users.balance` flow.

No extra conversion denominator is introduced for V-Claw credit accounting.

### 15.3 Price pinning

At admission/submit time, capture:

- catalog epoch and revision;
- requested/effective model revision;
- base price revision/payload;
- selected channel pricing record/version or normalized override snapshot;
- billing mode/tier;
- group/account multipliers used;
- any long-context/service-tier decisions available at settlement.

Synchronous token usage may know final token counts only after response, but unit prices and resolver provenance are pinned before upstream execution. Settlement multiplies actual usage by those pinned units.

### 15.4 Usage logs

Dual-write the new revision IDs and normalized pricing snapshot while retaining existing fields such as requested model, upstream model, mapping chain, billing tier/mode, token counts, component costs, total cost, actual cost, and multipliers.

A catalog publication or price change after admission must not alter an already written or pending usage cost.

### 15.5 Account statistics and estimates

Account statistics, preflight cost estimates, marketplace pricing, and runtime billing should call shared catalog/resolver code. Presentation may format or aggregate results but may not implement a separate fallback formula.

---

## 16. Model listing and discovery

### 16.1 Public model marketplace

Pipeline:

```text
active catalog projection
  -> effective-active and valid-price filter
  -> group/channel route scope
  -> search/provider/mode/capability filters
  -> sort
  -> totals/pages/facets
  -> pagination
```

Lifecycle and scope filtering occur before totals, facets, and pagination. Frontend must not filter a server page after receipt.

### 16.2 `/v1/models` and provider manifests

Every model-list response must be an effective projection or an intersection of upstream manifest with effective catalog policy. No provider response may be passed through unfiltered if it can expose a disabled/retired model.

This includes OpenAI-compatible model lists, Codex/provider manifests, Gemini model lists, static Antigravity/provider lists, and any group-scoped discovery endpoint.

### 16.3 `ProviderCatalogService`

Replace model capability heuristics in `provider_catalog.go` with catalog revision fields after shadow parity. Protocol metadata that is truly platform-level may remain code/config, but model reasoning, modalities, context window, max tokens, and compatibility flags come from the catalog projection.

### 16.4 Admin list

Admin sees all stable models, including disabled, retired, missing, and invalid. It must show operator state separately from source/effective state so an operator can understand why a model is not callable.

## 17. Admin API and UI contracts

All mutating APIs require admin authentication, validation, optimistic concurrency, a reason for destructive actions, business audit, and catalog outbox publication.

### 17.1 List and inspect

```text
GET /api/v1/admin/model-catalog
  ?page=&page_size=&query=&provider=&platform=
  &operator_state=&source_state=&effective_state=

GET /api/v1/admin/model-catalog/{id}
GET /api/v1/admin/model-catalog/{id}/history
GET /api/v1/admin/model-catalog/sync-runs
GET /api/v1/admin/model-catalog/sync-runs/{id}
```

Responses include stable identity, current revision, operator/source/effective state, reason, provider/platform/mode, capabilities, price summary/source, first seen, current/previous revision, route/account/channel counts for diagnostics, and replacement model where present.

Route counts are diagnostic projections, not catalog authority.

### 17.2 Lifecycle mutation

```text
PATCH /api/v1/admin/model-catalog/{id}/lifecycle
If-Match: <operator_version-or-publication-epoch>
{
  "operator_state": "enabled" | "disabled" | "retired",
  "reason": "required for disabled/retired",
  "replacement_model_id": 123
}
```

The transaction:

1. locks stable model policy/current publication as required;
2. verifies optimistic version;
3. validates transition;
4. increments operator/publication epoch;
5. writes append-only lifecycle audit;
6. writes catalog outbox event;
7. commits atomically.

Bulk disable/enable may be added but must preserve per-model audit reasons and all-or-nothing or explicit partial-result semantics.

### 17.3 Sync APIs

```text
POST /api/v1/admin/model-catalog/sync/preview
POST /api/v1/admin/model-catalog/sync-runs/{id}/publish
POST /api/v1/admin/model-catalog/sync-runs/{id}/reject
POST /api/v1/admin/model-catalog/publications/rollback
```

Publishing requires the expected predecessor epoch. Rollback is a forward, audited publication to previously validated content with a new epoch; it is not a destructive SQL down migration.

### 17.4 Admin UI

Recommended navigation: **Admin -> Mô hình**.

Minimum UI:

- server-paged table;
- search and provider/platform/operator/source/effective filters;
- separate badges for operator and source/effective state;
- price source/current revision and last changed timestamp;
- route/channel/account diagnostic counts;
- disable, re-enable, retire actions with reason dialog;
- sync preview with add/change/missing/invalid diff;
- publication and lifecycle history;
- destructive-diff quarantine warning;
- propagation epoch/status after mutation.

All Vietnamese UI copy and accessibility labels use the existing i18n architecture. Do not introduce raw user-facing literals or fallback English strings.

---

## 18. Rollout plan

Use independent modes so catalog import, listing, billing, and enforcement can be observed separately:

```text
CATALOG_IMPORT_MODE=off|shadow|publish
CATALOG_LIST_READ_MODE=legacy|shadow|db
CATALOG_PRICING_READ_MODE=legacy|shadow|db
CATALOG_ADMISSION_MODE=off|observe|enforce
```

Exact configuration location may use DB settings rather than environment flags, but semantics must remain independently switchable and auditable.

### Phase 0 — freeze and inventory

- enumerate all model sources and consumers;
- capture a canonical legacy fixture from remote/local/fallback/custom/channel/mapping/provider data;
- document current pricing precedence for every mode;
- add metrics for existing pricing source/fallback usage;
- define parity tolerances and expected exceptions.

**Exit:** every currently accepted/listed/priced model has a classified source and expected catalog representation.

### Phase 1 — additive schema and repository

- add stable model, sync run, immutable revision, model revision, alias, publication, lifecycle audit, and catalog outbox tables;
- add nullable usage revision/snapshot columns;
- add Ent schemas and generate code;
- implement transaction reuse and concurrent publication locking;
- no runtime behavior change.

Large indexes use a separate next-numbered `_notx.sql` migration only when concurrent creation is necessary.

**Exit:** migration/repository integration tests pass and generation is idempotent.

### Phase 2 — shadow importer/publication

- import the complete source union;
- validate and publish DB revisions without serving production decisions;
- compare normalized model counts, identities, aliases, capabilities, and all price fields;
- exercise failed-source and destructive-diff rejection.

**Exit:** stable revisions across repeated identical imports; no unresolved identity or price mismatch.

### Phase 3 — DB projection for display/discovery

- run legacy and DB model marketplace/provider catalog/list outputs in parallel;
- compare canonical content while ignoring presentation-only ordering;
- cut public listing to DB after strict parity;
- keep billing and admission legacy.

**Exit:** totals, facets, pagination, group scope, and provider manifests have parity or approved intentional deltas.

### Phase 4 — billing shadow

- for sampled or all eligible requests, calculate legacy and DB-backed effective pricing without changing debit;
- compare component unit prices, pricing mode/source, long-context/service-tier/cache decisions, multipliers, TotalCost, ActualCost, and projected balance deduction;
- dual-write catalog revision/snapshot metadata.

**Exit:** zero unexplained billing mismatches over an agreed traffic window and fixture suite.

### Phase 5 — DB base-pricing authority

- switch runtime resolver to immutable DB base price;
- keep channel override precedence unchanged but behind catalog lifecycle gate in observe mode;
- continue legacy shadow calculation;
- monitor cost and fallback deltas.

**Exit:** no regression in synchronous or batch settlement and no unpriced upstream calls.

### Phase 6 — admission observe then enforce

- adapters and schedulers resolve/gate but initially only record would-reject decisions;
- classify every false positive, especially custom models and mapping targets;
- enforce requested and effective mapped gates;
- verify disabled model cannot reach an upstream mock from any protocol.

**Exit:** all entry-point matrix tests pass; cache propagation SLO and fail-closed behavior are verified in multi-instance tests.

### Phase 7 — admin UI and operator workflow

- release catalog list/detail/history/sync/lifecycle UI;
- validate optimistic concurrency, reason/audit, propagation status, and rollback publication;
- write operator runbook.

### Phase 8 — remove parallel authorities

Only after a stable release window:

- remove direct runtime reads from remote/local JSON maps;
- remove hard-coded runtime pricing fallbacks after they are represented as imported/managed data;
- remove model capability/list heuristics replaced by catalog fields;
- keep import adapters and immutable history;
- remove legacy shadow mode last.

---

## 19. Rollback strategy

### 19.1 Before admission enforcement

Behavioral flags may return list/pricing readers to legacy while DB shadow state remains. No schema deletion is required.

### 19.2 After admission enforcement

A rollback must **never bypass DB operator policy**.

Allowed rollback patterns:

- roll base pricing/list projection back to a prior validated catalog publication using a new forward epoch;
- temporarily use a legacy price reader only after the same DB lifecycle gate has approved requested and effective models;
- revert a faulty adapter while keeping a defense-in-depth scheduler gate active;
- disable new catalog publication while retaining last-known-good DB authority.

Forbidden rollback:

- re-enable direct JSON/hard-coded admission without DB policy;
- delete catalog tables/history;
- decrement publication epoch;
- mutate an applied migration;
- overwrite an old immutable revision;
- clear Redis as the sole lifecycle action.

### 19.3 Bad publication

Publish a new audited epoch pointing to the prior validated immutable content. The higher epoch fences stale writers from restoring the bad projection.

---

## 20. Test plan

### 20.1 Migration/repository tests

Add coverage for:

- fresh migration and idempotent startup migration runner;
- Ent schema/SQL parity and `make generate` idempotence;
- immutable validated revisions;
- concurrent publish lock and optimistic predecessor conflict;
- publication + lifecycle audit + outbox atomic commit/rollback;
- transaction context reuse;
- alias uniqueness/target constraints;
- operator state surviving sync;
- forward rollback publication with monotonic epoch.

Recommended paths:

- `backend/internal/repository/catalog_repo_integration_test.go`
- `backend/internal/repository/catalog_outbox_repo_test.go`

### 20.2 Sync tests

- identical source produces stable normalized hash;
- malformed JSON/invalid price/duplicate alias rejects candidate;
- fetch failure preserves active publication;
- destructive count/provider deletion is quarantined;
- valid single-model removal becomes source-missing;
- new valid model defaults enabled/active;
- disabled model remains disabled after price change, disappearance, and reappearance;
- retired model never silently reopens;
- fallback-only/custom/mapping-derived models are represented;
- concurrent manual/scheduled sync has one publisher.

Recommended path: `backend/internal/service/catalog_sync_service_test.go`.

### 20.3 Projection/HA tests

- outbox dedup and ordered watermark;
- duplicate/out-of-order events are safe;
- event loss recovers from DB pointer/full rebuild;
- delayed epoch N rebuild cannot overwrite N+1;
- Redis loss rebuilds from DB;
- strict freshness failure rejects new decisions;
- multi-instance disable propagation meets SLO;
- old process projection cannot resurrect retired state.

Recommended path: `backend/internal/service/catalog_projection_service_test.go` plus integration tests modeled on scheduler snapshot tests.

### 20.4 Catalog parity tests

Fixture-driven old-vs-new comparisons must include:

- remote LiteLLM models;
- local/billing fallback-only models;
- custom group/channel models;
- gateway/default inferred models;
- provider catalog capabilities;
- aliases and dated/versioned names;
- token/cache/priority/long-context/image pricing;
- search/filter/facet/total/pagination;
- source removal and restore.

Recommended path: `backend/internal/service/catalog_shadow_parity_integration_test.go`.

### 20.5 Billing parity tests

For each pricing mode and protocol, compare old and DB results for:

- selected model/canonical/mapping chain;
- base and channel price source;
- all unit prices;
- token/cache/image component costs;
- service-tier and long-context adjustments;
- group/account multipliers;
- `TotalCost`;
- `ActualCost`;
- final `users.balance` delta;
- usage revision/snapshot fields;
- batch hold and settlement consistency.

Exact float tolerance must reflect current persisted decimal precision; do not bless material money deltas as rounding noise.

### 20.6 Admission/bypass tests

For every protocol surface:

- active model succeeds when route and price exist;
- unknown returns protocol-shaped 404;
- disabled returns 503 and upstream mock has zero calls;
- retired returns 404 and zero upstream calls;
- source-missing/invalid returns 503;
- channel override cannot bypass disabled status;
- group custom list cannot bypass status;
- active requested alias mapping to disabled effective target fails;
- disabled requested alias mapping to active target fails;
- account/channel mapping cannot bypass effective target gate;
- no route remains the existing scheduling error, distinct from lifecycle error;
- accepted request settles from old pinned price after new publication;
- queued batch retry preserves original decision;
- new retry observes new state.

### 20.7 Hot-path cache and load tests

The operational catalog is not complete until tests prove that DB authority did not become DB coupling on every request:

- unit tests wire DB/Redis repositories that panic on access, then verify active/disabled/unknown/pricing decisions complete from a preloaded snapshot without touching them;
- instrumentation asserts **zero catalog SQL queries and zero catalog Redis commands per user admission** in steady state;
- `BenchmarkCatalogResolveRequested`, mapped-model resolution, and base-price resolution measure `ns/op`, `allocs/op`, and bytes/op for exact, alias, normalized, and family/legacy names;
- benchmarks verify lookup cost does not grow linearly with catalog size because lookup candidates are precomputed at snapshot build time;
- concurrent tests run request readers while repeatedly building/swapping snapshots under `go test -race`; no partial map, mixed epoch, deadlock, or data race is allowed;
- a request pins one `CatalogReadView`, then a concurrent epoch swap occurs; requested/effective gates and pricing must all remain on the pinned epoch;
- load tests compare baseline main vs catalog branch at agreed concurrency and request mix; p50/p95/p99 admission latency, throughput, CPU, GC, allocations, DB pool utilization, and Redis commands are release gates;
- the numeric latency/CPU regression budget is set from the measured baseline before enforce-mode rollout rather than invented in the TDD, but query-count budget is fixed at zero catalog DB/Redis operations per normal request;
- with PostgreSQL and Redis paused after a snapshot is loaded, traffic within `max_stale` continues from memory without a connection storm;
- beyond `max_stale`, new admissions fail quickly from local freshness state with 503 and do not synchronously retry DB per request;
- refresh storms are coalesced; background rebuild concurrency remains bounded and cannot exhaust the request DB pool;
- snapshot size, build duration, allocation rate, retained old-snapshot count, and GC impact are measured with realistic current and projected catalog fixtures.

Recommended paths:

- `backend/internal/service/model_catalog_reader_test.go`
- `backend/internal/service/model_catalog_benchmark_test.go`
- `backend/internal/service/catalog_projection_concurrency_test.go`
- a repository load-test scenario that exports request and dependency query counters.

### 20.8 Frontend tests

- admin server pagination and filters;
- separate operator/source/effective badges;
- disable/enable/retire reason dialogs;
- optimistic conflict refresh;
- sync preview and quarantine;
- lifecycle history and publication rollback;
- i18n key parity and no hard-coded accessibility labels;
- public marketplace excludes non-active rows with correct server totals.

---

## 21. Observability and operations

Metrics:

- `catalog_active_epoch` by DB/Redis/instance;
- `catalog_projection_lag_seconds` and local snapshot age;
- `catalog_snapshot_build_duration_seconds`, model count, bytes, allocation/GC impact, and retained generations;
- `catalog_refresh_queue_depth`, coalesced revisions, in-flight rebuilds, failures, and retries;
- `catalog_hot_path_resolve_duration_seconds` and admission outcome;
- `catalog_hot_path_dependency_io_total` for catalog SQL/Redis access from request context; expected steady-state value is zero and any increment is an implementation violation;
- `catalog_outbox_lag_events` and age;
- sync duration/status/source and diff counts;
- candidate quarantine/rejection reasons;
- lifecycle changes by action;
- admission decisions/rejections by internal reason and protocol;
- pricing source/version usage;
- shadow parity mismatches by field/source/model;
- stale projection fail-closed count;
- unpriced decision prevention count.

Structured logs include request/correlation ID, catalog epoch, model revision IDs, canonical requested/effective model, internal lifecycle reason, pricing source, and sync/publication IDs. Do not log credentials or complete sensitive request bodies.

Alerts:

- no valid publication at startup;
- local/Redis epoch behind DB beyond SLO;
- outbox worker/rebuild repeatedly failing;
- source sync stale beyond expected interval;
- destructive candidate quarantined;
- billing parity mismatch above zero unexplained threshold;
- any upstream attempt without catalog decision in enforce mode.

Runbook must cover sync failure, quarantine review, disable propagation verification, bad-publication forward rollback, Redis rebuild, DB outage, and legacy pricing fallback under DB lifecycle gate.

---

## 22. Security and integrity

- Admin catalog mutations require existing admin authorization.
- Disable/retire/rollback/destructive publish requires nonempty reason.
- Use optimistic concurrency to prevent lost admin updates.
- Validate all source payloads before persistence/publication.
- Use parameterized repository queries and constrained enums/checks.
- Lifecycle audit and outbox are atomic with mutation.
- Avoid logging secrets, source credentials, account credentials, or request payloads.
- Public APIs expose only policy-safe model information, not operator notes or source errors.
- Unknown/retired public error behavior avoids unnecessary internal disclosure.

---

## 23. Key risks and mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Billing delta after source switch | Critical | Full-field shadow parity through balance delta; immutable price pinning |
| Channel override bypasses disable | Critical | Catalog gate before all pricing precedence |
| Mapping target bypasses disable | Critical | Gate requested and effective mapped canonical models |
| Incomplete source mass-disables catalog | Critical | Stage/validate, count/provider deletion guard, quarantine |
| Legacy rollback resurrects disabled model | Critical | DB lifecycle gate remains mandatory after enforcement |
| Cache misses lifecycle event | High | Durable catalog outbox, epoch polling, full rebuild, fencing |
| Disable races in-flight settlement | High | New-admission semantics plus pinned revision/snapshot |
| Existing custom model absent from LiteLLM | High | Import union of fallback/channel/group/mapping/default sources |
| Fuzzy matcher accepts unknown family | High | Explicit persisted aliases; strict unknown fail-closed |
| Mutable policy/history loses audit | High | Stable policy + immutable revisions + transactional lifecycle audit |
| Provider catalog heuristics drift | Medium | Migrate model capabilities to effective projection with parity |
| DB/cache outage causes broad 503 | Medium | Last-known-good local snapshot within strict freshness window; background-only rebuild/retry; no per-request DB fallback; readiness/alerts |
| Revision columns enlarge usage storage | Medium | Nullable rollout, retention/index review, normalized compact snapshot |

---

## 24. Acceptance criteria

The operational catalog is complete only when all are true:

1. PostgreSQL publication is the authority for canonical model, effective state, capabilities, and base price.
2. New valid imported models default enabled and appear without admin allowlisting.
3. Disabled models are absent from public lists and rejected before upstream across every supported protocol.
4. Retired models cannot reactivate through sync.
5. Source-missing/invalid models cannot receive new traffic after a valid publication.
6. Channel pricing, group lists, aliases, and mappings cannot bypass lifecycle policy.
7. All legacy/hard-coded runtime fallback models are represented in the catalog before their direct readers are removed.
8. Billing parity covers all pricing fields and final balance deduction with zero unexplained material delta.
9. Accepted requests and async jobs settle from pinned immutable revision/snapshot.
10. Publication, lifecycle audit, and outbox event are atomic.
11. The normal request path resolves lifecycle, alias/canonical model, and base pricing entirely from one pinned immutable process-local snapshot, with zero catalog SQL queries and zero catalog Redis commands.
12. Requested-model gate, effective-mapped-model gate, and base-price resolution for one admission use the same `CatalogReadView` epoch.
13. Snapshot refresh/rebuild is background-only, bounded, coalesced, atomically swapped, and cannot block request goroutines or exhaust the request DB pool.
14. HA caches recover from loss/out-of-order events and enforce monotonic epoch fencing.
15. Benchmarks/load tests establish an approved latency/CPU/allocation budget against current main and prove no request-path dependency-I/O regression.
16. Marketplace facets/totals/pagination are calculated after effective filtering.
17. Provider/model discovery endpoints filter through the same effective projection.
18. Rollback after enforcement cannot bypass DB operator policy.
19. Ent generation and all repository/service/frontend/security gates pass.
20. Staging verifies disable propagation, DB/Redis outage while serving a valid snapshot, billing parity, sync failure, bad-publication rollback, and production runbook.

---

## 25. Recommended implementation paths

Expected new files, subject to detailed implementation planning:

```text
backend/migrations/<next>_catalog_authority.sql
backend/migrations/<next>_catalog_indexes_notx.sql
backend/ent/schema/catalog_model.go
backend/ent/schema/catalog_sync_run.go
backend/ent/schema/catalog_revision.go
backend/ent/schema/catalog_model_revision.go
backend/ent/schema/catalog_model_alias.go
backend/ent/schema/catalog_publication.go
backend/ent/schema/catalog_lifecycle_audit.go
backend/ent/schema/catalog_outbox.go
backend/internal/repository/catalog_repo.go
backend/internal/repository/catalog_outbox_repo.go
backend/internal/service/catalog_sync_service.go
backend/internal/service/catalog_projection_service.go
backend/internal/service/model_catalog_service.go
backend/internal/handler/admin/model_catalog_handler.go
backend/internal/server/routes/admin.go
frontend/src/api/admin/modelCatalog.ts
frontend/src/views/admin/ModelCatalogView.vue
frontend/src/router/index.ts
frontend/src/components/layout/AppSidebar.vue
frontend/src/i18n/locales/*
```

Existing primary integration points:

```text
backend/internal/service/pricing_service.go
backend/internal/service/model_pricing_resolver.go
backend/internal/service/billing_service.go
backend/internal/service/model_marketplace_service.go
backend/internal/service/provider_catalog.go
backend/internal/service/gateway_service.go
backend/internal/service/gateway_scheduling.go
backend/internal/service/openai_gateway_model_availability.go
backend/internal/service/channel_available.go
backend/internal/service/openai_account_scheduler.go
backend/internal/service/batch_image_public.go
backend/internal/service/batch_image_settlement.go
backend/internal/service/account_stats_pricing.go
backend/internal/handler/gateway_handler.go
backend/internal/handler/openai_codex_models_handler.go
backend/internal/handler/gemini_v1beta_handler.go
backend/internal/service/wire.go
backend/internal/repository/wire.go
```

---

## 26. Decisions fixed by this TDD

- DB authority is operational, not display-only.
- Remote/local JSON is an import source, not runtime truth after cutover.
- New valid models default enabled.
- Admin disable means globally unavailable for new decisions and absent from public lists.
- Operator policy and source observation are separate.
- Retirement is terminal unless explicitly and auditably reversed.
- Base pricing is immutable/versioned and pinned for settlement.
- Channel override remains separate but cannot bypass lifecycle.
- Runtime uses one projection service, not direct SQL scattered across consumers.
- Outbox/epoch fencing is required for HA invalidation.
- Rollback cannot restore a policy-blind JSON admission path.

## 27. Approval boundary

This TDD is a design artifact only. Approval of the direction does not authorize production migration or deployment.

The next implementation action after explicit approval is **Phase 0/Phase 1 only**: produce the complete source/consumer inventory and an additive schema/repository plan, then implement migrations and repositories without changing production pricing, request admission, or public behavior.
