# Plugin Author Guide

This guide is the entry point for writing a sub2api plugin. It walks the
common path from "git clone" to a working admin page with settings, and
points at the canonical reference plugins (`plugins/hello-world` for the
minimal smoke-test, `plugins/channel-management` for a full-featured
example) plus the deeper architecture docs in this directory.

Audience: Go + Vue authors who already know the language stack and just
need to learn the sub2api SDK contract.

For background on *why* the system is shaped this way, read
`docs/plugin-architecture/DESIGN.md` and the V3/V4/V5 series. This guide
is purely "how to ship a plugin".

---

## §1 Overview & 5-minute quickstart

A sub2api plugin is a separate Go binary that talks to the host over
gRPC, plus an embedded Vue bundle that the host loads into a Shadow Root
on the admin SPA. The host owns the database connection pool, Redis,
auth, the sidebar, i18n, and routing; the plugin contributes routes,
menu items, scheduled jobs, settings and frontend pages.

```
┌──────────────────────┐  gRPC  ┌──────────────────────┐
│ sub2api host process │◄──────►│ plugin binary        │
│  (Go + Gin + Vue SPA)│        │  (Go, pluginsdk.Run) │
└──────────────────────┘        └──────────────────────┘
        │                               ▲
        │  HTTP /api/v1/plugin-assets/<name>/dist/entry.js
        ▼                               │
┌──────────────────────┐  Shadow DOM    │
│ browser (host SPA)   │ ───────────────┘
│  PluginView.vue      │  loads + mounts plugin entry
└──────────────────────┘
```

### Run the smoke test

`plugins/hello-world` is the minimal plugin. It exercises HTTP routes
(no-auth + admin), the SQL proxy, the Redis proxy, the SettingsExtension
read path, and the frontend bundle.

```bash
# 1. Build the host (with VERSION) and the plugin
cd backend && go build ./cmd/server
cd ../plugins/hello-world && go build .

# 2. Build the plugin's frontend bundle so OpenFrontendFile has files to
#    serve (see /c/Users/16790/.../plugins/hello-world/frontend/).
cd frontend && pnpm install && pnpm build

# 3. Run the host with -log-level=debug. Enable + start hello-world via
#    the admin UI (Settings → Plugins) or seed it directly into the
#    plugins table.

# 4. Hit the no-auth endpoint
curl http://localhost:8080/api/v1/plugin/hello-world/hello
# {"message":"Hello from plugin!","version":"0.1.0"}
```

The full lifecycle (`pluginsdk.Run` → gRPC handshake → `Init` →
`RegisterHTTP` → `GetFrontendBundle` → `Shutdown`) is documented in the
header of `plugin-sdk/plugin.go:1-34` and reference-implemented in
`plugins/hello-world/main.go:177-181`.

---

## §2 Project skeleton

A minimal plugin module looks like:

```
plugins/<name>/
├── go.mod                    # separate module; depends on plugin-sdk
├── main.go                   # implements pluginsdk.Plugin + Run()
└── frontend/
    ├── package.json
    ├── vite.config.ts
    ├── src/                  # Vue source
    └── dist/                 # build output, embedded via //go:embed
```

The plugin's `main.go` boilerplate (mirroring
`plugins/hello-world/main.go:1-181`):

```go
package main

import (
    "embed"
    "log"

    pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

type MyPlugin struct{ /* ... */ }

func (p *MyPlugin) Manifest() *pluginsdk.Manifest { /* see §3 */ }
func (p *MyPlugin) Init(ctx pluginsdk.PluginContext) error { return nil }
func (p *MyPlugin) Shutdown() error { return nil }

func main() {
    if err := pluginsdk.Run(&MyPlugin{}); err != nil {
        log.Fatal(err)
    }
}
```

### Required & recommended manifest fields

`plugin-sdk/manifest.go:63-117` defines `Manifest`:

