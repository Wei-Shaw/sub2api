# ADR-001: Separate Codex OS Profile identity from tenant runtime state

## Status

Accepted

## Date

2026-08-24

## Context

Shared OpenAI OAuth accounts need stable, bounded client identities across Windows, macOS, Linux and generic SDK traffic. Existing account-level convergence over-couples tenants, while per-API-Key convergence can expose unbounded device identities. PR #2 already provides tenant-scoped HTTP/WS/state isolation and must remain authoritative for runtime state.

Account creation and import currently expose an `active + schedulable` account before groups and post-create configuration finish. The new identity policy makes that race unacceptable.

## Decision

- Introduce an additive, default-off OS Profile device-pool mode.
- Keep device identity, session identity, account affinity and transport/state isolation as separate layers.
- Persist Profile/slot/binding lifecycle in typed relational entities, not untyped account extra.
- Make AccountProvisioningSpec the only account write contract and atomically activate accounts after complete configuration.
- Use structured HTTP/SSE/WS identity transformation and response restoration.
- Preserve PR #2 API Key isolation even when device identities are shared.
- Preserve legacy eligibility for default-off accounts in mixed pools; strict unsupported-device enforcement requires a fully Profile-managed candidate pool, and established Profile affinity never downgrades to legacy.
- Deliver as a stacked Draft PR whose base is the PR #2 branch.

## Alternatives Considered

### Account-wide device and session convergence

Rejected because all tenants share one upstream session/state identity and historical deployments reported quota regressions.

### Per-API-Key device identity only

Rejected as the sole new mode because device identity cardinality grows with tenant Key count.

### CPA-style pseudonymization only

Rejected as incomplete because it preserves device cardinality and does not establish HTTP/WS tenant isolation.

### Header-only OS masquerading

Rejected because UA, originator, body metadata, workspace, prompt cache and response protocol become inconsistent.

## Consequences

- Account import becomes a provisioning transaction rather than a thin Create call.
- Profile adapters and response restoration require broad protocol tests.
- Slot and proxy changes need epoch/draining semantics.
- Binding resolution adds a database transaction on Profile-managed attempts; canary testing must measure query/write load before production rollout.
- Profile/session/proxy handshake compatibility can reduce WS connection reuse, while the PR #2 account-wide connection budget remains the hard cap.
- The feature adds schema and maintenance cost but remains opt-in and isolated from existing account behavior.
