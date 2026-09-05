# Image Generation Job Contract Design

## Goal

Stop sending or persisting the selected API key secret in image-generation job payloads. Use the authenticated user's numeric API key ID instead, and keep asynchronous job responses compact until generation succeeds.

## Request Contract

`POST /plugins/image-generation/api/generate` replaces `provider_api_key` with `api_key_id`:

```json
{
  "prompt": "draw a cat",
  "api_key_id": 42,
  "model": "gpt-image-1",
  "size": "1024x1024",
  "response_format": "b64_json",
  "output_count": 1,
  "reference_images": []
}
```

The plugin frontend already loads the authenticated user's API keys and sends the selected key ID. It no longer reads the key secret when constructing a generation request. Metadata that is already represented by top-level fields, including `inputs.api_key_id`, is not duplicated.

The plugin service resolves `api_key_id` through the main service using the current request's authenticated user credentials. Resolution must verify that the key exists, belongs to the current user, is active, and is allowed to generate images. A missing, inaccessible, disabled, or incompatible key is rejected before a job is created.

The resolved secret is transient. It may be held in memory while an upstream request is executing, but it must not be written to plugin history or returned by a plugin API.

## Persisted Job Data

The history request stores the normalized generation parameters and `api_key_id`. It does not store `provider_api_key`.

Local asynchronous tasks receive the resolved secret in memory when they start. Batch tasks persist the upstream batch ID and selected API key ID. Later status, retry, and cancel operations resolve the current secret again from the stored ID using the authenticated caller. This makes key deletion, disabling, ownership changes, and rotation take effect on subsequent operations.

Existing history rows that contain `provider_api_key` remain readable. Responses continue to sanitize that field. Compatibility code may use the legacy secret only for an already-running legacy batch job when no `api_key_id` is available; newly created or retried jobs must use an ID and must never persist a new secret.

## Job Response Contract

Generation submission and retry return a compact envelope:

```json
{
  "job_id": "history-id",
  "status": "pending"
}
```

Status polling continues to identify the job only through the URL:

```text
GET /plugins/image-generation/api/history/{job_id}/status
```

Pending status responses contain only `job_id` and `status`. They do not repeat the prompt, model, references, selected key, internal batch state, or other stored request fields.

Successful status responses add `result`:

```json
{
  "job_id": "history-id",
  "status": "succeeded",
  "result": {
    "images": []
  }
}
```

Failed or canceled responses omit `result` and may include `error_message`. Submission never returns partial provider or batch metadata as a result. History and conversation APIs retain their existing rich record shape because they serve history reconstruction rather than job polling, while continuing to exclude secrets.

## Data Flow

1. The frontend sends the selected `api_key_id` and generation parameters once.
2. The plugin handler resolves and validates the key with the main service under the current user identity.
3. The generation service creates history containing `api_key_id`, starts the local or batch task, and returns only the job envelope.
4. The frontend polls by `job_id` only.
5. The plugin service returns a compact pending envelope until the job reaches a terminal state.
6. On success, the service archives generated media and returns the final result. On failure or cancellation, it returns the terminal status and error message without request data.

## Error Handling

- Invalid or inaccessible `api_key_id`: reject generation without creating history.
- Key resolution failure during batch polling, retry, or cancel: return an error without changing a pending job to succeeded.
- Upstream submission failure: mark the created history record failed and return the upstream error through the existing handler mapping.
- Terminal status polling is idempotent and does not resolve the key again when the persisted result or error is already final.
- A successful response is emitted only after result images have been fetched and archived successfully.

## Testing

Backend tests cover:

- generation accepts `api_key_id` and rejects a missing or invalid ID;
- key ownership, active status, and image-generation permission are enforced;
- newly persisted history contains `api_key_id` and no key secret;
- local and batch upstream requests receive the resolved secret;
- batch status, retry, and cancel re-resolve by ID;
- pending submission and polling responses omit request and partial result data;
- succeeded polling includes the final result;
- failed and canceled polling include no result and expose only the allowed error field;
- legacy pending batch history can still finish without exposing its stored secret.

Frontend tests cover:

- generation sends `api_key_id` and never sends `provider_api_key`;
- pending responses start polling with only `job_id`;
- polling continues to send only `job_id` in the path;
- final successful results render normally;
- failed and canceled terminal responses render their error state.

## Out Of Scope

- Moving image-generation job ownership into the main service.
- Encrypting or duplicating API key secrets in plugin storage.
- Changing the rich history and conversation response contracts beyond secret sanitization.
- Changing generated image persistence, conversation grouping, or user key preference behavior.