| Field | Required | Notes |
|-------|----------|-------|
| `Name` | yes | Stable identifier; becomes the URL slug `/api/v1/plugin/<name>` |
| `Version` | yes | Plugin version, e.g. `"0.1.0"` |
| `DisplayName` | recommended | Human-readable name shown on the admin Plugins page |
| `Description` | recommended | One-liner on the plugin card |
| `Author` | recommended | Free-form |
| `IconSVG` | recommended | Full `<svg>` markup; rendered next to DisplayName on the plugin card. Use one of the `Icon*` constants in `plugin-sdk/icons.go:21-40` (`IconPuzzle`, `IconBranchFork`, `IconCog`, `IconTag`) or supply your own |

Optional sections (covered later): `GatewayEndpoints`, `PluginEndpoints`,
`Frontend`, `Migrations`, `SettingsSchema`, `Capabilities`.

---

## §3 Declaring menu items & routes

The host renders a single sidebar that merges its own built-in items
with everything plugins contribute via `Frontend.MenuItems`. Each item
becomes a clickable entry that routes to `Frontend.Routes` of the same
`Path`.

### `MenuItemDecl` fields

`plugin-sdk/manifest.go:266-299`:

| Field | Notes |
|-------|-------|
| `Path` | Vue Router path, e.g. `/admin/plugins/hello-world`. Must match a `RouteDecl.Path`. |
| `IconSVG` | Full SVG markup. Prefer this over the legacy `Icon` (icon name). |
| `Labels` | `map[string]string` per-locale label, e.g. `pluginsdk.Labels("渠道管理", "Channel Management")`. Prefer this over the legacy `LabelKey`. |
| `Section` | `pluginsdk.SectionAdmin` or `pluginsdk.SectionUser`. |
| `RequiresAdmin` | If true, item is hidden for non-admin users. |
| `HideInSimpleMode` | If true, hidden when host is in "simple" UI mode. |
| `FeatureFlag` | Optional flag name; host gates the item on it. |
| `Children` | Nested submenu items (one level deep). |
| `SortOrder` | **Legacy** ordering inside Section. Items without `Placement` are appended at the end of the section in `SortOrder` order. |
| `Placement` | **V5/W7 Placement DSL** — see below. Strongly recommended for new code. |

### Placement DSL

`plugin-sdk/manifest.go:119-140`. Pointing a `MenuItemDecl` at a
`Placement` opts that item into the explicit "merge into bucket X at
order Y" algorithm rather than the legacy SortOrder append. Mental
model: VS Code's view containers — predictable buckets, plugin-supplied
relative orders.

The five buckets:

| Constant | Where it lands |
|----------|----------------|
| `PlacementAdminMain` | Top of admin sidebar (dashboard, groups, channels …) |
| `PlacementAdminSystem` | Settings / system area mid-sidebar |
| `PlacementAdminEnd` | Bottom of admin sidebar (test/auxiliary tools) |
| `PlacementUserMain` | Top of user sidebar (dashboard, keys, billing …) |
| `PlacementUserEnd` | Bottom of user sidebar (profile, preferences) |

`Order` is the relative position *inside* a bucket — lower renders
first. Pick numbers with gaps (e.g. 50, 100, 200) so other plugins can
slot in between yours.

Example from `plugins/channel-management/plugin.go:179-241`:

```go
MenuItems: []pluginsdk.MenuItemDecl{
    {
        Path:    "/admin/channels",
        IconSVG: pluginsdk.IconBranchFork,
        Labels:  pluginsdk.Labels("渠道管理", "Channel Management"),
        Section: pluginsdk.SectionAdmin,
        // V5/W7 Placement DSL — admin/main bucket, order 65 leaves
        // room for other plugins to slot in.
        Placement:     &pluginsdk.Placement{Group: pluginsdk.PlacementAdminMain, Order: 65},
        SortOrder:     200, // legacy fallback, kept for old hosts
        RequiresAdmin: true,
        Children: []pluginsdk.MenuItemDecl{
            {
                Path:    "/admin/channels",
                IconSVG: pluginsdk.IconTag,
                Labels:  pluginsdk.Labels("渠道定价", "Channel Pricing"),
                // children inherit ordering from parent — Placement only
                // applies at the top level
                SortOrder: 210,
            },
        },
    },
}
```

`SortOrder` and `Placement` may both be set; `Placement` wins on V5+
hosts and `SortOrder` is the fallback if you ever run on an older host.

