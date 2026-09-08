# AI Relay Plugin Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the AI Relay management UI into a standalone plugin-service Vue application while restoring the main-site frontend to its exact pre-feature Git state.

**Architecture:** Build a self-contained Vue/Vite app under `plugin-service/plugins/ai-relay/frontend` that calls same-origin `/plugins/ai-relay/api/*` endpoints and emits static assets into the plugin's existing `web` directory. Keep the completed Go path-mapping backend unchanged, replace the redirect-only hosted page with generated assets, and treat any main-site frontend diff against `41875379^` as a release blocker.

**Tech Stack:** Vue 3, TypeScript 5.6, Vite 5, Vitest, Vue Test Utils, CSS, Go `frontendhost`, Git diff verification.

---

## File Structure

- Restore `frontend/src/views/admin/AIRelayView.vue` from `41875379^`; do not otherwise edit it.
- Restore `frontend/src/views/admin/__tests__/AIRelayView.spec.ts` from `41875379^`; do not otherwise edit it.
- Create `plugin-service/plugins/ai-relay/frontend/package.json`: isolated build, typecheck, and test commands.
- Create `plugin-service/plugins/ai-relay/frontend/package-lock.json`: locked frontend dependencies.
- Create `plugin-service/plugins/ai-relay/frontend/tsconfig.json`: strict Vue TypeScript configuration.
- Create `plugin-service/plugins/ai-relay/frontend/vite.config.ts`: plugin base path and deterministic generated asset names.
- Create `plugin-service/plugins/ai-relay/frontend/index.html`: hosted application shell with plugin API base metadata.
- Create `plugin-service/plugins/ai-relay/frontend/src/main.ts`: Vue entry point.
- Create `plugin-service/plugins/ai-relay/frontend/src/types.ts`: route, platform, pagination, and form contracts.
- Create `plugin-service/plugins/ai-relay/frontend/src/api.ts`: same-origin route management client and typed errors.
- Create `plugin-service/plugins/ai-relay/frontend/src/pathMappings.ts`: source-target row conversion and normalization.
- Create `plugin-service/plugins/ai-relay/frontend/src/App.vue`: route management screen and dialogs.
- Create `plugin-service/plugins/ai-relay/frontend/src/styles.css`: standalone responsive management UI.
- Create `plugin-service/plugins/ai-relay/frontend/src/api.spec.ts`: API behavior tests.
- Create `plugin-service/plugins/ai-relay/frontend/src/pathMappings.spec.ts`: mapping helper tests.
- Create `plugin-service/plugins/ai-relay/frontend/src/App.spec.ts`: route management interaction tests.
- Replace generated `plugin-service/plugins/ai-relay/web/index.html` and create `web/assets/app.js`, `web/assets/app.css` through the Vite build.
- Modify `plugin-service/plugins/ai-relay/frontend_test.go`: verify a real hosted app and generated assets, with no main-site redirect.

### Task 1: Restore Main-Site Files To Their Git Baseline

**Files:**
- Restore: `frontend/src/views/admin/AIRelayView.vue`
- Restore: `frontend/src/views/admin/__tests__/AIRelayView.spec.ts`

- [ ] **Step 1: Capture and verify the exact baseline blobs**

Run:

```powershell
git rev-parse 41875379^:frontend/src/views/admin/AIRelayView.vue
git rev-parse 41875379^:frontend/src/views/admin/__tests__/AIRelayView.spec.ts
```

Expected: two blob hashes are printed.

- [ ] **Step 2: Restore only these two files from the approved baseline**

Use `apply_patch` to remove only the path-mapping additions introduced by commit `6367dd0b`, restoring the exact content shown by:

```powershell
git show 41875379^:frontend/src/views/admin/AIRelayView.vue
git show 41875379^:frontend/src/views/admin/__tests__/AIRelayView.spec.ts
```

Do not use `git checkout`, and do not alter the router, menu configuration, shared components, styles, API client, or any other main-site file.

