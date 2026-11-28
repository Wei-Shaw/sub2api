# API Key group and model discovery

`GET /v1/sub2api/key-group` returns the authenticated API key's current group and
the concrete request aliases for its actual upstream models. Authenticate with
the same `Authorization: Bearer <your-sub2api-key>` header used for generation.
The endpoint does not accept another key or group ID as input.

Example response:

```json
{
  "object": "sub2api.key_group",
  "schema_version": 1,
  "key": {"id": 100, "group_id": 42},
  "group": {"id": 42, "name": "assistant", "platform": "openai", "status": "active"},
  "models": [{
    "id": "provider-model",
    "upstream_model": "provider-model",
    "aliases": ["assistant-model", "provider-model"]
  }],
  "revision": "<hash-of-safe-response-fields>"
}
```

Each `models` entry represents an actual mapped target. Several aliases for the
same target are combined into one entry. `id` is a requestable alias: the target's
identity alias when allowed, otherwise a deterministic alias for that target.
Clients may retain any alias in `aliases` they already use. A client that requires
one model must check that the catalogue contains exactly one target, rather than
selecting the first entry from a multi-model group.

Models are read from the group's current schedulable accounts and filtered by
the current group allowlist and account mapping rules. Exact allowlist entries
also provide concrete candidates for unmapped or wildcard-mapped accounts.
Wildcards are not returned as request IDs. An empty catalogue indicates that the
current configuration does not identify a concrete, currently requestable model;
discovery does not substitute a guessed model or a global default list.

The key, owner permissions, IP restrictions and group are read from the database
for each discovery request. Discovery invalidates only this key's prior auth
snapshot so the following generation request can use the same current group and
allowlist. Removed, disabled or expired keys, inactive users/groups and disallowed
IPs are rejected by the existing authentication guards. Reading metadata does not
require a positive balance, consume quota, or update the key's last-used time.

Responses use `Cache-Control: no-store`. `revision` hashes only safe response
fields and changes with group/model/alias changes. Responses contain no API keys,
account identifiers, credentials, internal account routing tables, or other keys.

The endpoint intentionally bypasses auth and model-list snapshots for discovery;
ordinary generation requests keep their existing caching behavior. No schema
migration or admin credential is needed by clients.
