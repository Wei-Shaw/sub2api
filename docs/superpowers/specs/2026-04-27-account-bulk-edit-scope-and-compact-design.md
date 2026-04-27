# Account Bulk Edit Scope And Compact Design

## Summary

This change expands admin account bulk edit in two directions:

1. Add a second bulk-edit target scope based on the current filter result set, so operators do not need to manually select every account.
2. Align OpenAI bulk-edit fields with single-account create/edit for the compact-related settings that are already supported elsewhere.

The design keeps the existing selected-row workflow intact and adds a unified bulk-edit entry with two explicit actions:

- `Bulk edit selected accounts`
- `Bulk edit current filtered results`

`Current filtered results` reuses the existing account-list filters. That means:

- with no filters, it targets the whole account inventory
- with a group filter, it targets all accounts in that group
- with combined filters, it targets all matching accounts

## Goals

- Preserve the current selected-account bulk edit flow.
- Let operators bulk edit the full current filtered result set without manual row selection.
- Show the user the exact target scope before applying changes.
- Reuse the current list filter semantics instead of inventing a separate "all accounts" or "by group" API.
- Add the missing OpenAI bulk-edit fields:
  - OAuth `codex_cli_only`
  - API key `openai_apikey_responses_websockets_v2_mode`

## Non-Goals

- No new standalone "edit all accounts" route that ignores filters.
- No new dedicated "edit group" route separate from list filters.
- No change to the backend merge semantics for other bulk-edit fields.
- No attempt in this change to refactor all account form components into a shared schema system.

## Current State

### Bulk edit entry

The account list currently exposes bulk edit only through selected-row actions. `AccountsView.vue` passes `selIds`, `selPlatforms`, and `selTypes` into `BulkEditAccountModal.vue`.

### Filter state

The account page already keeps a central `params` object for current filters and reloads the table from that state. Group filtering already exists in `AccountTableFilters.vue`.

### Bulk edit payload

`BulkEditAccountModal.vue` builds a bulk update request around explicit account IDs.

### OpenAI field gap

Single-account create/edit already supports:

- `openai_passthrough`
- OAuth WS mode
- API key WS mode
- OAuth `codex_cli_only`

Bulk edit currently supports:

- `openai_passthrough`
- OAuth WS mode only

That leaves a real capability gap for operators managing large OpenAI account sets.

## User Experience

### Entry point

Use one compact `Bulk edit` dropdown button in the table-level bulk actions area above the grid.

The dropdown contains:

- `Bulk edit selected accounts`
- `Bulk edit current filtered results`

Behavior:

- If there is no row selection, the `selected accounts` action is disabled.
- `Current filtered results` is always available.
- The existing separate immediate `Edit` action in the selected-row bar is replaced by this unified dropdown to avoid duplicate buttons that mean different scopes.

### Modal scope messaging

The bulk edit modal gets a required scope descriptor prop.

For `selected accounts`:

- show the existing count-based info banner
- keep using explicit selected account metadata for platform/type compatibility checks

For `current filtered results`:

- show a banner stating that edits apply to the current filtered result set
- show the matched account count from a preview query
- show a short summary of active filters when practical, especially group/search/platform/type/status filters

### Safety

For filtered-result mode:

- disable submit if the preview count is `0`
- refresh the target count when the modal opens
- keep the final success toast count aligned with the backend result

The modal should not silently fall back from filtered mode to selected mode.

## Backend/API Design

### Request model

Extend bulk update to support two target modes:

- explicit IDs
- filter-based query

The request shape should keep backward compatibility for the selected-ID path while allowing a filter target. The backend handler can accept a payload that contains either:

- `account_ids`
- or `filters`

but not neither.

The `filters` payload should reuse the existing account-list query semantics already used by `/admin/accounts` and `/admin/accounts/data`, including:

- `search`
- `platform`
- `type`
- `status`
- `privacy_mode`
- `group`
- existing sort fields may be ignored for mutation targeting if not needed

### Preview count

The frontend needs an accurate target count before submit in filtered-result mode. The simplest compatible approach is:

- call the existing account list endpoint with the current filters and a minimal page size strategy sufficient to obtain total count

If the current API makes that awkward, add a narrow preview/count helper for bulk edit target resolution. Prefer reusing the existing listing contract first.

### Target resolution

For filtered-result mode, the backend must resolve matching account IDs server-side from the submitted filters rather than trusting only currently loaded page data. This is required so filtered-result mode can act on the full result set across pagination.

### Compatibility metadata

The frontend still needs platform/type compatibility to determine which fields to show. For filtered-result mode, derive this from the preview result set returned from the same query used to show count. If the preview spans mixed incompatible account types, show the same warnings/conditional UI that selected mode already uses.

## Frontend Design

### Accounts view

`AccountsView.vue` will:

- replace the direct selected-only bulk edit trigger with a dropdown action model
- keep a reactive description of the pending bulk edit scope
- pass either selected IDs or current filter params into the modal

The "current filtered results" action uses the live `params` object snapshot at open time, not a mutable live subscription while the modal is already open.

### Bulk edit modal

`BulkEditAccountModal.vue` will accept a richer target contract, for example:

- target mode
- selected IDs or filter snapshot
- preview count
- preview platform/type coverage if needed

The modal remains one form; only the scope banner and submission target differ.

### OpenAI field alignment

Add the missing OpenAI controls to bulk edit:

- OAuth `codex_cli_only`
- API key WS mode selector

Rules:

- OAuth accounts show OAuth WS mode and `codex_cli_only`
- API key accounts show API key WS mode
- mixed OpenAI OAuth/API key selections continue to show only fields that are safe for the entire target set

The payload builder must write:

- `extra.codex_cli_only`
- `extra.openai_apikey_responses_websockets_v2_mode`
- `extra.openai_apikey_responses_websockets_v2_enabled`

with the same enable/disable semantics already used by single-account forms.

## Testing Strategy

### Frontend tests

Add or extend tests for:

- bulk edit dropdown actions in the accounts view
- selected-account mode still calling bulk update by IDs
- filtered-result mode calling bulk update with filter target
- filtered-result mode showing preview count and blocking submit on zero matches
- OAuth bulk edit supporting `codex_cli_only`
- API key bulk edit supporting API key WS mode
- no regression for existing passthrough and OAuth WS mode tests

### Backend tests

Add or extend tests for:

- bulk update request validation for IDs vs filters
- filtered-result mode resolving all matching accounts across pagination semantics
- mixed-channel risk checks still running for filter-target updates if applicable
- backward compatibility for the existing selected-ID request path

## Risks

- Filter semantics can drift if bulk edit reimplements list-filter parsing differently from the listing endpoints.
- Filtered-result mode can surprise users if the active scope is not shown clearly enough.
- Large filtered updates may affect many rows; success/error messaging must stay explicit.

## Recommendation

Implement this as a targeted extension of the existing bulk edit flow:

- unify the entry point in the table action area
- add filter-target bulk update support
- align the missing OpenAI compact-related fields

This keeps the mental model simple and solves the large-account-management pain without introducing a second parallel batch-edit system.