- [ ] **Step 3: Prove the main-site files have zero Git difference**

Run:

```powershell
git diff --exit-code 41875379^ -- frontend/src/views/admin/AIRelayView.vue frontend/src/views/admin/__tests__/AIRelayView.spec.ts
```

Expected: exit code 0 and no output.

- [ ] **Step 4: Commit the ownership correction**

```powershell
git add frontend/src/views/admin/AIRelayView.vue frontend/src/views/admin/__tests__/AIRelayView.spec.ts
git commit -m "fix(ai-relay): keep management UI out of main frontend"
```

### Task 2: Scaffold The Independent Plugin Frontend With Mapping Helpers

**Files:**
- Create: `plugin-service/plugins/ai-relay/frontend/package.json`
- Create: `plugin-service/plugins/ai-relay/frontend/package-lock.json`
- Create: `plugin-service/plugins/ai-relay/frontend/tsconfig.json`
- Create: `plugin-service/plugins/ai-relay/frontend/vite.config.ts`
- Create: `plugin-service/plugins/ai-relay/frontend/index.html`
- Create: `plugin-service/plugins/ai-relay/frontend/src/types.ts`
- Create: `plugin-service/plugins/ai-relay/frontend/src/pathMappings.ts`
- Test: `plugin-service/plugins/ai-relay/frontend/src/pathMappings.spec.ts`

- [ ] **Step 1: Create package and build configuration**

Use the same dependency versions and scripts as `plugins/image-generation/frontend`, limited to Vue, Vite, TypeScript, Vitest, Vue Test Utils, and jsdom. Configure:

```ts
export default defineConfig({
  base: '/plugins/ai-relay/',
  plugins: [vue()],
  build: {
    outDir: fileURLToPath(new URL('../web', import.meta.url)),
    emptyOutDir: true,
    rollupOptions: { output: {
      entryFileNames: 'assets/app.js',
      chunkFileNames: 'assets/[name].js',
      assetFileNames: asset => asset.names?.some(name => name.endsWith('.css')) ? 'assets/app.css' : 'assets/[name][extname]',
    } },
  },
  test: { environment: 'jsdom' },
})
```

Set the application root metadata to:

```html
<div id="app" data-plugin-api-base="/plugins/ai-relay/api"></div>
```

- [ ] **Step 2: Write failing path mapping helper tests**

Cover exact behavior:

```ts
expect(canonicalPath('/v1/responses/compact/')).toBe('responses/compact')
expect(mappingRecordFromRows([
  { id: 1, source: '/v1/responses/compact/', target: '/api/paas/v4/chat/completions/' },
  { id: 2, source: '', target: '' },
])).toEqual({ 'responses/compact': 'api/paas/v4/chat/completions' })
expect(mappingRowsFromRecord({ models: 'api/paas/v4/models' })[0]).toMatchObject({ source: 'models', target: 'api/paas/v4/models' })
```

- [ ] **Step 3: Run tests and verify the helpers are missing**

Run: `npm test -- --run src/pathMappings.spec.ts` from `plugin-service/plugins/ai-relay/frontend`.

Expected: FAIL because `pathMappings.ts` exports do not exist.

- [ ] **Step 4: Implement mapping helpers and types**

Define:

```ts
export interface RelayRoute {
  platform: string
  slug: string
  name: string
  base_url: string
  path_mappings: Record<string, string>
}

export interface MappingRow { id: number; source: string; target: string }
```

Implement `canonicalPath`, `mappingRecordFromRows`, and `mappingRowsFromRecord` without importing main-site code. Preserve stable row IDs through a caller-provided ID factory.

- [ ] **Step 5: Run helper tests and typecheck**

Run: `npm test -- --run src/pathMappings.spec.ts`.

Run: `npm run typecheck`.

Expected: both pass.

- [ ] **Step 6: Commit the plugin frontend foundation**

