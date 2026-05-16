# Changelog

## Unreleased - 2026-05-16

### Added

- Added API key regeneration for user-owned keys, including backend route support and frontend confirmation/copy flow.
- Added automatic default API key creation during user registration.
- Added automatic entitlement resolution for unbound API keys, preferring active subscriptions and falling back to the default standard group when available.
- Added a dry-run capable `migrate-key-routing` command for creating default keys and clearing legacy key group bindings.
- Added a database migration to switch default 100 USD monthly subscription grants to weekly grants.

### Changed

- Updated the user key management page so users create and manage general-purpose keys without choosing a fixed group.
- Updated OpenAI and Google-compatible API key auth middleware to resolve subscription or balance routing at request time.
- Redesigned the payment page around account status and subscription package entry points.
- Updated the public homepage copy and calls to action to guide users through sign-up or sign-in before console usage.
- Refreshed Agents Hub search and filter controls for a clearer responsive layout.
- Documented Oceanway deployment and compose environment updates.

### Fixed

- Preserved cache invalidation and key uniqueness checks when rotating API key secrets.
- Kept future default grant settings aligned with shortened weekly subscription validity.
