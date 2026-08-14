# Authentication

Every gateway request needs an API key. The gateway accepts three headers, in
this order of preference, so the client you already have works unmodified.

## Accepted headers

### `Authorization: Bearer` — OpenAI-style

```bash
curl {{SITE_ORIGIN}}/v1/chat/completions \
  -H "Authorization: Bearer sk-..."
```

The scheme is matched case-insensitively, so `bearer` also works.

### `x-api-key` — Anthropic-style

```bash
curl {{SITE_ORIGIN}}/v1/messages \
  -H "x-api-key: sk-..."
```

### `x-goog-api-key` — Gemini CLI compatibility

```bash
curl "{{SITE_ORIGIN}}/v1beta/models/gemini-2.5-pro:generateContent" \
  -H "x-goog-api-key: sk-..."
```

Send one. If several are present, `Authorization` wins, then `x-api-key`, then
`x-goog-api-key`.

## The `api_key` query parameter is rejected

Passing the key in the URL is refused outright, with `400`:

```json
{
  "code": "api_key_in_query_deprecated",
  "message": "API key in query parameter is deprecated. Please use Authorization header instead."
}
```

This is deliberate. A key in a query string ends up in browser history, proxy
logs, and referrer headers. Move it to a header.

## Missing key

With no recognised header at all, the gateway answers `401`:

```json
{
  "code": "API_KEY_REQUIRED",
  "message": "API key is required in Authorization header (Bearer scheme), x-api-key header, or x-goog-api-key header"
}
```

## Group assignment

Authentication proves who you are. It does not by itself grant access: the key
must also be assigned to a group. An unassigned key is refused with `403` and a
message in the protocol's own error shape:

```json
{
  "type": "error",
  "error": {
    "type": "permission_error",
    "message": "API Key is not assigned to any group and cannot be used. Please contact the administrator to assign it to a group."
  }
}
```

Operators can allow ungrouped keys as a system setting, in which case this check
does not fire. If you hit it, the fix is on the operator's side, not in your
code.

## Handling keys safely

- Keep keys in environment variables or a secret manager, never in a repository
  or a frontend bundle. A key in shipped client code is a key that is already
  public.
- Use one key per application, so revoking one does not take down the rest.
- Rotate by creating the new key first, deploying it, then deleting the old one.
- Delete a key the moment you suspect it leaked. Deletion takes effect for new
  requests immediately.
- The dashboard shows a key's full value only at creation time.