```powershell
git add plugin-service/plugins/ai-relay/frontend
git commit -m "feat(ai-relay): scaffold plugin frontend"
```

### Task 3: Implement The Typed Plugin API Client

**Files:**
- Create: `plugin-service/plugins/ai-relay/frontend/src/api.ts`
- Test: `plugin-service/plugins/ai-relay/frontend/src/api.spec.ts`

- [ ] **Step 1: Write failing API client tests**

Use a fake `fetch` to verify:

- list query encodes `page`, `page_size`, `platform`, and `search`;
- create sends `POST /routes` JSON;
- update URL-encodes platform and slug and sends `PUT`;
- batch delete sends `{ items: [{ platform, slug }] }`;
- `204` produces `undefined` without JSON parsing;
- JSON `{ error: "administrator access is required" }` with status `403` throws a typed error carrying status `403`;
- non-JSON upstream errors fall back to status text.

The desired factory is:

```ts
const api = createRelayApi('/plugins/ai-relay/api', fetcher)
```

- [ ] **Step 2: Run the API tests and verify failure**

Run: `npm test -- --run src/api.spec.ts`.

Expected: FAIL because `createRelayApi` does not exist.

- [ ] **Step 3: Implement the API client**

Expose `listRoutes`, `listPlatforms`, `createRoute`, `updateRoute`, and `deleteRoutes`. Use `credentials: 'same-origin'`; do not read main-site stores or import its Axios client. Rely on `frontendhost`'s injected auth bridge for bearer authentication.

- [ ] **Step 4: Run API tests and typecheck**

Run: `npm test -- --run src/api.spec.ts`.

Run: `npm run typecheck`.

Expected: both pass.

- [ ] **Step 5: Commit the API client**

```powershell
git add plugin-service/plugins/ai-relay/frontend/src/api.ts plugin-service/plugins/ai-relay/frontend/src/api.spec.ts
git commit -m "feat(ai-relay): add plugin route API client"
```

### Task 4: Build The Plugin Route Management Application

**Files:**
- Create: `plugin-service/plugins/ai-relay/frontend/src/main.ts`
- Create: `plugin-service/plugins/ai-relay/frontend/src/App.vue`
- Create: `plugin-service/plugins/ai-relay/frontend/src/styles.css`
- Test: `plugin-service/plugins/ai-relay/frontend/src/App.spec.ts`

- [ ] **Step 1: Write failing application interaction tests**

Mount `App.vue` with an injected fake `RelayApi`. Cover:

- initial platform and route loading;
- loading, empty, API error, and status-403 unauthorized states;
- search and platform filters reload page 1;
- add dialog validates name, platform, slug, and HTTPS base URL;
- adding/removing mapping rows uses accessible controls;
- create payload includes normalized `path_mappings` and omits blank rows;
- edit dialog loads existing mappings and uses `updateRoute`;
- row selection and batch delete send exact route references;
- copy action writes the route URL through an injected clipboard abstraction;
- pagination changes page and page size.

Use stable selectors such as `route-add`, `route-edit`, `path-mapping-add`, `path-mapping-source`, `path-mapping-target`, `path-mapping-remove`, and `route-delete-selected`.

- [ ] **Step 2: Run the app tests and verify failure**

Run: `npm test -- --run src/App.spec.ts`.

Expected: FAIL because the application does not exist.

- [ ] **Step 3: Implement the management application**

Build a quiet operations UI with:

- compact header containing title, route count, refresh icon, and add command;
- filter row with labeled search and platform select;
- semantic table with checkboxes, route identity, target URL, relay URL, and icon actions;
- full-width empty/error/unauthorized states rather than decorative cards;
- native modal dialogs with backdrop, Escape close, initial focus, focus restoration, and named controls;
- responsive mapping rows that stack on mobile;
- confirmation dialog for single and batch deletion;
- fixed-size icon buttons with `aria-label` and inline SVG icons local to the plugin;
- no gradients, animation, nested cards, or main-site CSS imports.

