# Image Generation Recent Key Sync Design

## Goal

Remember the API key most recently selected in the image-generation plugin across browsers and devices. If that key is no longer available, select the first available image-generation key instead.

## Persistence

Add a nullable `last_image_api_key_id` column to the `users` table. The value records only the selected API key ID and does not duplicate or expose key material.

The column does not need a foreign-key relationship. API key deletion may leave a stale ID temporarily; the read and frontend reconciliation paths treat a missing or unavailable key as invalid and replace it with the first available key. This keeps API key deletion independent and makes the fallback behavior explicit.

## API

Add authenticated current-user endpoints dedicated to image-generation preferences:

- `GET /api/v1/user/preferences/image-generation`
  - Returns `{ "last_api_key_id": number | null }`.
- `PUT /api/v1/user/preferences/image-generation`
  - Accepts `{ "last_api_key_id": number | null }`.
  - When the value is non-null, verifies that the API key belongs to the authenticated user.
  - Rejects IDs owned by another user or IDs that do not exist.
  - Returns the persisted preference.

The service stores selection state only. The frontend remains responsible for determining whether a key is currently enabled and supports image generation because it already filters the available key list using those rules.

## Frontend Flow

On plugin initialization, load the available API keys and the server preference in parallel. After the available keys are filtered for image-generation support:

1. Select the saved key when its ID exists in the filtered list.
2. Otherwise select the first filtered key.
3. If the fallback differs from the saved preference, persist the fallback ID. Persist `null` when no usable key exists.

When the user changes the key selector, update local reactive state immediately and send the new ID to the preference endpoint. A failed update does not interrupt the current session, but it is surfaced through the composable error state so the failure is observable. A later initialization will use the last successfully persisted value.

Concurrent selections from multiple terminals use last-write-wins semantics.

## Compatibility

Existing users start with a null preference and therefore keep today's first-key behavior until they select a key. The database migration is additive and nullable.

The plugin API client obtains authentication in the same way as the existing key-list request. No API key secret is written to browser storage or the preference endpoint.

## Testing

Backend tests cover:

- reading a null and populated preference;
- saving a key owned by the current user;
- rejecting a missing key and another user's key;
- clearing the preference;
- migration/schema behavior for existing users.

Frontend tests cover:

- restoring a saved usable key instead of the first key;
- falling back to the first key when the saved key was deleted or filtered out;
- persisting the fallback, including null for an empty list;
- persisting a user selection;
- retaining the local selection and exposing an error when persistence fails.

## Out Of Scope

- Remembering separate keys per conversation or per model.
- Maintaining a history of selected keys.
- Resolving simultaneous writes beyond last-write-wins.
- Adding a general-purpose user preference framework.
