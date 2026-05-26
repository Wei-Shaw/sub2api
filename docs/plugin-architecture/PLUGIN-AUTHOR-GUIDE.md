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

## §11 Subscribing to host events

The Phase A `EventsExtension` lets a plugin react to typed business
events emitted by the host (payments, gateway invocations, user
registration, account rate limits). Worked example:
`plugins/hello-world/main.go:147,160-170,187-205`.

### §11.1 Concept

- **At-most-once** delivery. The host does not persist or replay
  events; if your plugin is offline when an event fires, you will
  never see it. Reconciliation that needs replay must use a
  Job-driven pull instead (see §5).
- **256-slot ring buffer per subscriber** with **drop-oldest** on
  overflow. The host increments `dropped_since_last_send` on the next
  event so your handler can tell the SDK is behind.
- **2 s send timeout.** If a handler stalls and back-pressures the
  ring, the host closes the stream; the SDK then reconnects with
  exponential backoff.
- **Reconnect schedule:** `1s → 2s → 4s → 8s → 30s`, ±10 % jitter
  (see `plugin-sdk/events.go` constants).

The SDK itself does not retry handler logic — once `EventHandler`
returns the event is gone. If you need durable processing, push the
event onto your own Redis list / SQL queue inside the handler and
process it from a Job.

### §11.2 Subscribing

Two steps. First, declare the events in your manifest:

```go
return &pluginsdk.Manifest{
    Name: "my-plugin",
    // ...
    SubscribedEvents: []string{
        pluginsdk.EventTypePaymentOrderFulfilled,
        pluginsdk.EventTypeAuthUserRegistered,
    },
}
```

The host validates the manifest at install time and rejects
`Subscribe` calls for types not on this list with `InvalidArgument`.

Second, call `Events().Subscribe` from `Init`:

```go
func (p *MyPlugin) Init(ctx pluginsdk.PluginContext) error {
    p.eventsCtx, p.eventsCancel = context.WithCancel(context.Background())
    return ctx.Events().Subscribe(
        p.eventsCtx,
        []string{pluginsdk.EventTypePaymentOrderFulfilled},
        p.onPaymentFulfilled,
    )
}

func (p *MyPlugin) onPaymentFulfilled(ctx context.Context, evt *pluginsdk.HostEvent) {
    payload := evt.GetPaymentOrderFulfilled()
    if payload == nil {
        return // defensive: schema migration could send empty oneof
    }
    // ... business logic ...
}
```

Notes:

- `Subscribe` returns immediately. It spins up its own goroutine for
  the receive loop; you do not need to wrap the call in a `go`.
- The handler runs **serially per subscription** (one goroutine per
  `Subscribe` call). If you need to do non-trivial work, fork a
  goroutine inside the handler. Do **not** retain the `*HostEvent`
  pointer past the handler return.
- Cancel the context (typically from `Shutdown`) to stop the loop
  cleanly before the SDK tears down the gRPC connection.
- Multiple `Subscribe` calls are fine: each gets its own stream and
  goroutine. Use this when you want separate handlers for separate
  event types.

### §11.3 Event schemas

All event types and field names are declared in
`plugin-sdk/proto/sdk.proto`; the SDK exposes Go aliases in
`plugin-sdk/events.go` (`PaymentOrderCreated`, `GatewayModelInvoked`,
…) plus `EventType*` string constants for use in `SubscribedEvents`.

| Event type | Payload getter | Use case |
|---|---|---|
| `payment.order.created` | `GetPaymentOrderCreated()` | invoice numbering, anti-fraud first-pass |
| `payment.order.fulfilled` | `GetPaymentOrderFulfilled()` | receipt email, downstream ledger sync |
| `gateway.model.invoked` | `GetGatewayModelInvoked()` | usage-based billing, custom analytics |
| `auth.user.registered` | `GetAuthUserRegistered()` | welcome email, referral credit, CRM push |
| `account.rate_limit.triggered` | `GetAccountRateLimitTriggered()` | ops alerting, automatic account swap |

Key fields per payload:

- **`PaymentOrderCreated`** — `OrderId`, `OutTradeNo`, `UserId`,
  `AmountCents`, `PlanId` (empty for non-subscription),
  `ProviderKey` (`stripe` / `easypay` / …), `BizType`
  (`balance` / `subscription`), `CreatedAtUnixNano`.
- **`PaymentOrderFulfilled`** — `OrderId`, `UserId`, `AmountCents`,
  `BizType`, `AuditAction` (matches the row in `audit_log`),
  `FulfilledAtUnixNano`.
- **`GatewayModelInvoked`** — `RequestId`, `UserId`, `AccountId`,
  `Platform` (`antigravity` / `anthropic` / `openai` / `gemini`),
  `Model`, `PromptTokens`, `CompletionTokens`, `StatusCode`
  (upstream HTTP), `LatencyMs`, `StartedAtUnixNano`.