Read the API base from `#app[data-plugin-api-base]` and construct the API in `main.ts`.

- [ ] **Step 4: Run app tests, complete plugin tests, and typecheck**

Run: `npm test`.

Run: `npm run typecheck`.

Expected: all pass with no Vue warnings.

- [ ] **Step 5: Commit the application source**

```powershell
git add plugin-service/plugins/ai-relay/frontend/src
git commit -m "feat(ai-relay): add standalone route manager"
```

### Task 5: Generate And Serve The Plugin Application

**Files:**
- Modify: `plugin-service/plugins/ai-relay/frontend_test.go`
- Generate: `plugin-service/plugins/ai-relay/web/index.html`
- Generate: `plugin-service/plugins/ai-relay/web/assets/app.js`
- Generate: `plugin-service/plugins/ai-relay/web/assets/app.css`

- [ ] **Step 1: Update the Go frontend test first**

Replace redirect assertions with hosted-app assertions:

```go
for _, marker := range []string{
	`/plugins/ai-relay-assets/app.js`,
	`/plugins/ai-relay-assets/app.css`,
	`data-plugin-api-base="/plugins/ai-relay/api"`,
} {
	if !strings.Contains(rec.Body.String(), marker) { t.Fatalf("missing %q", marker) }
}
for _, forbidden := range []string{`/admin/ai-relay`, `window.location.replace`} {
	if strings.Contains(rec.Body.String(), forbidden) { t.Fatalf("contains redirect %q", forbidden) }
}
```

Add asset GET checks for JavaScript and CSS content types and non-empty bodies.

- [ ] **Step 2: Run Go frontend tests and verify failure**

Run: `go test ./plugins/ai-relay -run 'TestFrontend' -count=1` from `plugin-service`.

Expected: FAIL because `web/index.html` is still the redirect shell and assets do not exist.

- [ ] **Step 3: Build generated frontend assets**

Run: `npm run build` from `plugin-service/plugins/ai-relay/frontend`.

Expected: Vite writes deterministic `web/index.html`, `web/assets/app.js`, and `web/assets/app.css`.

- [ ] **Step 4: Run hosted frontend and plugin tests**

Run: `go test ./plugins/ai-relay -run 'TestFrontend' -count=1` from `plugin-service`.

Run: `go test ./plugins/ai-relay/... -count=1` from `plugin-service`.

Expected: all pass.

- [ ] **Step 5: Commit generated assets and host tests**

```powershell
git add plugin-service/plugins/ai-relay/frontend_test.go plugin-service/plugins/ai-relay/web
git commit -m "feat(ai-relay): serve standalone plugin frontend"
```

### Task 6: Final Ownership And Regression Verification

**Files:**
- Verify only; change plugin-owned files only if a regression is found.

- [ ] **Step 1: Prove the main-site files match the baseline**

Run:

```powershell
git diff --exit-code 41875379^ -- frontend/src/views/admin/AIRelayView.vue frontend/src/views/admin/__tests__/AIRelayView.spec.ts
git status --short frontend
```

Expected: the first command has exit code 0 with no output; the second command has no output. If either command reports a main-site change, stop and restore only the identified unintended difference. Do not modify another main-site file without user approval.

- [ ] **Step 2: Run all plugin frontend verification**

From `plugin-service/plugins/ai-relay/frontend` run:

```powershell
npm test
npm run typecheck
npm run build
```

Expected: tests, typecheck, and build pass without application warnings.

- [ ] **Step 3: Run all plugin-service tests**

From `plugin-service` run:

```powershell
go test ./... -count=1
```

Expected: all packages pass.

- [ ] **Step 4: Inspect final scope and generated files**

Run:

```powershell
git diff --check
git status --short
git diff --stat 41875379^
```

Expected: clean worktree after commits; feature changes are limited to plugin-service AI Relay backend/frontend, generated plugin assets, documentation, and the explicit restoration commit. No main-site frontend content differs from the baseline.