### `RouteDecl` ↔ frontend component

`plugin-sdk/manifest.go:301-307`. Each route declares a Vue Router path,
a unique route name, and a `ComponentPath` that the host maps to a Vue
component **registered by your plugin's `install()` function** (see §6).

```go
Routes: []pluginsdk.RouteDecl{
    {
        Path:          "/admin/plugins/hello-world",
        Name:          "PluginHelloWorld",
        ComponentPath: "HelloWorldView.vue",
    },
},
```

> **Do not** put `titleKey`/`descriptionKey` in `RouteDecl.Meta`. Plugin
> i18n is registered asynchronously inside `install()`; AppHeader
> resolves the page title before that namespace is loaded, so the i18n
> key would render verbatim. Titles fall back to `MenuItemDecl.Labels`
> via the host loader runtime — see
> `plugins/channel-management/plugin.go:243-251` for the explanation.

---

## §4 Declaring a settings schema

Plugins describe their admin-tunable knobs with a JSON Schema (Draft-07).
The host renders the schema as a tab on the admin Settings page, persists
writes, validates input, and the plugin reads values back via
`PluginContext.Settings()`.

Reference: `plugin-sdk/manifest.go:142-208` (Go types) and
`plugin-sdk/SETTINGS_API.md` (full quickstart).

### Inline schema (the simple case)

Lifted from `plugins/hello-world/main.go:67-86,130-133`:

```go
var (
    helloWorldSettingsSchema = []byte(`{
      "$schema": "http://json-schema.org/draft-07/schema#",
      "title": "Hello World Plugin Settings",
      "type": "object",
      "properties": {
        "greeting": {
          "type": "string",
          "title": "Greeting",
          "description": "The string returned by /api/v1/plugin/hello-world/hello.",
          "default": "Hello"
        }
      }
    }`)
    helloWorldSettingsDefaults = []byte(`{"greeting":"Hello"}`)
)

// in Manifest():
SettingsSchema: &pluginsdk.SettingsSchemaDoc{
    Schema:   helloWorldSettingsSchema,
    Defaults: helloWorldSettingsDefaults,
},
```

The SDK auto-promotes `CapabilitySettingsExtension` on plugins that ship
a non-empty schema, so you do not need to repeat it in `Capabilities`
(`plugin-sdk/manifest.go:331-345`).

### Generating the schema from a Go struct (recommended)

For non-trivial schemas, generate JSON Schema from a Go struct rather
than hand-maintaining the JSON.

> TODO (verify): the codebase ships `santhosh-tekuri/jsonschema/v5` for
> validation but does not currently pull `github.com/invopop/jsonschema`
> for generation. If your plugin wants codegen, add it to the plugin's
> `go.mod` independently — it's not part of the SDK's transitive deps.

```go
// import "github.com/invopop/jsonschema"

type Settings struct {
    Greeting string `json:"greeting" jsonschema:"title=Greeting,default=Hello"`
    LogLevel string `json:"log_level" jsonschema:"enum=debug|info|warn|error,default=info"`
}

func buildSchema() []byte {
    s := jsonschema.Reflect(&Settings{})
    out, _ := json.Marshal(s)
    return out
}
```

### `properties_meta` (visibility / deprecated / requires_reload)

`plugin-sdk/manifest.go:190-208`. Per-property markers that change the
admin UI behaviour. Either populate
`SettingsSchemaDoc.PropertyMeta` directly or use JSON Schema vendor
extensions on the property (`x-visibility`, `x-deprecated`,
`x-requires-reload`); both paths work, the explicit map wins on conflict.

| Field | Values | Effect |
|-------|--------|--------|
| `Visibility` | `frontend` / `backend` / `secret` | `backend` hides from the form; `secret` masks reads + UI |
| `Deprecated` | non-empty string (migration message) | Strikethrough + warning badge in admin form |
| `RequiresReload` | bool | Host triggers plugin process reload after save |

`channel-management` opts out of an explicit map and lets the host
derive markers from schema vendor extensions
(`plugins/channel-management/plugin.go:165-171`).

