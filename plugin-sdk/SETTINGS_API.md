# SettingsExtension SDK Quickstart

Plugins ship a JSON Schema describing their admin-tunable knobs, then
read the current values via `PluginContext.Settings()`. The host
persists writes, validates against the schema, and renders a per-plugin
form on the admin UI at `/admin/plugin-settings`.

## 1. Declare the schema in your manifest

```go
var (
    helloWorldSettingsSchema = []byte(`{
      "$schema": "http://json-schema.org/draft-07/schema#",
      "type": "object",
      "properties": {
        "greeting": {
          "type": "string",
          "title": "Greeting",
          "description": "Returned by the /hello endpoint.",
          "default": "Hello"
        }
      }
    }`)
    helloWorldSettingsDefaults = []byte(`{"greeting":"Hello"}`)
)

func (p *HelloPlugin) Manifest() *pluginsdk.Manifest {
    return &pluginsdk.Manifest{
        Name: "hello-world",
        // ... other manifest fields ...
        SettingsSchema: &pluginsdk.SettingsSchemaDoc{
            Schema:   helloWorldSettingsSchema,
            Defaults: helloWorldSettingsDefaults,
        },
    }
}
```

The SDK auto-promotes `CapabilitySettingsExtension` for plugins that ship
a non-empty schema, so you do not need to declare it separately. The host
seeds the `Defaults` JSON for keys that have not been written yet, so
`Settings.Get` returns a value immediately after install.

## 2. Read values at runtime

```go
func (p *HelloPlugin) handleHello(w http.ResponseWriter, r *http.Request) {
    var greeting string
    err := p.ctx.Settings().GetTyped(r.Context(), "greeting", &greeting)
    if err != nil {
        // ErrSettingNotFound: schema/defaults missed; fall back.
        greeting = "Hello"
    }
    writeJSON(w, http.StatusOK, map[string]string{"message": greeting})
}
```

`GetTyped` runs `Settings.Get` followed by `json.Unmarshal`; missing keys
return `pluginsdk.ErrSettingNotFound` so callers can branch on them
without parsing error strings.

## 3. React to admin changes (optional)

```go
ch, cleanup, err := p.ctx.Settings().Watch(ctx, "")
if err != nil {
    return err
}
defer cleanup()
for change := range ch {
    p.logger.Info("setting updated",
        "key", change.Key, "rev", change.Revision)
}
```

An empty key subscribes to every key in the namespace. The host emits a
synthetic snapshot event for each existing key when the stream opens, so
the plugin always sees current state without an extra `Get` round-trip.

## 4. Admin REST surface

| Method | Endpoint | Body | Notes |
|---|---|---|---|
| `GET`  | `/api/v1/admin/plugin-settings`                | — | List plugins with registered schemas |
| `GET`  | `/api/v1/admin/plugin-settings/:plugin`        | — | Returns `{schema, defaults, values}` |
| `PUT`  | `/api/v1/admin/plugin-settings/:plugin/:key`   | `{"value": ...}` | 422 on schema violation, 409 if no schema yet |

Authentication: standard admin session or `x-api-key` Admin API Key.

## 5. Schema notes

* JSON Schema **Draft 07** is the recommended dialect. The host uses
  `santhosh-tekuri/jsonschema/v5`, which also accepts later drafts when
  declared via `$schema`.
* The top-level `properties` map is what the admin UI renders. Use
  `title`, `description`, `default`, `enum` — the renderer reads them
  directly.
* Schema changes require a plugin restart. The host caches the compiled
  schema in memory and writes to `plugin_settings_schemas` on
  `RegisterSchema`; old values stay intact across schema changes (the
  plugin is responsible for graceful degradation).
