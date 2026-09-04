# ADR-004: Manage Codex profiles as templates and key slots by surface

## Status

Accepted

## Date

2026-08-31

## Context

ADR-001 introduced account-owned Codex OS Profile policies. The first model
stores one `canonical_surface` per OS and keys device bindings by
`(account, API key, OS)`. That makes Desktop and CLI mutually exclusive within
Windows, macOS and Linux, even though they are distinct client surfaces with
independent architecture, proxy and slot requirements.

Editing the complete policy inside every account also makes repeated rollout
slow and inconsistent. Administrators need named reusable configurations in
System Settings, while account editors should only decide whether the feature
is enabled and which configuration is assigned.

## Decision

- Add named Codex identity templates under System Settings. A template owns
  the profile variants, session policy, affinity TTL and proxy overrides.
- Treat a template as the configuration authority. An account stores an
  enabled/disabled assignment and a template reference; its existing policy,
  profile and slot rows are a runtime projection used for scheduling,
  affinity, epoch rotation and draining.
- Require optimistic concurrency for template updates with an expected
  revision. A runtime-material update increments the template revision.
- Before applying an update referenced by accounts, show the affected account
  count and require an explicit administrator confirmation. The update applies
  to every assigned account through the same provisioning transition. A
  failure leaves the template and all account projections unchanged.
- Identify a profile variant by `(os_class, canonical_surface)`, not OS alone.
  Windows, macOS and Linux may each enable Desktop and CLI simultaneously.
  Generic may enable SDK and third-party surfaces simultaneously.
- Key device bindings, profile epochs, affinity and WebSocket ownership by the
  same OS/surface identity. A change to one surface must not rotate, drain or
  rebind its sibling surface.
- Keep account-level proxy as the final fallback. Template profile and slot
  routes retain the precedence `slot -> profile -> account -> direct`.
- Let every template slot choose a Codex client version mode. `inherit` follows
  the deployment-wide version resolved as `administrator override -> synced
  stable version -> built-in fallback`; `pinned` stores one explicitly
  validated version and requires at least `0.144.0`.
- Use the slot's effective client version as one source for the User-Agent
  version segment, the `version` header and structured client/turn metadata.
  It does not select a model and does not change the Desktop application build.
- Treat a slot version change as runtime material: advance only the affected
  profile epoch, create replacement slots and drain the old epoch. Keep
  `catalog_version` separate; it versions the closed identity fixture format,
  not the Codex client release.
- A deployment-wide version refresh is deliberately different from a slot
  configuration change. Slots in `inherit` mode pick up the newly validated
  global version on the next attempt, while an already running attempt keeps
  the version captured in its plan; the device identity and epoch are retained
  just as they are across a normal first-party client upgrade. No account-wide
  epoch fan-out or drain is performed for that global refresh.
- Define `device_shared` capacity as one in-progress HTTP/SSE request stream or
  one WebSocket session per physical device slot. The lease is released when
  that request/session ends. This is not an account-wide concurrency limit, and
  the other session policies add no equivalent slot mutex.
- Define `sessions_per_device` as the number of reusable upstream session
  identities in `session_pool`; it is not a request-concurrency setting.
- Keep usage parsing, billing, scheduler ownership and durable A-to-B usage
  relay behavior in the host. The Transport plugin continues to receive only
  the already resolved proxy URL and an opaque API-key connection scope.

## API Contract

Templates are resources under the admin settings namespace:

- `GET /api/v1/admin/settings/codex-identity-templates`
- `POST /api/v1/admin/settings/codex-identity-templates`
- `GET /api/v1/admin/settings/codex-identity-templates/:id`
- `PUT /api/v1/admin/settings/codex-identity-templates/:id`
- `DELETE /api/v1/admin/settings/codex-identity-templates/:id`

An account write uses an assignment object rather than an editable policy
snapshot. Existing policy output remains available as the effective runtime
projection during migration.

The account device-slot lifecycle response exposes `client_version_mode`, the
optional pinned `client_version`, and `effective_client_version` so operators
can verify the version that will actually be declared upstream.

## Alternatives Considered

### Keep account-owned policies and add a copy-template button

Rejected. A copied template stops being centrally controlled immediately and
does not satisfy the settings-level control requirement.

### Store Desktop and CLI flags inside one OS profile

Rejected. The two surfaces can require different architecture, slot count and
proxy routes. A list of flags would recreate nested per-surface conditionals
and ambiguous binding ownership.

### Change only the account editor from radio buttons to checkboxes

Rejected. Database uniqueness, repository lookups, affinity keys and
WebSocket ownership are all OS-only. A frontend-only change would silently
cross-bind Desktop and CLI traffic.

### Let template edits silently update assigned accounts

Rejected. A central edit can rotate live slots and change egress routes. The
UI must disclose the affected account count and require explicit confirmation.

## Consequences

- The schema adds template, template-profile and template-slot resources plus
  account assignment metadata. A later additive migration adds the two client
  version fields to template slots and materialized account slots.
- Existing profile and binding uniqueness moves to OS plus surface. Affinity
  and WebSocket namespaces must advance so old OS-only keys expire naturally.
- Template runtime edits can touch multiple accounts and therefore require a
  bounded transaction, deterministic lock order and scheduler outbox events.
- Deleting a referenced template is rejected until its accounts are disabled
  or moved to another template.
- Once an account enables two surfaces for one OS, an older binary cannot read
  that policy. Application rollback must either retain the new binary or first
  prove that no dual-surface assignment exists and perform an explicit data
  downgrade. Image rollback alone is not sufficient.
- `deploy/rollback-codex-profile-templates-v1.sql` is the explicit downgrade
  path for an old binary. It preserves account, credential, proxy and billing
  data but disables all Codex Profile assignments before restoring OS-only
  constraints.
- The lightweight account-data export records assignment ID and applied
  revision so a same-database restore cannot silently detach from a template.
  Moving templates between deployments requires the database backup path; a
  mismatched or missing template fails import instead of binding by accident.
