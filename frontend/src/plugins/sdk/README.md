# Sub2API Plugin Frontend SDK

This document is the contract between the host shell (this repo's frontend) and
plugin frontend bundles served via `/api/v1/plugin-assets/<plugin>/<path>`.

> Stable as of `HOST_SDK_VERSION = 1.0.0`. Plugin authors should read the
> `version` field at runtime and feature-gate accordingly.

## How a plugin is loaded

1. The host injects `window.__PLUGIN_MANIFESTS__` from the backend (each entry
   is a `PluginManifest`, see `loader.ts`).
2. When the user navigates to a route registered by a plugin, the
   `PluginView.vue` container reads `manifest.entry_js_url` and asks
   `loader-runtime.ts` to dynamically `import()` it.
3. The runtime loader picks the module's `install` export (either named or via
   `default`) and invokes `install(sdk)`.
4. `install` returns `{ components: { [componentPath]: VueComponent } }` (and
   optionally extra `routes`). The host caches these and resolves the right
   component via `manifest.routes[].component_path`.

## install contract

```ts
// entry.js — must be a valid ESM module
import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'

export function install(sdk: HostSdk): PluginRuntimeAssets {
  // ...
  return { components: { 'MyView.vue': MyComponent } }
}

export default { install }
```

`install` may be sync or `async`. The host awaits the returned promise.

## HostSdk surface

```ts
interface HostSdk {
  version: string                // "1.0.0"
  theme: HostTheme               // light / dark + toggle
  font: HostFont                 // size + family (defaults only for now)
  i18n: HostI18n                 // t / currentLocale / registerNamespace
  notify: HostNotify             // success / error / warning / info
  router: HostRouter             // push / back / currentRoute
  auth: HostAuth                 // user / isAdmin / isAuthenticated (read-only)
  http: HostHttp                 // axios instance with auth + locale headers
  vue: HostVue                   // h, defineComponent, ref, computed, watch...
}
```

### Theme

```js
function install(sdk) {
  // mode is a Vue Ref<'light' | 'dark'>. Reading is reactive in setup().
  console.log(sdk.theme.mode.value)
  // Toggle the host theme; effect is reflected on <html>.
  sdk.theme.toggle()
  // Force a specific mode.
  sdk.theme.set('dark')
}
```

### i18n

```js
function install(sdk) {
  sdk.i18n.registerNamespace('myPlugin', {
    en: { hello: 'Hi {name}' },
    zh: { hello: '你好 {name}' },
  })
  // Inside a setup(), this is reactive on locale change.
  sdk.i18n.t('myPlugin.hello', { name: 'world' })
  console.log(sdk.i18n.currentLocale.value) // 'en' or 'zh'
}
```

`registerNamespace` calls vue-i18n's `mergeLocaleMessage` per locale. Calling it
multiple times for the same namespace is safe; later calls merge into earlier
ones rather than overwriting.

### Notify

```js
sdk.notify.success('Saved!', 3000)  // duration is optional (host defaults apply)
sdk.notify.error('Oops')
```

### Vue runtime

```js
function install(sdk) {
  const { defineComponent, h, ref, computed } = sdk.vue
  const View = defineComponent({
    setup() {
      const count = ref(0)
      const isDark = computed(() => sdk.theme.mode.value === 'dark')
      return () => h('div', null, [`count=${count.value}`, ` dark=${isDark.value}`])
    },
  })
  return { components: { 'View.vue': View } }
}
```

Plugins should prefer `sdk.vue.*` over importing `vue` themselves; this keeps
the bundle small and avoids loading two Vue runtimes side-by-side.

### HTTP

```js
const { apiClient } = sdk.http
const { data } = await apiClient.get('/admin/accounts')
```

`apiClient` already attaches `Authorization`, `Accept-Language`, and timezone
on requests, plus the host's response interceptor for `code/message` errors.

## Component lookup

A plugin manifest's `routes[]` entry looks like:

```jsonc
{
  "path": "/admin/plugins/my-plugin",
  "name": "PluginMy",
  "component_path": "MyView.vue"
}
```

`PluginView.vue` will look up `assets.components['MyView.vue']` after `install`
returns. If only one component is registered and the manifest leaves
`component_path` blank, the host renders that single component as a fallback.

## Bundling

The hello-world plugin ships a hand-written ESM (`plugins/hello-world/frontend/dist/entry.js`)
to keep the smoke test bundler-free. Real plugins should use Vite library mode
(see `plugins/channel-management/frontend/vite.config.ts` for an existing
config, though it predates this loader).

Vite library config sketch:

```ts
// vite.config.ts
import { defineConfig } from 'vite'
export default defineConfig({
  build: {
    lib: { entry: 'src/index.ts', formats: ['es'], fileName: () => 'entry.js' },
    rollupOptions: { external: [/^@?vue($|\/)/, /^vue-router/, /^pinia/] },
  },
})
```

## Versioning rules

- `HOST_SDK_VERSION` is semver. New optional fields bump the minor; removing or
  changing field types bumps the major.
- Plugins should check `sdk.version` at install time and degrade gracefully if
  needed (e.g. branch on major).

## Isolation modes

`manifest.isolation` is one of:

- `'shared'` (default) — entry.js runs in the host realm with full access to
  the SDK. Use this unless you know you need stricter isolation.
- `'iframe'` — reserved; not implemented in 1.0.0. Loader will error out.