- **`AuthUserRegistered`** — `UserId`, `Email`, `Source`
  (`register` / `oauth` / …), `ReferrerId` (0 if none),
  `RegisteredAtUnixNano`.
- **`AccountRateLimitTriggered`** — `AccountId`, `Platform`, `Model`
  (empty if account-wide), `Scope` (`account` / `model` / …),
  `ResetAtUnixNano`, `Reason`.

Event envelope fields (on `HostEvent` itself) are useful too:
`GetEventId()` for de-duplication if you write to your own queue,
`GetTimestampNanos()` for ordering, and
`GetDroppedSinceLastSend()` (see §11.6).

### §11.4 High-frequency events require a capability

`gateway.model.invoked` fires on **every** gateway request and can
saturate a slow handler in seconds. To prevent accidental
subscription, the host gates it behind the `events.gateway`
capability:

```go
return &pluginsdk.Manifest{
    Capabilities: []string{"events.gateway"},
    SubscribedEvents: []string{pluginsdk.EventTypeGatewayModelInvoked},
}
```

Subscribing without the capability returns
`status.Code(err) == codes.PermissionDenied` synchronously from
`Subscribe`. The capability is opt-in only — the operator can grant
it from the plugin install screen. Plugins should drop the event into
a buffered channel or Redis stream and process asynchronously rather
than do anything blocking inside the handler.

### §11.5 Error handling

`Subscribe` returns errors in two distinct shapes:

| Source | Behaviour |
|---|---|
| Synchronous `error` from `Subscribe` | Setup failure: nil/empty inputs, `PermissionDenied`, `InvalidArgument`, or `Unimplemented`. Operator action required. |
| Internal log line, no return | Stream-level transport drop. The SDK reconnects with backoff; you see `events stream lost; reconnecting` in the host log. |

The SDK only stops the loop on a non-retryable status code
(`PermissionDenied` / `InvalidArgument` / `Unimplemented`) or on ctx
cancel. Everything else (EOF, `Unavailable`, network errors) is
retried indefinitely.

### §11.6 Debugging

Three log lines are useful when troubleshooting:

- `events stream lost; reconnecting` — emitted by the SDK at WARN.
  Look for `error` and `backoff` attributes; sustained reconnects
  point to a host crash loop or a slow handler.
- `host dropped events before this delivery; consider faster
  handlers or fewer subscriptions` — emitted by the SDK at WARN
  when `dropped_since_last_send > 0`. Action: profile your handler
  or batch work onto a goroutine.
- `plugin event dropped` / `event send timeout` — emitted by the
  **host** at WARN when its 256-slot buffer overflows or the 2 s
  send timeout fires. Look for `plugin` + `event_type` to identify
  the slow subscriber.

### §11.7 What is NOT supported

- **Filter / mutation hooks** (running before the host commits state
  and rejecting / rewriting the action). Phase A is fire-and-forget
  notification only. If you need to block a registration or rewrite
  a payment, register an HTTP middleware via the gateway plugin
  surface and run your logic there — the host calls those
  synchronously.
- **Replay / historical events.** The host does not retain a log;
  events that fire while you are offline are lost. Use a
  `Jobs().Register` interval that polls the relevant SQL table for
  reconciliation flows.
- **Cross-plugin events.** Plugins receive only host-emitted events.
  If two plugins need to coordinate, publish through Redis Pub/Sub
  with a shared key (requires `redis_raw_keys`).

---

## §12 Capabilities

Capabilities are the host's allow-list of privileged surfaces a plugin can
touch (raw Redis keys, gateway-scoped routes, host-shared DB tables, secret
encryption, etc.). They live in your manifest's `Capabilities` slice and
are enforced at every gRPC entry point in the host SDK server.

### §12.1 Concept — two categories

| Category | Behaviour | Examples |
|----------|-----------|----------|
| **default-grant** | Host approves automatically. Listed for transparency / audit. Plugins do **not** need to declare them to use the corresponding API surface (own-namespace tables, own-namespace settings, jobs, plugin-scoped routes, low-frequency events, outbound HTTP). | `db.own.read`, `db.own.write`, `redis.own`, `settings.own.read`, `settings.own.write`, `jobs.register`, `http.register.plugin`, `events.subscribe.lowfreq`, `outbound.http`, `migrations.apply` |
| **declare-required** | Plugin **must** list the capability in its manifest. Host honours the request today; admin-side approval is a Phase 2 follow-up (`docs/plugin-architecture/V5-DESIGN.md` §A-future). Sensitivity is the rationale — these surfaces touch shared state or escape the plugin's own namespace. | `redis.raw`, `secrets.encrypt`, `events.subscribe.gateway`, `http.register.gateway`, `db.core.read`, `db.core.write` |