### Dynamic fields with if/then/else (V5/W6)

The admin form supports a curated subset of JSON Schema conditional
clauses to hide irrelevant fields based on current values. Backend
validation runs the **full** schema via `santhosh-tekuri/jsonschema`;
the form evaluator (`frontend/src/components/admin/plugin-settings-widgets/evaluateConditions.ts:1-23`)
is purely a UX optimisation.

**Supported subset**:

- Top-level `if` / `then` / `else` (one layer)
- `dependentSchemas: { K: { properties } }` — applies iff `values[K]` is set
- `allOf[*].if/then/else` — order-preserving union of hidden sets
- Leaf condition matchers: `properties.<key>.const`, `properties.<key>.enum`, `required: [...]`, schema-level `type: 'object'`

**NOT supported (will be ignored by the evaluator — values still validate)**:

- `oneOf` / `anyOf`
- Nested `if` inside another `if`'s `then` / `else`

Because the worst case is "field shown but rejected on save", schema
authors can extend their schema with `oneOf`/`anyOf` for validation and
the form will gracefully degrade to "show all".

Example — show `advancedConfig` only when `mode === "advanced"`:

```json
{
  "type": "object",
  "properties": {
    "mode":           { "type": "string", "enum": ["simple", "advanced"], "default": "simple" },
    "simpleTimeout":  { "type": "integer", "default": 30 },
    "advancedConfig": { "type": "object", "properties": { "retries": { "type": "integer" } } }
  },
  "if":   { "properties": { "mode": { "const": "advanced" } } },
  "then": { "required": ["advancedConfig"] },
  "else": { "required": ["simpleTimeout"] }
}
```

**Feature flag**: the conditional renderer is on by default. Set
`VITE_PLUGIN_SETTINGS_CONDITIONS=off` at host build time to disable it
and render every property unconditionally
(`frontend/src/components/admin/PluginSettingsForm.vue:126`).

### Schema versioning

`SettingsSchemaDoc.Version` (e.g. `"1.0.0"`) is stamped into
`plugin_settings.schema_version_at_write` on every write. When the
plugin reads a value back, `GetTyped` returns
`pluginsdk.ErrSchemaVersionMismatch` if the persisted version does not
match the manifest's `Version`. See
`docs/plugin-architecture/SETTINGS-V2-DESIGN.md` §5.5 for the upgrade
playbook (you typically log + use defaults until the admin saves the
form again).

---

## §5 gRPC handler / Job registration

A plugin contributes HTTP endpoints by:

1. Declaring them in `Manifest.PluginEndpoints` (admin/management) or
   `Manifest.GatewayEndpoints` (user-facing gateway forwards).
2. Implementing `pluginsdk.HTTPRegistrar` and registering handlers on
   the SDK-provided mux.

### `HTTPRegistrar`

`plugin-sdk/plugin.go:42-56`:

```go
func (p *HelloPlugin) RegisterHTTP(mux pluginsdk.HTTPMux) {
    // Paths must match what Manifest declared verbatim — the host gateway
    // does NOT strip the /api/v1/plugin/<name> prefix before forwarding.
    mux.Handle(pluginRoutePrefix+"/hello", http.HandlerFunc(p.handleHello))
    mux.Handle(pluginRoutePrefix+"/db-test", http.HandlerFunc(p.handleDBTest))
    mux.Handle(pluginRoutePrefix+"/redis-test", http.HandlerFunc(p.handleRedisTest))
}
```

Reference: `plugins/hello-world/main.go:155-162`.

For larger surfaces, plug a `*gin.Engine` (or any `http.Handler`) into
the mux. `plugins/channel-management/plugin.go:380-387` mounts a Gin
engine at the prefix and then attaches groups in
`registerRoutes()` (lines 391-435).

### Endpoints declaration & auth types

`plugin-sdk/manifest.go:230-235`. `AuthType` is one of:

| Constant | Effect |
|----------|--------|
| `AuthTypeNone` | Public; gateway forwards without auth |
| `AuthTypeUser` | Requires logged-in user; host injects `X-Plugin-User-*` headers |
| `AuthTypeAPIKey` | Requires valid API key; useful for gateway-style endpoints |
| `AuthTypeAdmin` | Admin-only; rejects non-admins at the gateway |

