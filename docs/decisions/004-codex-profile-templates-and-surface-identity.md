# ADR-004: Manage Codex profiles as templates and key slots by surface

## Status

Proposed

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
- Make `device_shared.max_active_conversations_per_slot` configurable from 0
  to 1000. Zero adds no local slot cap; positive values count simultaneous
  HTTP/SSE request streams or WebSocket sessions. Existing explicit limits,
  including 1, remain unchanged. Other session policies add no slot cap.
- Prefer the API key's stable device slot. If its local capacity is full, a
  new conversation with a stable key may use another active slot of the same
  OS, surface, architecture and profile epoch. Persist that conversation's
  binding without moving other conversations. A known conversation stays on
  its bound slot; missing identity or an unbound previous_response_id never
  authorizes a move. No upstream request has been sent during this selection.
- Renew and release each request lease independently. Keep its database
  affinity alive during long requests and detached upstream draining.
- Classify explicit quota, concurrency and rate-limit errors separately; a
  bare 429 remains unknown. Quota errors do not enter same-account retries.
  An upstream 429 never triggers device switching. Existing account, user
  and API-key concurrency settings continue to apply.
- Provide a version/device-keyed `CodexClientProfileProvider` extension point
  with source, evidence and verification status. The default catalog remains
  explicitly unverified. This does not extract release fingerprints, install
  Codex, or configure TLS/HTTP2 fingerprints. Future providers must be wired
  into both gateway resolution and admin status before deployment.
- Define `sessions_per_device` as the number of reusable upstream session
  identities in `session_pool`; it is not a request-concurrency setting.
- Keep existing usage parsing and billing unchanged. Lease lifetime includes
  upstream draining so slot capacity is not released before a request ends.

The feature adds account provisioning, template and device-binding schema.
The normal migration runner applies these changes before the new core starts.
Existing official accounts are backfilled as active with the feature disabled.
Earlier experimental revisions of this PR are not an official upgrade baseline.

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
can verify the version that will actually be declared upstream. It also exposes
`client_profile_source` and `client_profile_verification`; a version string
alone is not evidence of a verified fingerprint.

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
- Core instances handling profile-managed accounts must understand their
  template assignments and binding lifecycle. Disable the feature before
  rolling back to an official version without device-slot support.
- The lightweight account-data export records assignment ID and applied
  revision so a same-database restore cannot silently detach from a template.
  Moving templates between deployments requires the database backup path; a
  mismatched or missing template fails import instead of binding by accident.