> **Note on default-grant**: even though the host approves these
> automatically, listing them in your manifest is encouraged — it makes
> the admin "Permissions" panel a complete audit surface and self-documents
> the plugin's resource footprint.

### §12.2 Capability catalogue

The full set is defined in
`backend/internal/plugin/manager.go::allowedPluginCapabilities`. Anything
outside this set is dropped with a `plugin requested unknown capability`
WARN at register time.

| Capability | Default? | Used for | Where it gates |
|------------|----------|----------|----------------|
| `redis.own` | default | Plugin-namespaced Redis keys (auto-prefixed) | SDK Redis proxy |
| `redis.raw` | declare | Raw / shared Redis keys (no prefix) | `grpc_server_redis_do.go` raw_key=true |
| `db.own.read` | default | Read plugin-private tables created by the plugin's migrations | SQL gate, `OwnedTables` allow-list |
| `db.own.write` | default | Write to plugin-private tables | SQL gate |
| `db.core.read` | declare | Read host shared tables (users, accounts, payment_orders, ...) | SQL gate, host shared allow-list |
| `db.core.write` | declare | Write to host shared tables — **dangerous** | SQL gate (Phase 2: admin-approve gate) |
| `migrations.apply` | default | Run plugin-shipped SQL migrations on plugin startup | MigrationProxy |
| `settings.own.read` | default | Read plugin-namespaced W3 settings | SettingsExtension |
| `settings.own.write` | default | Write plugin-namespaced W3 settings | SettingsExtension |
| `secrets.encrypt` | declare | Encrypt / decrypt secrets via host AES-GCM | SecretEncryption |
| `jobs.register` | default | Register host-coordinated cron / leader-locked jobs | JobScheduler |
| `events.subscribe.lowfreq` | default | Subscribe to low-frequency events (payments, account rate-limits, ...) | EventsExtension |
| `events.subscribe.gateway` | declare | Subscribe to per-request gateway events — high cardinality | EventsExtension |
| `http.register.plugin` | default | Register HTTP handlers under `/api/plugins/<name>/...` | Router gate |
| `http.register.gateway` | declare | Register handlers under `/api/v1/...` (host gateway namespace) | Router gate |
| `outbound.http` | default | Make outbound HTTP calls via the host-managed proxy with default block list | SafeOutboundHTTP |

### §12.3 Declaring capabilities in the manifest

```go
return &pluginsdk.Manifest{
    Name: "my-plugin",
    // ...
    Capabilities: []string{
        pluginsdk.CapabilityRedisRaw,        // declare-required
        pluginsdk.CapabilitySecretsEncrypt,  // declare-required
        pluginsdk.CapabilityEventsSubscribeGateway, // declare-required
        pluginsdk.CapabilityHTTPRegisterPlugin,     // default-grant; listing for audit
    },
}
```

Use the `pluginsdk.Capability*` constants (defined in
`plugin-sdk/manifest.go`) — the host parses string values, but the
constants keep your code in sync if a name is ever renamed.

### §12.4 Owned tables (`db.own.*` allow-list)

When you use the SQL gate (i.e., do anything via the host DB proxy), you
must enumerate the tables your migrations create:

```go
return &pluginsdk.Manifest{
    // ...
    OwnedTables: []string{
        "channel_management_logs",
        "channel_management_alerts",
    },
}
```

The SQL gate composes this with the host's shared table allow-list
(`users`, `accounts`, `payment_orders`, ...). Reads / writes to host
shared tables additionally require `db.core.read` / `db.core.write`.

### §12.5 Migrating from legacy snake_case names

P12·B-1 renamed all capabilities from `snake_case_with_periods` to
canonical `dotted.lowercase`. The host normalises legacy declarations
internally and emits a deprecation WARN per plugin at register time:

```
plugin uses deprecated capability name — please migrate
  plugin=my-plugin deprecated=redis_raw_keys replacement=redis.raw
```

| Legacy | Canonical |
|--------|-----------|
| `redis_raw_keys` | `redis.raw` |
| `safe_outbound_http` | `outbound.http` |
| `secret_encryption` | `secrets.encrypt` |
| `job_scheduler` | `jobs.register` |
| `settings_extension` | `settings.own.read` (add `settings.own.write` if you write) |
| `events.gateway` | `events.subscribe.gateway` |

Plugins that still ship `events.gateway` continue to work for one
release; they appear in the admin "Permissions" panel under their
canonical name regardless of which form the manifest used.

### §12.6 Debugging

If your plugin is hitting "permission denied" at runtime:

1. Look at the host log around plugin start. The host emits
   `plugin requested unknown capability — ignored` for typos and
   `plugin uses deprecated capability name` for legacy aliases. Both
   include the plugin name and the offending string.