### Scheduled jobs (`PluginContext.Jobs()`)

If you need cron-style or interval-style background work, declare
`pluginsdk.CapabilityJobScheduler` and register specs from `Init`.

API: `plugin-sdk/jobs.go:14-108`. Key types:

- `JobTriggerKind`: `TriggerInterval` / `TriggerCron` / `TriggerFixedDelay`
- `JobSpec`: name, trigger, `LeaderOnly` (run only on the leader replica), `Concurrency`, `Timeout`
- `JobsClient.Register(spec, handler)`: call before returning from `Init`

Channel-management uses this for periodic monitor checks
(`plugins/channel-management/plugin.go:344-353`):

```go
jobRunner := monitorService.NewMonitorJobRunner(p.monitorService, ctx.Jobs(), ctx.Logger())
if err := jobRunner.Register(); err != nil {
    ctx.Logger().Warn("channel-monitor: job registration failed", "error", err)
}
```

The host owns the schedule clock, the per-(plugin, job) leader lock, and
admin-visible job history; the plugin owns the handler. See
`docs/plugin-architecture/V5-DESIGN.md` §2 for the full lifecycle.

> Other extension points exposed via `PluginContext` (`plugin-sdk/context.go`):
> `DB()`, `Redis()`, `Secrets()` (encryption), `Logger()`. See the
> respective files (`plugin-sdk/secrets.go`, `plugin-sdk/REDIS_API.md`)
> and per-API godoc.

---

## §6 Frontend view authoring

The host loads your `frontend/dist/entry.js` into a Shadow Root via
`frontend/src/views/plugin/PluginView.vue`, then calls your bundle's
`install(sdk)` and mounts the returned root component. The host injects
Tailwind preflight + design-token CSS into the shadow root so plugin
components visually match the host without redeclaring `@tailwind base`
(`frontend/src/plugins/mount-plugin.ts:34,91-92`).

### Page layout primitives (recommended)

Always wrap a plugin page in `PluginPageLayout`. The host's title-bar
fallback chain reads the title from this component (and from
`MenuItemDecl.Labels`), so omitting it produces an unreliable header.

`frontend/packages/plugin-sdk/src/index.ts:19-22` exports:

- `PluginPageLayout` — outer page frame; slots: `default`, `actions`
- `TablePageLayout` — page frame with named slots `actions`, `filters`, `table`, `pagination`
- `PageActions` — button row container (right-aligned)
- `FilterBar` — slots `#left` and `#right` for filter widgets

Widgets:

- `SearchInput` (line 24) — text input with a built-in 300ms debounce; emits `@search`
- `StatusBadge` (line 25) — coloured pill for status enums

Generic UI:

- `Icon`, `DataTable`, `BaseDialog`, `ConfirmDialog`, `Select`, `Pagination`, `EmptyState`, `Toggle`, `PlatformIcon` (lines 7-15)

### Example view (simplified from channel-management)

Adapted from
`plugins/channel-management/frontend/src/views/user/AvailableChannelsView.vue:1-122`:

```vue
<template>
  <PluginPageLayout
    :title="t('availableChannels.title')"
    :description="t('availableChannels.description')"
  >
    <template #actions>
      <PageActions>
        <button
          @click="loadChannels"
          :disabled="loading"
          class="btn btn-secondary"
          :title="t('common.refresh')"
        >
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
        </button>
      </PageActions>
    </template>

    <FilterBar>
      <template #left>
        <div class="w-full sm:w-80">
          <SearchInput
            v-model="searchQuery"
            :placeholder="t('availableChannels.searchPlaceholder')"
          />
        </div>
      </template>
    </FilterBar>

    <AvailableChannelsTable :rows="filteredChannels" :loading="loading" />
  </PluginPageLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  FilterBar, Icon, PageActions, PluginPageLayout, SearchInput,
} from '@sub2api/plugin-sdk'
import { getSdk } from '../api/sdk'
import { extractApiErrorMessage } from '../utils/apiError'

const { t } = useI18n()
const sdk = getSdk()

const channels = ref<Channel[]>([])
const loading = ref(false)
const searchQuery = ref('')

const filteredChannels = computed(() =>
  channels.value.filter(c => c.name.toLowerCase().includes(searchQuery.value.toLowerCase())),
)

async function loadChannels() {
  loading.value = true
  try {
    channels.value = await fetchChannels()
  } catch (err: unknown) {
    sdk.notify.error(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadChannels)
</script>
```

