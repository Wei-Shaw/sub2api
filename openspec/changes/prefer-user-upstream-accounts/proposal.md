# Prefer the caller’s upstream accounts in a shared OpenAI group

## Problem

Operators may put accounts contributed by several users and a shared backup account in one OpenAI group. Today account priority applies to every caller, while session affinity only takes effect after a session has an account. Giving each user a separate group duplicates group configuration, changes key assignment, and makes the shared pool harder to operate.

For example, Alice contributes account A, Bob contributes B, and C is the shared backup. With this proposal, a fresh eligible request from Alice tries A first and a fresh request from Bob tries B first. If A is rate limited or has no immediately available capacity, Alice’s request can use the existing scheduler in the same group. This is a preference: other users can still use A through the shared pool, and the feature does not reserve its quota for Alice.

## Configuration

- Add an optional **Preferred user** field to the admin edit form for OpenAI upstream accounts. Store a nullable `preferred_user_id` referencing the platform user ID, with `ON DELETE SET NULL`. One user can be preferred by multiple accounts. Existing accounts default to no preference. The admin can clear or reassign the field.
- Add a default-off **Prefer user’s accounts** setting to standard-mode OpenAI groups. An account’s preference takes effect only when the account is eligible in the request’s group and the group opts in. A preference grants no group access and exposes no upstream credentials.
- Derive the caller from the authenticated platform API key owner. All of that user’s keys in opted-in groups share the same preference. Ignore caller-provided identity headers and keep the preferred user field out of public account and model responses.

## Scheduling contract

1. Preserve existing account-bound continuations and session affinity. A valid existing session on B stays on B even if Alice’s A has become available. An account-bound continuation keeps the existing behavior if its account is unavailable.
2. Before ordinary fresh selection, use the existing eligibility checks to find candidates in the current group for the request’s model, transport and capabilities. Preserve exclusions, channel/model restrictions, rate limits, quota gates, pricing gates and the final concurrency admission. Only matching preferred accounts move to the first tier. Rank within that tier using the current policy for that scheduler.
3. Attempt admission in the preferred tier without waiting merely because the account is preferred. If that tier has no immediately admissible candidate, continue with the existing pool selection and wait behavior. If forwarding fails before the existing replay boundary, reuse normal failover and exclusions so a failed preferred account is not repeatedly selected. Once output starts, preserve existing streaming behavior.
4. Keep the selected account in the existing session affinity store. New sessions may try the preferred account again. Requests without a matching preference, and groups with the setting off, keep their current ordering and billing.

## Implementation and verification

- Add the account and group fields to the existing admin update and readback paths, scheduler snapshot and group authentication cache. Invalidate snapshots on account reassignment and cached group settings on toggle changes. Verify deletion cleanup cannot leave a stale preference active.
- Carry the authenticated caller ID through the OpenAI scheduler request and apply the same tier semantics in legacy and advanced scheduling. Start with HTTP and WebSocket Responses, including Codex. Leave model listing, other providers and unrelated endpoints on their existing policies. Reuse admission checks instead of adding a separate path that can bypass them.
- Verify A/B preference in one group, multiple keys owned by one user, opt-out/default behavior, no cross-group or model-policy bypass, immediate spillover on exhausted capacity, rate-limit failover, and session continuity after spillover. Cover both scheduling modes and Responses transports. Verify admin-only configuration and updates after cache invalidation.

Hard reservations, per-user quotas on an upstream account and self-service account import are outside this proposal. Existing billing remains independent of the upstream account preference.