2. Open the admin → Plugins page → click the plugin → "Permissions"
   panel. The list there is the host-approved set; if a capability is
   missing it never reached enforcement.
3. The actual gate that rejected your call lives in the SDK server
   (e.g., `redis_raw_keys` rejections come from
   `backend/internal/plugin/grpc_server_redis_do.go`). Each gate logs
   the plugin name + the requested resource on rejection.

---

## §13 Lifecycle

P13·C introduced the four-state lifecycle so admins can archive a plugin
without losing data and later either restore or permanently purge it.

### §13.1 State machine

```
absent ──install──► installed ──enable──► enabled
                       ▲ │                   │
                       │ └──────disable──────┘
                       │
                  [Restore]                  [Disable]
                       │                          │
                       ▼                          ▼
                  uninstalled (uninstalled_at NOT NULL)
                       │
                  [hard purge]
                       ▼
                     absent
```

| State | DB row? | Process running? | Data preserved? |
|-------|---------|------------------|-----------------|
| absent | no | no | n/a |
| installed (= disabled) | yes, `enabled=false`, `uninstalled_at IS NULL` | no | yes |
| enabled | yes, `enabled=true`, `uninstalled_at IS NULL` | yes | yes |
| uninstalled (soft) | yes, `enabled=false`, `uninstalled_at NOT NULL` | no | **yes** |

### §13.2 enable / disable semantics

- `enable`: runs all pending up-migrations, spawns the plugin process,
  registers schema + jobs + events.
- `disable`: stops the process, calls `UnregisterSchema`,
  unsubscribes events, but **does not** drop any data. Toggling
  `enable ↔ disable` is non-destructive — `plugin_settings`,
  `plugin_migrations`, and the plugin's own tables stay put across
  arbitrarily many cycles.

### §13.3 Soft uninstall

`POST /api/v1/admin/plugins/:name/uninstall`

Equivalent to `disable` plus a `uninstalled_at = NOW()` stamp on the
plugins row. The default list query filters out soft-uninstalled rows
(sidebar / menu / route list never show them) so they're effectively
hidden until an admin opts into the "Show uninstalled only" view.

Reversal: `POST /api/v1/admin/plugins/:name/install` clears
`uninstalled_at` and leaves the plugin in `installed` (disabled-but-
present) state. The admin must explicitly enable it again — restore is
intentionally conservative so a plugin archived because it broke does
not auto-spawn on recovery.

### §13.4 Hard purge

`DELETE /api/v1/admin/plugins/:name?purge=true` with body
`{"name": "<plugin-name>"}`.

Two-stage gate:

1. The plugin must already be soft-uninstalled. The host returns 409
   `must be soft-uninstalled` otherwise.
2. The body's `name` must match the path's `:name` exactly. Mismatch →
   400. Mirrors the GitHub-repo-delete typed-name confirm pattern.

In a single DB transaction the host:

1. Runs the down migrations (in reverse order) for every applied
   migration that declared a `DownFilename` + `DownChecksumSHA256` in
   the manifest. Migrations without a down file are skipped with a
   WARN — the resulting tables/columns remain in the DB.
2. Deletes the rows for this plugin from `plugin_settings`,
   `plugin_settings_schemas`, `plugin_migrations`, and `plugins`.

The plugin **binary itself is preserved** — purge only touches database
state. To remove the binary, contact ops; rolling out a new image
without the binary takes the plugin to `absent` on next restart.

### §13.5 Writing down migrations (optional, recommended)

Place a paired `.up.sql` + `.down.sql` under your plugin's
`migrations/` directory and reference both in the manifest:

```go
Migrations: []pluginsdk.MigrationDeclaration{
    {
        Filename:           "001_create_logs.up.sql",
        ChecksumSHA256:     "abc...",
        DownFilename:       "001_create_logs.down.sql",
        DownChecksumSHA256: "def...",
    },
},
```

Hard purge runs the down files in reverse declaration order. If you
omit `DownFilename` for a migration the purge logs:

```
plugin migration has no down — skipping; admin must clean up manually
  plugin=my-plugin migration=001_create_logs.up.sql
```

and the corresponding tables / columns remain. The admin "Hard delete"
dialog warns about this case so operators are not surprised.

### §13.6 Builtin plugins are not uninstallable

Plugins that ship under the host's `BuiltinDir` (e.g., `hello-world`,
`channel-management`) cannot be soft-uninstalled — the manager checks
the binary path and rejects with `ErrPluginIsBuiltin`. The admin UI
hides the "Uninstall" button on builtin cards entirely, matching the
backend gate. Builtins can still be disabled (process stops; row stays)
and re-enabled at will; lifecycle for them is just the
`installed ↔ enabled` toggle.

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