### Styling rules

- **Use Tailwind utilities** + the design tokens injected by the host.
  Plugin components should look identical to host components by default.
- **Avoid raw Tailwind semantic colour classes** (`text-red-500`,
  `bg-blue-100`, …). Prefer SDK semantic utility classes (`.btn-primary`,
  `.btn-icon-danger`, `.input-required`, `.badge-*`, `.card-highlight-*`).
  Enforced by the custom ESLint rule
  `frontend/.eslint-rules/no-raw-semantic-color.cjs`; run
  `pnpm --filter sub2api-frontend run lint:plugins` to check. Forbidden
  hues: `emerald|red|blue|amber|green|yellow`. Brand colours `primary-*`
  (theme accent) and `purple-*` (Antigravity platform) are intentionally
  allowed.
- Do **not** ship `@tailwind base` in your plugin's CSS — preflight is
  injected into the shadow root by the host loader at mount time.

---

## §7 Reading plugin settings

### From the backend

`plugin-sdk/settings.go:107-122`. `PluginContext.Settings()` exposes a
`SettingsClient` with three methods:

```go
// Raw JSON value (or pluginsdk.ErrSettingNotFound)
raw, err := pctx.Settings().Get(ctx, "greeting")

// Unmarshal into a Go value
var greeting string
err := pctx.Settings().GetTyped(ctx, "greeting", &greeting)

// Subscribe to changes; pass "" to watch every key in the namespace.
ch, cleanup, err := pctx.Settings().Watch(ctx, "")
defer cleanup()
for change := range ch {
    log.Println("setting updated:", change.Key, change.Revision)
}
```

`hello-world` reads its greeting on every request
(`plugins/hello-world/main.go:187-205`):

```go
func (p *HelloPlugin) handleHello(w http.ResponseWriter, r *http.Request) {
    greeting := "Hello"
    if pctx := p.context(); pctx != nil {
        ctx, cancel := context.WithTimeout(r.Context(), dbTestTimeout)
        defer cancel()
        var got string
        if err := pctx.Settings().GetTyped(ctx, "greeting", &got); err == nil && got != "" {
            greeting = got
        }
    }
    writeJSON(w, http.StatusOK, map[string]string{
        "message": greeting + " from plugin!",
        "version": pluginVersion,
    })
}
```

`Watch` is the right choice for hot-reloading config into a long-lived
service. The host emits a synthetic snapshot for every existing key
when the stream opens, so subscribers see current state without an
extra `Get`.

### Schema-version drift

If the persisted value was written under a stale schema version,
`GetTyped` returns `pluginsdk.ErrSchemaVersionMismatch`. Recommended
handling:

```go
var cfg MySettings
err := pctx.Settings().GetTyped(ctx, "config", &cfg)
switch {
case errors.Is(err, pluginsdk.ErrSettingNotFound),
     errors.Is(err, pluginsdk.ErrSchemaVersionMismatch):
    // Fall back to defaults; admin will resave to clear the drift.
    cfg = defaultConfig()
case err != nil:
    return err
}
```

The admin Settings page also surfaces a banner card indicating the
saved version is stale, so operators know to re-save.

### From the frontend

> TODO (verify): the host frontend SDK (`frontend/packages/plugin-sdk/src/host-sdk.ts`)
> does **not** currently expose `sdk.settings.get(key)` or
> `sdk.settings.watch(key, cb)`. Plugin frontends that need values
> must either:
>
> 1. Have the backend handler include the relevant settings in its
>    response, or
> 2. Hit the admin REST surface (`GET /api/v1/admin/plugin-settings/:plugin`)
>    directly when running in admin context.
>
> A `sdk.settings` API is on the SETTINGS-V2 roadmap — see
> `docs/plugin-architecture/SETTINGS-V2-DESIGN.md` for the eventual
> shape.

---

## §8 Debugging

### Host-side logs

```bash
go run ./backend/cmd/server -log-level=debug
```

Every plugin gRPC call is logged at `debug`; lifecycle events (Init,
Manifest, Shutdown) at `info`.

### Plugin-side logs

`pctx.Logger()` returns an `*slog.Logger` whose output is forwarded to
the host's structured log via the `LogProxy` gRPC service
(`plugin-sdk/proto/sdk.proto` lines 69-…). All plugin lines appear in
the host's stdout with `plugin=<name>` attached, so a single
`docker logs` covers everything.

```go
ctx.Logger().Info("hello-world plugin initialised", "version", pluginVersion)
```

(Reference: `plugins/hello-world/main.go:142`.)

### Settings inspection

The admin REST surface (declared in
`backend/internal/server/routes/admin.go:569-573`):

```
GET  /api/v1/admin/plugin-settings              — list plugin namespaces
GET  /api/v1/admin/plugin-settings/:plugin      — schema + current values
PUT  /api/v1/admin/plugin-settings/:plugin/:key — overwrite one key
```

These are admin-only; authenticate with the admin API key (see project
`CLAUDE.md` for the convention).

### Watch debugging

Watch streams reconnect with a 1s → 3s → 9s → 30s back-off
(`plugin-sdk/settings.go:84-90`). If you see periodic
`runWatchLoop reconnect` lines in the plugin log, the host is bouncing
its gRPC server — usually a host restart, not a plugin bug.

### Migration drift

The host re-verifies SHA-256 of every migration body fetched via
`PluginLifecycle.GetMigration` against `MigrationDecl.ChecksumSha256`
(`plugin-sdk/manifest.go:237-255`). A drift error during host startup
("checksum mismatch for plugin X migration 003_foo.sql") almost always
means the SQL file was edited after shipping. Add a follow-up file
instead of rewriting history.

---

## §9 Reference index — guide section ↔ canonical example

| Guide § | Topic | Reference file:line |
|---------|-------|---------------------|
| §2 Project skeleton | minimal `main.go` | `plugins/hello-world/main.go:1-181` |
| §2 Project skeleton | manifest fields | `plugin-sdk/manifest.go:63-117` |
| §3 Menu / Routes | basic menu item | `plugins/hello-world/main.go:108-127` |
| §3 Placement DSL | bucket + order | `plugin-sdk/manifest.go:119-140` |
| §3 Placement DSL | full example with parent + children | `plugins/channel-management/plugin.go:179-241` |
| §3 Routes | RouteDecl + i18n caveat | `plugins/channel-management/plugin.go:243-281` |
| §4 Settings schema | inline schema bytes | `plugins/hello-world/main.go:67-86,130-133` |
| §4 Settings schema | embedded JSON files + version | `plugins/channel-management/plugin.go:42-51,165-171` |
| §4 Conditional fields | evaluator subset doc | `frontend/src/components/admin/plugin-settings-widgets/evaluateConditions.ts:1-35` |
| §4 Conditional fields | feature flag | `frontend/src/components/admin/PluginSettingsForm.vue:126` |
| §5 HTTPRegistrar | net/http handlers | `plugins/hello-world/main.go:155-162` |
| §5 HTTPRegistrar | gin engine | `plugins/channel-management/plugin.go:380-435` |
| §5 Job scheduler | spec + Register | `plugin-sdk/jobs.go:14-108` |
| §5 Job scheduler | usage | `plugins/channel-management/plugin.go:344-353` |
| §6 Layout components | exports list | `frontend/packages/plugin-sdk/src/index.ts:1-30` |
| §6 Page layout | full example | `plugins/channel-management/frontend/src/views/user/AvailableChannelsView.vue:1-122` |
| §7 Settings reads | GetTyped fallback | `plugins/hello-world/main.go:187-205` |
| §7 Settings API | client interface | `plugin-sdk/settings.go:107-122` |
| §8 Debugging | logger | `plugins/hello-world/main.go:142` |
| §8 Debugging | admin REST routes | `backend/internal/server/routes/admin.go:569-573` |

---

## §10 Common pitfalls

1. **Redis double prefix.** The SDK auto-prefixes every Redis key with
   `plugin:<name>:`. Passing `plugin:hello-world:smoke-test` to
   `Redis().Get` produces `plugin:hello-world:plugin:hello-world:smoke-test`.
   Pass the bare key (`smoke-test`); see
   `plugins/hello-world/main.go:45-51`.

2. **Menu Path ↔ Route Path mismatch.** A `MenuItemDecl.Path` that does
   not have a matching `RouteDecl.Path` in the same manifest produces a
   sidebar entry that 404s. Always declare both.

3. **`schema_version` drift.** Bumping `SettingsSchemaDoc.Version` while
   keeping the manifest schema *shape* the same will cause
   `GetTyped` to return `ErrSchemaVersionMismatch` for every existing
   row until the admin re-saves the form. If the shape did not actually
   change, leave the version alone.

4. **Frontend bundle path.** Plugins must put their built bundle at
   `frontend/dist/` *inside the plugin module*. The manifest declares
   `Frontend.EntryJS = "dist/entry.js"`, and the SDK's
   `OpenFrontendFile` reads `frontend/<rel>` from the embedded FS —
   missing `frontend/dist/entry.js` → host returns
   `fs.ErrNotExist` and the plugin page never loads
   (`plugins/channel-management/plugin.go:24-30,463-471`).

5. **Tailwind preflight in plugin CSS.** The host injects preflight into
   the shadow root at mount time. Plugins shipping their own
   `@tailwind base` produce double-injected styles + cascade fights.
   Use the SDK Tailwind preset (`frontend/packages/plugin-sdk/tailwind-preset.cjs`)
   for tokens only.

6. **i18n key in `RouteDecl.Meta.titleKey`.** Plugin i18n is registered
   asynchronously inside `install()`. AppHeader resolves
   `pageTitle` before that namespace lands, so the i18n key renders
   verbatim. Rely on `MenuItemDecl.Labels` instead — the host passes
   them through to AppHeader's fallback chain
   (`plugins/channel-management/plugin.go:243-251`).

7. **HTTP route prefix stripping.** The host gateway forwards the
   *full* path (`/api/v1/plugin/<name>/foo`) to your plugin's HTTP
   server. Register handlers at the full path, not at the suffix
   (`plugins/hello-world/main.go:153-162`).

8. **Migrations are append-only.** Never reorder or rewrite a shipped
   migration in place. The host pins each filename + SHA-256 in
   `plugin_migrations` history; rewriting one breaks every existing
   install. Add `004_fix_…sql` instead
   (`plugins/channel-management/plugin.go:154-164`).

9. **Watch handler blocking.** `SettingsClient.Watch` returns an
   unbuffered channel; if your handler blocks, you stall the watch
   stream and the plugin's local cache (`plugin-sdk/settings.go:71-76`)
   diverges from the host. Drain quickly or fan out.

10. **`Init` is the only place to call `Jobs().Register`.** Once the
    plugin has returned from `Init`, the SDK opens the JobScheduler
    Subscribe stream and rejects further `Register` calls with
    `ErrJobsRegistered` (`plugin-sdk/jobs.go:110-115`).

---

## Where to go next

- `docs/plugin-architecture/DESIGN.md` — system-wide design rationale
- `docs/plugin-architecture/V5-DESIGN.md` — current SDK capabilities
  (W1 migrations, W2 jobs, W3 settings, W5 secret encryption, W6 monitor)
- `docs/plugin-architecture/SETTINGS-V2-DESIGN.md` — settings schema
  versioning, property metadata, admin UX details
- `docs/plugin-architecture/SDK-V2-CURATE.md` — frontend SDK Plan B/C
  (layout primitives, widgets, conditional renderer)
- `plugin-sdk/SETTINGS_API.md` — settings API quickstart (mirrors §4 + §7)
- `plugin-sdk/REDIS_API.md` — Redis proxy semantics + `redis_raw_keys`
